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
		log.Fatalf("Database initialization errored: %s", err)
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)

	s := &Server{
		db: db,
		r:  r,
	}

	r.Post("/login", s.POSTLoginHandler)
	r.Post("/change-password", authMiddleware(s.POSTChangePasswordHandler))

	r.Get("/standings", s.GetStandings)
	r.Get("/standings/year/{year}", s.GetStandingsByYear)
	r.Post("/standings", authMiddleware(s.POSTStandingsUrls))
	r.Post("/refresh-standings", authMiddleware(s.POSTRefreshStandings))

	r.Get("/events", s.GETEvents)
	r.Get("/events/{eventID}", s.GETEvent)
	r.Get("/events/year/{year}", s.GETEventsByYear)
	r.Get("/events/year/current", s.GetCurrentYear)
	r.Post("/events", authMiddleware(s.POSTEvent))
	r.Put("/events/{eventID}", authMiddleware(s.PUTEvent))
	r.Delete("/events/{eventID}", authMiddleware(s.DELETEEvent))

	r.Get("/results/net/{eventID}", s.GetNetResults)
	r.Get("/results/gross/{eventID}", s.GetGrossResults)
	r.Get("/results/skins/{eventID}", s.GetSkinsResults)
	r.Get("/results/teams/{eventID}", s.GetTeamResults)
	r.Get("/results/wgr/{eventID}", s.GetWgrResults)

	r.Get("/disabled-golfers/{name}", s.GetDisabledGolfer)
	r.Post("/disabled-golfers", authMiddleware(s.PostDisabledGolfer))
	r.Put("/disabled-golfers/{name}", authMiddleware(s.PutDisabledGolfer))
	r.Delete("/disabled-golfers/{name}", authMiddleware(s.DeleteDisabledGolfer))

	r.Get("/colony-cup/{year}", s.GetColonyCupInfo)
	r.Post("/colony-cup", authMiddleware(s.PostColonyCupInfo))
	r.Put("/colony-cup/{year}", authMiddleware(s.PutColonyCupInfo))
	r.Delete("/colony-cup/{year}", authMiddleware(s.DeleteColonyCupInfo))

	r.Get("/match-play", s.GetMatchPlayInfo)
	r.Put("/match-play", authMiddleware(s.PutMatchPlayInfo))

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
	if err := applyMigrations(db); err != nil {
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

			result = db.Create(&MatchPlayInfo{
				RegistrationOpen: false,
			})
			if result.Error != nil {
				return nil, err
			}
		} else {
			return nil, result.Error
		}
	}

	return db, nil
}

func applyMigrations(db *gorm.DB) error {
	return db.AutoMigrate(
		&DBCredentials{},
		&Event{},
		&Standings{},
		&SeasonRank{},
		&WGRRank{},
		&MatchPlayInfo{},
		&MatchPlayMatch{},
		&ColonyCupInfo{},
		&DisabledGolfer{},
		&NetResult{},
		&GrossResult{},
		&SkinsPlayerResult{},
		&SkinsHolesResult{},
		&TeamResult{},
		&WGRResult{})
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
