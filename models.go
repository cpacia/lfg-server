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
	EventID             string `json:"eventID" gorm:"primaryKey"`
	Date                datatypes.Date
	DateString          string `json:"date"`
	Name                string `json:"name"`
	Course              string `json:"course"`
	Town                string `json:"town"`
	State               string `json:"state"`
	HandicapAllowance   string `json:"handicapAllowance"`
	BlueGolfUrl         string `json:"blueGolfUrl"`
	ShopifyUrl          string `json:"shopifyUrl"`
	Thumbnail           string `json:"thumbnail"`
	RegistrationOpen    bool   `json:"registrationOpen"`
	IsComplete          bool   `json:"isComplete"`
	NetLeaderboardUrl   string `json:"netLeaderboardUrl"`
	GrossLeaderboardUrl string `json:"grossLeaderboardUrl"`
	SkinsLeaderboardUrl string `json:"skinsLeaderboardUrl"`
	TeamsLeaderboardUrl string `json:"teamsLeaderboardUrl"`
	WgrLeaderboardUrl   string `json:"wgrLeaderboardUrl"`
}

func (e *Event) BeforeSave(tx *gorm.DB) (err error) {
	parsedDate, err := time.Parse("2006-01-02", e.DateString)
	if err != nil {
		return err
	}
	e.Date = datatypes.Date(parsedDate)
	return
}

func (e *Event) BeforeCreate(tx *gorm.DB) (err error) {
	// Extract year
	t := time.Time(e.Date)
	year := t.Year()

	name := strings.ToLower(e.Name)
	name = strings.ReplaceAll(name, " ", "-")

	// Remove all non-alphanumeric/dash characters
	safeSlug := basicSanitize(strings.ToLower(strings.ReplaceAll(name, " ", "")))

	// Set the ID
	e.EventID = fmt.Sprintf("%d-%s", year, safeSlug)
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
	CalendarYear       string `json:"calendarYear" gorm:"uniqueIndex"`
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
	Year             string `json:"year" gorm:"uniqueIndex"`
	RegistrationOpen bool   `json:"registrationOpen"`
	BracketUrl       string `json:"bracketUrl"`
	ShopifyUrl       string `json:"shopifyUrl"`
}

type MatchPlayMatch struct {
	gorm.Model
	Year     string `gorm:"index"`
	Round    string `json:"round"` // e.g. "Round of 32", "Quarterfinals", etc.
	Player1  string `json:"player1"`
	Player2  string `json:"player2"`
	Winner   string `json:"winner"` // Optional: empty if not yet played
	Score    string `json:"score"`
	MatchNum int    `json:"matchNum"` // Optional: for ordering within round
}

type ColonyCupInfo struct {
	gorm.Model
	Year        string         `json:"year" gorm:"uniqueIndex"`
	Team        datatypes.JSON `gorm:"type:json" json:"team"`
	WinningTeam bool           `json:"winningTeam"`
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
