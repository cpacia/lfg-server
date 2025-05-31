// Copyright (c) 2022 The illium developers
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestServer_POSTChangePasswordHandler(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	db.AutoMigrate(&SeasonRank{})
	db.AutoMigrate(&WGRRank{})

	err = updateStandings(&Standings{
		CalendarYear:       "2025",
		SeasonStandingsUrl: "https://nhgaclub.bluegolf.com/bluegolfw/nhgaclublivefreegc25/poy/lfgchampiongolferoftheyear/index.htm",
		WgrStandingsUrl:    "https://nhgaclub.bluegolf.com/bluegolfw/nhgaclublivefreegc25/poy/lfgwgr/index.htm",
	}, db)
	fmt.Println(err)
}
