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
	var strconvErr error

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

	if strconvErr != nil {
		return strconvErr
	}

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
