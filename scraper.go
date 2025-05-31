package main

import (
	"fmt"
	"github.com/gocolly/colly"
	"gorm.io/gorm"
	"strings"
)

func updateStandings(s *Standings, db *gorm.DB) error {
	err := updateStandingsGeneric(db, s.SeasonStandingsUrl, s.CalendarYear, func(year, player, rank, events, points string) *SeasonRank {
		return &SeasonRank{
			Year:   year,
			Player: player,
			Rank:   rank,
			Events: events,
			Points: points,
		}
	})
	if err != nil {
		return err
	}
	return updateStandingsGeneric(db, s.WgrStandingsUrl, s.CalendarYear, func(year, player, rank, events, points string) *WGRRank {
		return &WGRRank{
			Year:   year,
			Player: player,
			Rank:   rank,
			Events: events,
			Points: points,
		}
	})
}

func updateStandingsGeneric[T any](db *gorm.DB, url string, year string, newRow func(string, string, string, string, string) T) error {
	c := colly.NewCollector()
	c.Async = true

	rows := make([]T, 0, 30)
	c.OnHTML("table.table-sortable tbody > tr", func(e *colly.HTMLElement) {
		tds := e.DOM.ChildrenFiltered("td")
		if tds.Length() != 4 {
			// This is not one of our four‑column rows—skip it.
			return
		}

		rank := strings.TrimSpace(tds.Eq(0).Text())
		player := strings.TrimSpace(tds.Eq(1).Find(".plr-data a").Text())
		events := strings.TrimSpace(tds.Eq(2).Text())
		points := strings.TrimSpace(tds.Eq(3).Text())

		parts := strings.SplitN(points, ".", 2)
		integerPoints := parts[0]
		rows = append(rows, newRow(year, player, rank, events, integerPoints))
	})

	if err := c.Visit(url); err != nil {
		return err
	}
	c.Wait()

	if len(rows) == 0 {
		return fmt.Errorf("no rows parsed from URL: %s", url)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// We use the first element to determine the table
		var zero T
		if err := tx.Where("year = ?", year).Delete(&zero).Error; err != nil {
			return err
		}
		if err := tx.Create(&rows).Error; err != nil {
			return err
		}
		return nil
	})
}

func updateResultsGeneric[T any](db *gorm.DB, url string, eventID string, newRow func(string, string, string, string, string, string, string) T) error {
	c := colly.NewCollector()
	c.Async = true

	rows := make([]T, 0, 30)

	c.OnHTML("table.table-sortable tbody#lbBody > tr", func(e *colly.HTMLElement) {
		rank := strings.TrimSpace(e.ChildText("td:nth-child(1)"))
		player := strings.TrimSpace(e.ChildText("td:nth-child(2) a span.d-none.d-md-inline"))
		// td:nth-child(3) is "thru", not used here
		total := strings.TrimSpace(e.ChildText("td:nth-child(4)"))
		strokes := strings.TrimSpace(e.ChildText("td:nth-child(5)"))
		points := strings.TrimSpace(e.ChildText("td:nth-child(6)"))
		playerID := strings.TrimPrefix(e.Attr("id"), "tr_")

		if player == "" || rank == "" {
			// Skip header/invalid rows
			return
		}

		base := strings.TrimSuffix(url, "leaderboard.htm")
		scorecardURL := fmt.Sprintf("%scontestant/%s/scorecard.htm", base, playerID)
		
		parts := strings.SplitN(points, ".", 2)
		integerPoints := parts[0]
		rows = append(rows, newRow(eventID, rank, player, total, strokes, integerPoints, scorecardURL))
	})

	if err := c.Visit(url); err != nil {
		return err
	}
	c.Wait()

	if len(rows) == 0 {
		return fmt.Errorf("no rows parsed from URL: %s", url)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// We use the first element to determine the table
		var zero T
		if err := tx.Where("event_id = ?", eventID).Delete(&zero).Error; err != nil {
			return err
		}
		if err := tx.Create(&rows).Error; err != nil {
			return err
		}
		return nil
	})
}

func updateSkinsResults(db *gorm.DB, url string, eventID string) error {
	c := colly.NewCollector()
	c.Async = true

	playerRows := make([]*SkinsPlayerResult, 0, 30)
	holeRows := make([]*SkinsHolesResult, 0, 30)

	// Scrape player results
	c.OnHTML("tbody#lbBody > tr", func(e *colly.HTMLElement) {
		// Rank is in the first <td>
		rank := strings.TrimSpace(e.ChildText("td:nth-child(1)"))
		// Full name is in the second <td> → <a> → <span class="d-none d-md-inline">
		player := strings.TrimSpace(e.ChildText("td:nth-child(2) a span.d-none.d-md-inline"))
		// “Thru” is td:nth-child(3) but we don’t need it here
		skins := strings.TrimSpace(e.ChildText("td:nth-child(4)"))
		// ID is in the row’s id attribute (“tr_{playerID}”)
		playerID := strings.TrimPrefix(e.Attr("id"), "tr_")

		if player == "" || rank == "" {
			return
		}

		base := strings.TrimSuffix(url, "leaderboard.htm")
		scorecardURL := fmt.Sprintf("%scontestant/%s/scorecard.htm", base, playerID)

		playerRows = append(playerRows, &SkinsPlayerResult{
			EventID:      eventID,
			Rank:         rank,
			Player:       player,
			Skins:        skins,
			ScorecardUrl: scorecardURL,
		})
	})

	// Scrape hole-by-hole results
	c.OnHTML("table.table-bordered.table-striped.table-sm.lb tbody > tr", func(e *colly.HTMLElement) {
		hole := strings.TrimSpace(e.ChildText("td:nth-child(1)"))
		// “Par” lives in td:nth-child(2) but uses “d-none d-md-table-cell” styling
		par := strings.TrimSpace(e.ChildText("td:nth-child(2)"))
		score := strings.TrimSpace(e.ChildText("td:nth-child(3)"))
		// “Potential skin” (winner) lives in the 4th <td>
		winner := strings.TrimSpace(e.ChildText("td:nth-child(4) span.d-none.d-md-inline"))
		// “Tie” names live in the 5th <td>
		ties := strings.TrimSpace(e.ChildText("td:nth-child(5) span.d-none.d-md-inline"))

		// Skip a header or any row that doesn’t have actual data
		if hole == "" || score == "" {
			return
		}

		holeRows = append(holeRows, &SkinsHolesResult{
			EventID: eventID,
			Hole:    hole,
			Par:     par,
			Score:   score,
			Won:     winner,
			Tie:     ties,
		})
	})

	// Run the collector
	if err := c.Visit(url); err != nil {
		return err
	}
	c.Wait()

	// Save to DB
	if len(playerRows) > 0 {
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("event_id = ?", eventID).Delete(&SkinsPlayerResult{}).Error; err != nil {
				return err
			}
			return tx.Create(&playerRows).Error
		}); err != nil {
			return err
		}
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("event_id = ?", eventID).Delete(&SkinsHolesResult{}).Error; err != nil {
			return err
		}
		return tx.Create(&holeRows).Error
	})
}
