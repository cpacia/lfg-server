package main

import (
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func Test_updateStandings(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = applyMigrations(db)
	assert.NoError(t, err)

	path := filepath.Join("testdata", "standings.html")
	htmlContent, err := os.ReadFile(path)
	assert.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(htmlContent)
	}))
	defer server.Close()

	err = updateStandings(db, &Standings{
		CalendarYear:       "2025",
		SeasonStandingsUrl: server.URL,
		WgrStandingsUrl:    server.URL,
	})
	assert.NoError(t, err)

	var seasonRankings []*SeasonRank
	err = db.Find(&seasonRankings).Error
	assert.NoError(t, err)

	var wgrRankings []*SeasonRank
	err = db.Find(&wgrRankings).Error
	assert.NoError(t, err)

	assert.Len(t, seasonRankings, 58)
	assert.Len(t, wgrRankings, 58)

	assert.Equal(t, seasonRankings[0].Player, "Chris Roussin")
	assert.Equal(t, wgrRankings[0].Player, "Chris Roussin")

	assert.Equal(t, seasonRankings[0].Rank, "1")
	assert.Equal(t, wgrRankings[0].Rank, "1")

	assert.Equal(t, seasonRankings[0].Events, "3")
	assert.Equal(t, wgrRankings[0].Events, "3")

	assert.Equal(t, seasonRankings[0].Year, "2025")
	assert.Equal(t, wgrRankings[0].Year, "2025")

	assert.Equal(t, seasonRankings[0].Points, "193")
	assert.Equal(t, wgrRankings[0].Points, "193")
}

func Test_updateNetResults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = applyMigrations(db)
	assert.NoError(t, err)

	path := filepath.Join("testdata", "net-results.html")
	htmlContent, err := os.ReadFile(path)
	assert.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(htmlContent)
	}))
	defer server.Close()

	err = updateResultsGeneric(db, server.URL, "2025-impact-fire-open", func(eventID, rank, player, total, strokes, points, scorecardUrl string) *NetResult {
		return &NetResult{
			EventID:      eventID,
			Rank:         rank,
			Player:       player,
			Total:        total,
			Strokes:      strokes,
			Points:       points,
			ScorecardUrl: scorecardUrl,
		}
	})
	assert.NoError(t, err)

	var netResults []*NetResult
	err = db.Find(&netResults).Error
	assert.NoError(t, err)

	assert.Len(t, netResults, 38)
	assert.Equal(t, netResults[0].Player, "Connor Shaw")
	assert.Equal(t, netResults[0].Rank, "1")
	assert.Equal(t, netResults[0].Total, "+2")
	assert.Equal(t, netResults[0].Strokes, "73")
	assert.Equal(t, netResults[0].Points, "115")
}

func Test_updateGrossResults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = applyMigrations(db)
	assert.NoError(t, err)

	path := filepath.Join("testdata", "gross-results.html")
	htmlContent, err := os.ReadFile(path)
	assert.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(htmlContent)
	}))
	defer server.Close()

	err = updateResultsGeneric(db, server.URL, "2025-impact-fire-open", func(eventID, rank, player, total, strokes, points, scorecardUrl string) *GrossResult {
		return &GrossResult{
			EventID:      eventID,
			Rank:         rank,
			Player:       player,
			Total:        total,
			Strokes:      strokes,
			ScorecardUrl: scorecardUrl,
		}
	})
	assert.NoError(t, err)

	var grossResults []*GrossResult
	err = db.Find(&grossResults).Error
	assert.NoError(t, err)

	assert.Len(t, grossResults, 38)
	assert.Equal(t, grossResults[0].Player, "Andy Lee")
	assert.Equal(t, grossResults[0].Rank, "1")
	assert.Equal(t, grossResults[0].Total, "+11")
	assert.Equal(t, grossResults[0].Strokes, "82")
}

