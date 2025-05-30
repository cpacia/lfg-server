package main

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log"
	"net/http"
	"os"
	"os/user"
	"path"
)

const (
	dataDir        = ".lfgserver"
	dbName         = "lfg.db"
	jwtKeyHex      = "e2e9b8cde02cf305bd521ff4ec987d1bb5a8627743f93db1efe15a228b4daaaa"
	userContextKey = contextKey("user")
)

type contextKey string

type Server struct {
	db *gorm.DB
	r  chi.Router
}

var jwtKey []byte

func init() {
	var err error
	jwtKey, err = hex.DecodeString(jwtKeyHex)
	if err != nil {
		log.Fatal("error parsing jwt key")
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

	r.Post("/login", s.POSTLoginHandler)
	r.Post("/changepw", authMiddleware(s.POSTChangePasswordHandler))

	http.ListenAndServe(":8080", r)
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

// Validate the JWT token. It can either been in a cookie or a header.
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
