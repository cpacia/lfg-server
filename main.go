package main

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/ulule/limiter/v3"
	mhttp "github.com/ulule/limiter/v3/drivers/middleware/stdlib"
	memstore "github.com/ulule/limiter/v3/drivers/store/memory"
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
	imageDirName   = "images"
	dbName         = "lfg.db"
	jwtKeyHex      = "e2e9b8cde02cf305bd521ff4ec987d1bb5a8627743f93db1efe15a228b4daaaa"
	userContextKey = contextKey("user")
)

type contextKey string

type Server struct {
	db       *gorm.DB
	r        chi.Router
	imageDir string
}

var (
	jwtKey    []byte
	rateLimit = "4-H"
)

func init() {
	var err error
	jwtKey, err = hex.DecodeString(jwtKeyHex)
	if err != nil {
		log.Fatal("error parsing jwt key")
	}
}

func main() {
	db, dataDir, err := initDatabase()
	if err != nil {
		log.Fatalf("Database initialization errored: %s", err)
	}

	r := chi.NewRouter()

	store := memstore.NewStore()
	rate, err := limiter.NewRateFromFormatted(rateLimit)
	if err != nil {
		log.Fatal("error parsing jwt key")
	}

	lim := limiter.New(store, rate, limiter.WithTrustForwardHeader(true))
	rateLimitMiddleware := mhttp.NewMiddleware(lim)

	// Middleware
	r.Use(middleware.Logger)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300, // in seconds
	}))

	s := &Server{
		db:       db,
		r:        r,
		imageDir: path.Join(dataDir, imageDirName),
	}

	r.Post("/login", func(w http.ResponseWriter, r *http.Request) {
		rateLimitMiddleware.Handler(http.HandlerFunc(s.POSTLoginHandler)).ServeHTTP(w, r)
	})
	r.Get("/auth/me", authMiddleware(s.POSTAuthMe))
	r.Post("/change-password", authMiddleware(s.POSTChangePasswordHandler))

	r.Get("/standings", s.GETStandings)
	r.Post("/standings", authMiddleware(s.POSTStandingsUrls))
	r.Post("/refresh-standings", authMiddleware(s.POSTRefreshStandings))

	r.Get("/events", s.GETEvents)
	r.Get("/events/{eventID}", s.GETEvent)
	r.Get("/events/{eventID}/thumbnail", s.GETEventThumbnail)
	r.Post("/events", authMiddleware(s.POSTEvent))
	r.Put("/events/{eventID}", authMiddleware(s.PUTEvent))
	r.Delete("/events/{eventID}", authMiddleware(s.DELETEEvent))

	r.Get("/results/net/{eventID}", s.GETNetResults)
	r.Get("/results/gross/{eventID}", s.GETGrossResults)
	r.Get("/results/skins/{eventID}", s.GETSkinsResults)
	r.Get("/results/teams/{eventID}", s.GETTeamResults)
	r.Get("/results/wgr/{eventID}", s.GETWgrResults)

	r.Get("/disabled-golfers/{name}", s.GETDisabledGolfer)
	r.Post("/disabled-golfers", authMiddleware(s.POSTDisabledGolfer))
	r.Put("/disabled-golfers/{name}", authMiddleware(s.PUTDisabledGolfer))
	r.Delete("/disabled-golfers/{name}", authMiddleware(s.DELETEDisabledGolfer))

	r.Get("/colony-cup/{year}", s.GETColonyCupInfo)
	r.Post("/colony-cup", authMiddleware(s.POSTColonyCupInfo))
	r.Put("/colony-cup/{year}", authMiddleware(s.PUTColonyCupInfo))
	r.Delete("/colony-cup/{year}", authMiddleware(s.DELETEColonyCupInfo))

	r.Get("/match-play", s.GETMatchPlayInfo)
	r.Put("/match-play", authMiddleware(s.PUTMatchPlayInfo))
	r.Get("/match-play/results", s.GETMatchPlayResults)

	r.Get("/current-year", s.GETCurrentYear)

	http.ListenAndServe(":8080", r)
}

// Check to see if the database exists. If not create it and initialize
// it with a default admin password to be changed later.
func initDatabase() (*gorm.DB, string, error) {
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

	err = os.MkdirAll(path.Join(dataDirPath, imageDirName), os.ModePerm)
	if err != nil {
		return nil, "", err
	}

	db, err := gorm.Open(sqlite.Open(path.Join(dataDirPath, dbName)), &gorm.Config{})
	if err != nil {
		return nil, "", err
	}

	// Migrate the schema
	if err := applyMigrations(db); err != nil {
		return nil, "", err
	}

	var creds DBCredentials
	result := db.First(&creds)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			hash, err := bcrypt.GenerateFromPassword([]byte("letmein"), bcrypt.DefaultCost)
			if err != nil {
				return nil, "", err
			}
			result := db.Create(&DBCredentials{Username: "admin", PasswordHash: string(hash)})
			if result.Error != nil {
				return nil, "", err
			}

			result = db.Create(&MatchPlayInfo{
				RegistrationOpen: false,
			})
			if result.Error != nil {
				return nil, "", err
			}
		} else {
			return nil, "", result.Error
		}
	}

	return db, dataDirPath, nil
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
