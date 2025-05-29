package main

import (
	"errors"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log"
	"os"
	"os/user"
	"path"
)

const (
	dataDir = ".lfgserver"
	dbName  = "lfg.db"
)

// User represents a user in the admin site.
// Passwords will be stored as hashes.
type User struct {
	gorm.Model
	Username     string
	PasswordHash string
}

func main() {
	_, err := initDatabase()
	if err != nil {
		log.Fatalf("Database initialization errored: %w", err)
	}
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
	err = db.AutoMigrate(&User{})
	if err != nil {
		return nil, err
	}

	var user User
	result := db.First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			hash, err := bcrypt.GenerateFromPassword([]byte("letmein"), bcrypt.DefaultCost)
			if err != nil {
				return nil, err
			}
			result := db.Create(&User{Username: "admin", PasswordHash: string(hash)})
			if result.Error != nil {
				return nil, err
			}
		} else {
			return nil, result.Error
		}
	}

	return db, nil
}
