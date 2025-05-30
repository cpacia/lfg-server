package main

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"strings"
	"time"
)

type Credentials struct {
	Username string `json:"username" gorm:"index"`
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

type DBCredentials struct {
	gorm.Model
	Username     string `gorm:"unique"`
	PasswordHash string
}

type Event struct {
	gorm.Model
	EventID             string         `json:"eventID" gorm:"primaryKey"`
	Date                datatypes.Date `json:"date"`
	Name                string         `json:"name"`
	Course              string         `json:"course"`
	Town                string         `json:"town"`
	State               string         `json:"state"`
	HandicapAllowance   string         `json:"handicapAllowance"`
	BlueGolfUrl         string         `json:"blueGolfUrl"`
	Thumbnail           string         `json:"thumbnail"`
	RegistrationOpen    bool           `json:"registrationOpen"`
	IsComplete          bool           `json:"isComplete"`
	NetLeaderboardUrl   string         `json:"netLeaderboardUrl"`
	GrossLeaderboardUrl string         `json:"grossLeaderboardUrl"`
	SkinsLeaderboardUrl string         `json:"skinsLeaderboardUrl"`
	TeamsLeaderboardUrl string         `json:"teamsLeaderboardUrl"`
	WgrLeaderboardUrl   string         `json:"wgrLeaderboardUrl"`
}

func (e *Event) BeforeCreate(tx *gorm.DB) (err error) {
	// Extract year
	t := time.Time(e.Date)
	year := t.Year()

	// Sanitize the name to avoid spaces or weird characters in the ID
	nameSlug := strings.ReplaceAll(strings.ToLower(e.Name), " ", "-")

	// Set the ID
	e.EventID = fmt.Sprintf("%d-%s", year, nameSlug)
	return
}

func (e *Event) ResultsUpdated() bool {
	return e.NetLeaderboardUrl != "" ||
		e.GrossLeaderboardUrl != "" ||
		e.SkinsLeaderboardUrl != "" ||
		e.TeamsLeaderboardUrl != "" ||
		e.WgrLeaderboardUrl != ""
}

type NetResult struct {
	gorm.Model
	EventID      string `json:"eventID" gorm:"index"`
	Rank         string `json:"rank"`
	Player       string `json:"player"`
	Total        string `json:"total"`
	Strokes      string `json:"strokes"`
	Points       string `json:"points"`
	ScorecardUrl string `json:"scorecardUrl"`
}

type GrossResult struct {
	gorm.Model
	EventID      string `json:"eventID" gorm:"index"`
	Rank         string `json:"rank"`
	Player       string `json:"player"`
	Total        string `json:"total"`
	Strokes      string `json:"strokes"`
	ScorecardUrl string `json:"scorecardUrl"`
}

type SkinsPlayerResult struct {
	gorm.Model
	EventID      string `json:"eventID" gorm:"index"`
	Rank         string `json:"rank"`
	Player       string `json:"player"`
	Skins        string `json:"skins"`
	ScorecardUrl string `json:"scorecardUrl"`
}

type SkinsHolesResult struct {
	gorm.Model
	EventID string `json:"eventID" gorm:"index"`
	Hole    string `json:"hole"`
	Par     string `json:"par"`
	Score   string `json:"score"`
	Won     string `json:"won"`
	Tie     string `json:"tie"`
}

type TeamResult struct {
	gorm.Model
	EventID string `json:"eventID" gorm:"index"`
	Rank    string `json:"rank"`
	Team    string `json:"team"`
	Total   string `json:"total"`
	Strokes string `json:"strokes"`
}

type WGRResult struct {
	gorm.Model
	EventID      string `json:"eventID" gorm:"index"`
	Rank         string `json:"rank"`
	Player       string `json:"player"`
	Total        string `json:"total"`
	Strokes      string `json:"strokes"`
	Points       string `json:"points"`
	ScorecardUrl string `json:"scorecardUrl"`
}

type Standings struct {
	gorm.Model
	CalendarYear       string `json:"calendarYear" gorm:"unique"`
	SeasonStandingsUrl string `json:"seasonStandingsUrl"`
	WgrStandingsUrl    string `json:"wgrStandingsUrl"`
}

type DisabledGolfer struct {
	gorm.Model
	Name     string `json:"name" gorm:"uniqueIndex"`
	Reason   string `json:"reason"`
	Duration string `json:"duration"`
}

type MatchPlayInfo struct {
	gorm.Model
	RegistrationOpen bool   `json:"registrationOpen"`
	Year             string `json:"year"`
	BracketUrl       string `json:"bracketUrl"`
}

type MatchPlayMatch struct {
	gorm.Model
	Year     string `gorm:"index"`
	Round    string // e.g. "Round of 32", "Quarterfinals", etc.
	Player1  string
	Player2  string
	Winner   string // Optional: empty if not yet played
	MatchNum int    // Optional: for ordering within round
}

type ColonyCupInfo struct {
	gorm.Model
	Year        string         `json:"year" gorm:"uniqueIndex"`
	WinningTeam datatypes.JSON `gorm:"type:json"`
}

type SeasonRank struct {
	gorm.Model
	Year   string `json:"year"`
	Player string `json:"player"`
	Rank   string `json:"rank"`
	Events string `json:"events"`
	Points string `json:"points"`
}

type WGRRank struct {
	gorm.Model
	Year   string `json:"year"`
	Player string `json:"player"`
	Rank   string `json:"rank"`
	Events string `json:"events"`
	Points string `json:"points"`
}
