// Copyright (c) 2022 The illium developers
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.

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

	// Start a test HTTP server that serves the mock HTML
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(htmlContent)
	}))
	defer server.Close()

	err = updateStandings(&Standings{
		CalendarYear:       "2025",
		SeasonStandingsUrl: server.URL,
		WgrStandingsUrl:    server.URL,
	}, db)
	assert.NoError(t, err)

	var (
		season SeasonRank
		wgr    WGRRank
	)
	result := db.First(&season)
	assert.NoError(t, result.Error)

	result = db.First(&wgr)
	assert.NoError(t, result.Error)

	assert.Equal(t, season.Player, "Chris Roussin")
	assert.Equal(t, wgr.Player, "Chris Roussin")

	assert.Equal(t, season.Rank, "1")
	assert.Equal(t, wgr.Rank, "1")

	assert.Equal(t, season.Events, "3")
	assert.Equal(t, wgr.Events, "3")

	assert.Equal(t, season.Year, "2025")
	assert.Equal(t, wgr.Year, "2025")

	assert.Equal(t, season.Points, "193")
	assert.Equal(t, wgr.Points, "193")
}
