package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
	"gorm.io/gorm"
	"sort"
	"strings"
)

func updateStandings(db *gorm.DB, s *Standings) error {
	if s.SeasonStandingsUrl != "" {
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
	}
	if s.WgrStandingsUrl != "" {
		err := updateStandingsGeneric(db, s.WgrStandingsUrl, s.CalendarYear, func(year, player, rank, events, points string) *WGRRank {
			return &WGRRank{
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
	}
	return nil
}

func updateResults(db *gorm.DB, eventID, netUrl, grossUrl, skinsUrl, teamsUrl, wgrUrl string) error {
	var rerr error
	if netUrl != "" {
		err := updateResultsGeneric(db, netUrl, eventID, func(eventID, rank, player, total, strokes, points, scorecardUrl string) *NetResult {
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
		if err != nil {
			rerr = err
		}
	}
	if grossUrl != "" {
		err := updateResultsGeneric(db, grossUrl, eventID, func(eventID, rank, player, total, strokes, points, scorecardUrl string) *GrossResult {
			return &GrossResult{
				EventID:      eventID,
				Rank:         rank,
				Player:       player,
				Total:        total,
				Strokes:      strokes,
				ScorecardUrl: scorecardUrl,
			}
		})
		if err != nil {
			rerr = err
		}
	}
	if skinsUrl != "" {
		err := updateSkinsResults(db, skinsUrl, eventID)
		if err != nil {
			rerr = err
		}
	}
	if teamsUrl != "" {
		err := updateResultsGeneric(db, teamsUrl, eventID, func(eventID, rank, team, total, strokes, points, scorecardUrl string) *TeamResult {
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
		if err != nil {
			rerr = err
		}
	}
	if wgrUrl != "" {
		err := updateResultsGeneric(db, wgrUrl, eventID, func(eventID, rank, player, total, strokes, points, scorecardUrl string) *WGRResult {
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
		if err != nil {
			rerr = err
		}
	}
	return rerr
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

		// Check if this is multi-round tournament
		if player == "" {
			player = strings.TrimSpace(e.ChildText("td:nth-child(3) a span.d-none.d-md-inline"))
			total = strings.TrimSpace(e.ChildText("td:nth-child(4)"))
			strokes = strings.TrimSpace(e.ChildText("td:nth-child(9)"))
			points = strings.TrimSpace(e.ChildText("td:nth-child(10)"))
		}

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

		skinsMultiRound := strings.TrimSpace(e.ChildText("td:nth-child(7)"))
		if skinsMultiRound != "" {
			skins = skinsMultiRound
		}

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

func updateMatchPlayResults(db *gorm.DB, year string, url string) error {
	c := colly.NewCollector()
	c.Async = true

	var matches []*MatchPlayMatch

	// Trims text or returns "" if idx is out of range
	getText := func(cells *goquery.Selection, idx int) string {
		if idx < 0 || idx >= cells.Length() {
			return ""
		}
		return strings.TrimSpace(cells.Eq(idx).Text())
	}

	c.OnHTML("table.matchtree", func(e *colly.HTMLElement) {
		var rows []*goquery.Selection
		e.DOM.Find("tr").Each(func(_ int, sel *goquery.Selection) {
			rows = append(rows, sel)
		})

		if len(rows) < 2 {
			return
		}

		// 2) Read header <th> row (rows[0]) to build map[colIndex]→roundLabel
		headerCells := rows[0].Find("th")
		if headerCells.Length() == 0 {
			return
		}
		roundNames := make(map[int]string, headerCells.Length())
		for j := 0; j < headerCells.Length(); j++ {
			label := getText(headerCells, j)
			if label == "" || strings.EqualFold(label, "Season long match play") {
				if strings.EqualFold(label, "Season long match play") {
					roundNames[j+1] = "Champion"
				}
				continue
			}
			label = strings.TrimSuffix(label, " Matches")
			// Each header cell j corresponds to td-column j*2
			roundNames[j+1] = label
		}

		var cols []int
		for col := range roundNames {
			cols = append(cols, col)
		}
		sort.Ints(cols)

		// 3) Build dataRows = all <tr> that have ≥1 <td>
		// Every two rows has data we want with a separator row in between.
		var dataRows []*goquery.Selection
		for i := 2; i < len(rows); i += 3 {
			if rows[i].Find("td").Length() > 0 {
				dataRows = append(dataRows, rows[i])
			}
			if rows[i+1].Find("td").Length() > 0 {
				dataRows = append(dataRows, rows[i+1])
			}
		}
		if len(dataRows) == 0 {
			return
		}

		// Iterate over the dataRows to get the first round match ups.
		// The first rows have a different format than the rest.
		matchNum := 0
		var nextRows []*goquery.Selection
		for i := 0; i+1 < len(dataRows); i += 2 {
			player1 := dataRows[i].
				Find("td").Eq(1).     // second <td>
				Find("span").First(). // first <span> inside it
				Text()

			player2 := dataRows[i+1].
				Find("td").Eq(1).     // second <td>
				Find("span").First(). // first <span> inside it
				Text()

			winner := dataRows[i].
				Find("td").Eq(3). // fourth <td>
				Text()

			score := dataRows[i+1].
				Find("td").Eq(3).  // second <td>
				Find("a").First(). // first <span> inside it
				Text()

			if score == "Tied" {
				score = ""
			}
			score = strings.TrimPrefix(score, " ")
			match := &MatchPlayMatch{
				Year:     year,
				Round:    roundNames[1],
				Player1:  player1,
				Player2:  player2,
				MatchNum: matchNum,
				Winner:   winner,
				Score:    score,
			}
			matches = append(matches, match)
			matchNum++

			nextRows = append(nextRows, dataRows[i])
		}

		// Now we're going to iterate over the rest of the columns.
		// The next round data should be found as follows:
		// Round2: indexes 0, 2, 4, 6, 8, 10, 12, 14
		// Round3: indexes 0, 4, 8, 12
		// Round4: indexes 0, 8
		// etc
		n := 0
		for _, col := range cols[1:] {
			rowsCopy := make([]*goquery.Selection, len(nextRows))
			copy(rowsCopy, nextRows)
			nextRows = make([]*goquery.Selection, 0, len(rowsCopy)/2)

			matchNum = 0
			for i := 0; i+1 < len(rowsCopy); i += 2 {
				player1 := rowsCopy[i].
					Find("td").Eq(3 + n). // fourth <td>
					Text()

				player2 := rowsCopy[i+1].
					Find("td").Eq(3 + n). // fourth <td>
					Text()

				// FIXME: The winner of round two is found in datarows[i+1] not rowscopy.
				// This hack might work for round two winners but probably not other rounds.
				winner := dataRows[i+1].
					Find("td").Eq(5 + n). // fourth <td>
					Text()

				score := rowsCopy[i+1].
					Find("td").Eq(5 + n). // second <td>
					Find("a").First().    // first <span> inside it
					Text()

				if score == "Tied" {
					score = ""
				}
				score = strings.TrimPrefix(score, " ")

				matches = append(matches, &MatchPlayMatch{
					Year:     year,
					Round:    roundNames[col],
					Player1:  player1,
					Player2:  player2,
					Winner:   winner,
					Score:    score,
					MatchNum: matchNum,
				})
				nextRows = append(nextRows, rowsCopy[i])
				matchNum++
			}
			n += 2
		}
	})

	// Visit URL and wait for scraping
	if err := c.Visit(url); err != nil {
		return err
	}
	c.Wait()

	// Bulk-insert: delete old for this year, then create new
	if len(matches) > 0 {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Unscoped().Where("year = ?", year).Delete(&MatchPlayMatch{}).Error; err != nil {
				return err
			}
			return tx.Create(&matches).Error
		})
	}
	return nil
}
