package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/csrf"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log"
	"net/http"
	"os"
	"os/user"
	"path"
	"time"
)

const (
	dataDir        = ".lfgserver"
	dbName         = "lfg.db"
	jwtKeyHex      = "e2e9b8cde02cf305bd521ff4ec987d1bb5a8627743f93db1efe15a228b4daaaa"
	csrfKeyHex     = "669bdc2841faa7a80ab72d19e6084a72a050a61d266b8edaafdaf80b127f31a0"
	userContextKey = contextKey("user")
)

type contextKey string

// DBCredentials represents admin credentials stored
// in the database
type DBCredentials struct {
	gorm.Model
	Username     string
	PasswordHash string
}

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type PWChangeRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

type Server struct {
	db *gorm.DB
	r  chi.Router
}

var (
	jwtKey  []byte
	csrfKey []byte
)

func init() {
	var err error
	jwtKey, err = hex.DecodeString(jwtKeyHex)
	if err != nil {
		log.Fatal("error parsing jwt key")
	}
	csrfKey, err = hex.DecodeString(csrfKeyHex)
	if err != nil {
		log.Fatal("error parsing csrf key")
	}
}

func main() {
	db, err := initDatabase()
	if err != nil {
		log.Fatalf("Database initialization errored: %w", err)
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)

	s := &Server{
		db: db,
		r:  r,
	}

	r.Post("/login", s.loginHandler)

	r.Post("/changepw", authMiddleware(s.changePWHandler))
	r.Get("/csrf-token", func(w http.ResponseWriter, r *http.Request) {
		token := csrf.Token(r)
		log.Println("[CSRF] Issuing token:", token)
		w.Header().Set("X-CSRF-Token", token)
	})

	http.ListenAndServe(":8080", csrf.Protect([]byte(csrfKey), csrf.Secure(false))(r))
}

// Check to see if the database exists. If not create it and initialize
// it with a default admin password to be changed later.
func initDatabase() (*gorm.DB, error) {
	// Get the OS specific home directory via the Go standard lib.
	var homeDir string
	usr, err := user.Current()
	if err == nil {
		homeDir = usr.HomeDir
	}

	// Fall back to standard HOME environment variable that works
	// for most POSIX OSes if the directory from the Go standard
	// lib failed.
	if err != nil || homeDir == "" {
		homeDir = os.Getenv("HOME")
	}

	dataDirPath := path.Join(homeDir, dataDir)

	err = os.MkdirAll(dataDirPath, os.ModePerm)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(sqlite.Open(path.Join(dataDirPath, dbName)), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Migrate the schema
	err = db.AutoMigrate(&DBCredentials{})
	if err != nil {
		return nil, err
	}

	var creds DBCredentials
	result := db.First(&creds)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			hash, err := bcrypt.GenerateFromPassword([]byte("letmein"), bcrypt.DefaultCost)
			if err != nil {
				return nil, err
			}
			result := db.Create(&DBCredentials{Username: "admin", PasswordHash: string(hash)})
			if result.Error != nil {
				return nil, err
			}
		} else {
			return nil, result.Error
		}
	}

	return db, nil
}

func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	dbCreds := &DBCredentials{}
	result := s.db.First(dbCreds, "username = ?", creds.Username)
	if result.Error != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	err := bcrypt.CompareHashAndPassword([]byte(dbCreds.PasswordHash), []byte(creds.Password))
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	expiration := time.Now().Add(15 * time.Minute)
	claims := &Claims{
		Username: creds.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiration),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(jwtKey))
	if err != nil {
		http.Error(w, "Could not generate token", http.StatusInternalServerError)
		return
	}

	// Set HTTP-only JWT cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    tokenStr,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) changePWHandler(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(userContextKey).(*Claims)
	if !ok || claims == nil {
		http.Error(w, "User info not found in context", http.StatusInternalServerError)
		return
	}

	var pwChangeReq PWChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&pwChangeReq); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	dbCreds := &DBCredentials{}
	result := s.db.First(dbCreds, "username = ?", claims.Username)
	if result.Error != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	err := bcrypt.CompareHashAndPassword([]byte(dbCreds.PasswordHash), []byte(pwChangeReq.CurrentPassword))
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(pwChangeReq.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Could not check password", http.StatusInternalServerError)
		return
	}
	dbCreds.PasswordHash = string(hash)
	if err := s.db.Save(&dbCreds).Error; err != nil {
		http.Error(w, "Could not save password", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var tokenStr string

		// First try Authorization header
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) >= 7 && authHeader[:7] == "Bearer " {
			tokenStr = authHeader[7:]
		} else {
			// Fallback to auth_token cookie
			cookie, err := r.Cookie("auth_token")
			if err != nil {
				http.Error(w, "Missing auth token", http.StatusUnauthorized)
				return
			}
			tokenStr = cookie.Value
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Token is valid, proceed
		ctx := context.WithValue(r.Context(), userContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