func Test_updateWGRResults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = applyMigrations(db)
	assert.NoError(t, err)

	path := filepath.Join("testdata", "wgr-results.html")
	htmlContent, err := os.ReadFile(path)
	assert.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(htmlContent)
	}))
	defer server.Close()

	err = updateResultsGeneric(db, server.URL, "2025-impact-fire-open", func(eventID, rank, player, total, strokes, points, scorecardUrl string) *WGRResult {
		return &WGRResult{
			EventID:      eventID,
			Rank:         rank,
			Player:       player,
			Total:        total,
			Strokes:      strokes,
			Points:       points,
			ScorecardUrl: scorecardUrl,
		}
	})
	assert.NoError(t, err)

	var wgrResults []*WGRResult
	err = db.Find(&wgrResults).Error
	assert.NoError(t, err)

	assert.Len(t, wgrResults, 38)
	assert.Equal(t, wgrResults[0].Player, "Andy Lee")
	assert.Equal(t, wgrResults[0].Rank, "1")
	assert.Equal(t, wgrResults[0].Total, "+8")
	assert.Equal(t, wgrResults[0].Strokes, "79")
	assert.Equal(t, wgrResults[0].Points, "299")
}

func Test_updateTeamResultsWithoutTotal(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = applyMigrations(db)
	assert.NoError(t, err)

	path := filepath.Join("testdata", "team-results.html")
	htmlContent, err := os.ReadFile(path)
	assert.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(htmlContent)
	}))
	defer server.Close()

	err = updateResultsGeneric(db, server.URL, "2025-impact-fire-open", func(eventID, rank, team, total, strokes, points, scorecardUrl string) *TeamResult {
		if strokes == "" {
			strokes = total
			total = ""
		}
		return &TeamResult{
			EventID: eventID,
			Rank:    rank,
			Team:    team,
			Total:   total,
			Strokes: strokes,
		}
	})
	assert.NoError(t, err)

	var teamResults []*TeamResult
	err = db.Find(&teamResults).Error
	assert.NoError(t, err)

	assert.Len(t, teamResults, 9)
	assert.Equal(t, teamResults[0].Team, "Bomberg/Lawrence/Shaw/Vanti")
	assert.Equal(t, teamResults[0].Rank, "1")
	assert.Equal(t, teamResults[0].Total, "")
	assert.Equal(t, teamResults[0].Strokes, "80.25")
}

func Test_updateTeamResultsWithTotal(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = applyMigrations(db)
	assert.NoError(t, err)

	path := filepath.Join("testdata", "team-results2.html")
	htmlContent, err := os.ReadFile(path)
	assert.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(htmlContent)
	}))
	defer server.Close()

	err = updateResultsGeneric(db, server.URL, "2025-impact-fire-open", func(eventID, rank, team, total, strokes, points, scorecardUrl string) *TeamResult {
		if strokes == "" {
			strokes = total
			total = ""
		}
		return &TeamResult{
			EventID: eventID,
			Rank:    rank,
			Team:    team,
			Total:   total,
			Strokes: strokes,
		}
	})
	assert.NoError(t, err)

	var teamResults []*TeamResult
	err = db.Find(&teamResults).Error
	assert.NoError(t, err)

	assert.Len(t, teamResults, 10)
	assert.Equal(t, teamResults[0].Team, "Benedict/Christian/Graham/Lawrence")
	assert.Equal(t, teamResults[0].Rank, "1")
	assert.Equal(t, teamResults[0].Total, "-8")
	assert.Equal(t, teamResults[0].Strokes, "208")
}

func Test_updateSkinsPlayers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = applyMigrations(db)
	assert.NoError(t, err)

	path := filepath.Join("testdata", "skins-result.html")
	htmlContent, err := os.ReadFile(path)
	assert.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(htmlContent)
	}))
	defer server.Close()

	err = updateSkinsResults(db, server.URL, "2025-impact-fire-open")
	assert.NoError(t, err)

	var skinPlayerResults []*SkinsPlayerResult
	err = db.Find(&skinPlayerResults).Error
	assert.NoError(t, err)

	assert.Len(t, skinPlayerResults, 7)
	assert.Equal(t, skinPlayerResults[0].Player, "Chad Lawrence")
	assert.Equal(t, skinPlayerResults[0].Rank, "1")
	assert.Equal(t, skinPlayerResults[0].Skins, "3")

	var skinHoleResults []*SkinsHolesResult
	err = db.Find(&skinHoleResults).Error
	assert.NoError(t, err)

	assert.Len(t, skinHoleResults, 18)
	assert.Equal(t, skinHoleResults[0].Hole, "1")
	assert.Equal(t, skinHoleResults[0].Par, "5")
	assert.Equal(t, skinHoleResults[0].Won, "")
	assert.Equal(t, skinHoleResults[0].Tie, "Robert Judson, Jim Tokanel, John Theriault...")
}
