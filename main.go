package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jessevdk/go-flags"
	"github.com/ulule/limiter/v3"
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

type Options struct {
	Dev     bool   `long:"dev" description:"Use run a development server on localhost"`
	DataDir string `short:"d" long:"datadir" description:"Data directory to use for the db and images"`
}

type contextKey string

type Server struct {
	db               *gorm.DB
	r                chi.Router
	imageDir         string
	loginRateLimiter *limiter.Limiter
	devMode          bool
}

var (
	jwtKey    []byte
	rateLimit = "5-H"
)

func init() {
	var err error
	jwtKey, err = hex.DecodeString(jwtKeyHex)
	if err != nil {
		log.Fatal("error parsing jwt key")
	}
}

func main() {
	var opts Options
	parser := flags.NewNamedParser("faucet", flags.Default)
	parser.AddGroup("Options", "Configuration options for the server", &opts)
	if _, err := parser.Parse(); err != nil {
		return
	}

	db, dataDir, err := initDatabase(opts.DataDir)
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

	// Middleware
	r.Use(middleware.Logger)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "livefreegolfwebite-production.up.railway.app"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300, // in seconds
	}))

	s := &Server{
		db:               db,
		r:                r,
		imageDir:         path.Join(dataDir, imageDirName),
		loginRateLimiter: lim,
		devMode:          opts.Dev,
	}

	r.Post("/api/login", s.POSTLoginHandler)
	r.Post("/api/logout", s.POSTLogoutHandler)
	r.Get("/api/auth/me", authMiddleware(s.POSTAuthMe))
	r.Post("/api/change-password", authMiddleware(s.POSTChangePasswordHandler))

	r.Get("/api/standings", s.GETStandings)
	r.Get("/api/standings-urls", s.GETStandingsUrls)
	r.Post("/api/standings-urls", authMiddleware(s.POSTStandingsUrls))
	r.Put("/api/standings-urls", authMiddleware(s.PUTStandingsUrls))
	r.Delete("/api/standings-urls", authMiddleware(s.DELETEStandingsUrls))
	r.Post("/api/refresh-standings", authMiddleware(s.POSTRefreshStandings))

	r.Get("/api/events", s.GETEvents)
	r.Get("/api/events/{eventID}", s.GETEvent)
	r.Get("/api/events/{eventID}/thumbnail", s.GETEventThumbnail)
	r.Post("/api/events", authMiddleware(s.POSTEvent))
	r.Put("/api/events/{eventID}", authMiddleware(s.PUTEvent))
	r.Delete("/api/events/{eventID}", authMiddleware(s.DELETEEvent))

	r.Get("/api/results/net/{eventID}", s.GETNetResults)
	r.Get("/api/results/gross/{eventID}", s.GETGrossResults)
	r.Get("/api/results/skins/{eventID}", s.GETSkinsResults)
	r.Get("/api/results/teams/{eventID}", s.GETTeamResults)
	r.Get("/api/results/wgr/{eventID}", s.GETWgrResults)

	r.Get("/api/disabled-golfers", s.GETDisabledGolfer)
	r.Post("/api/disabled-golfers/{name}", authMiddleware(s.POSTDisabledGolfer))
	r.Put("/api/disabled-golfers/{name}", authMiddleware(s.PUTDisabledGolfer))
	r.Delete("/api/disabled-golfers/{name}", authMiddleware(s.DELETEDisabledGolfer))

	r.Get("/api/colony-cup", s.GETColonyCupInfo)
	r.Get("/api/colony-cup/all", s.GETAllColonyCupInfo)
	r.Post("/api/colony-cup", authMiddleware(s.POSTColonyCupInfo))
	r.Put("/api/colony-cup", authMiddleware(s.PUTColonyCupInfo))
	r.Delete("/api/colony-cup", authMiddleware(s.DELETEColonyCupInfo))

	r.Get("/api/match-play", s.GETMatchPlayInfo)
	r.Put("/api/match-play", authMiddleware(s.PUTMatchPlayInfo))
	r.Post("/api/match-play", authMiddleware(s.POSTMatchPlayInfo))
	r.Delete("/api/match-play", authMiddleware(s.DELETEMatchPlayInfo))
	r.Post("/api/refresh-match-play-bracket", authMiddleware(s.POSTRefreshMatchPlayBracket))
	r.Get("/api/match-play/results", s.GETMatchPlayResults)

	r.Get("/api/current-year", s.GETCurrentYear)

	http.ListenAndServe(":8080", r)
}

// Check to see if the database exists. If not create it and initialize
// it with a default admin password to be changed later.
func initDatabase(dataDirOpt string) (*gorm.DB, string, error) {

	var dataDirPath string
	if dataDirOpt == "" {
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

		dataDirPath = path.Join(homeDir, dataDir)
	} else {
		dataDirPath = dataDirOpt
	}
	fmt.Println("Using data directory:", dataDirPath)

	err := os.MkdirAll(path.Join(dataDirPath, imageDirName), os.ModePerm)
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
