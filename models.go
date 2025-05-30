package main

import (
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/datatypes"
	"gorm.io/gorm"
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
	Username     string
	PasswordHash string
}

type Event struct {
	gorm.Model
	Date                datatypes.Date
	Name                string `json:"name" gorm:"index"`
	Course              string `json:"course"`
	Town                string `json:"town"`
	State               string `json:"state"`
	HandicapAllowance   string `json:"handicapAllowance"`
	BlueGolfUrl         string `json:"blueGolfUrl"`
	Thumbnail           string `json:"thumbnail"`
	RegistrationOpen    bool   `json:"registrationOpen"`
	IsComplete          bool   `json:"isComplete"`
	NetLeaderboardUrl   string `json:"netLeaderboardUrl"`
	GrossLeaderboardUrl string `json:"grossLeaderboardUrl"`
	SkinsLeaderboardUrl string `json:"skinsLeaderboardUrl"`
	TeamsLeaderboardUrl string `json:"teamsLeaderboardUrl"`
	WgrLeaderboardUrl   string `json:"wgrLeaderboardUrl"`
}

type Standings struct {
	gorm.Model
	SeasonStandingsUrl string `json:"seasonStandingsUrl"`
	WgrStandingsUrl    string `json:"wgrStandingsUrl"`
}

type DisabledGolfer struct {
	gorm.Model
	Name     string `json:"name"`
	Reason   string `json:"reason"`
	Duration string `json:"duration"`
}

type CalendarYear struct {
	gorm.Model
	CurrentYear string `json:"currentYear"`
}

type MatchPlayInfo struct {
	gorm.Model
	RegistrationOpen bool   `json:"registrationOpen"`
	BracketUrl       string `json:"bracketUrl"`
}

type ColonyCupInfo struct {
	gorm.Model
	Year        string         `json:"year"`
	WinningTeam datatypes.JSON `gorm:"type:json"`
}
