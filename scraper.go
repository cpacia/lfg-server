package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
	"gorm.io/gorm"
	"sort"
	"strconv"
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
			return err
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
			return err
		}
	}
	if skinsUrl != "" {
		err := updateSkinsResults(db, skinsUrl, eventID)
		if err != nil {
			return err
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
			return err
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
			return err
		}
	}
	return nil
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

func updateMatchPlayResults(db *gorm.DB, year string, url string) error {
	c := colly.NewCollector()
	c.Async = true

	var matches []*MatchPlayMatch

	// Trims text or returns "" if index is out of range
	getText := func(cells *goquery.Selection, idx int) string {
		if idx < 0 || idx >= cells.Length() {
			return ""
		}
		return strings.TrimSpace(cells.Eq(idx).Text())
	}

	// getPlayerName returns the full player name if wrapped in a <span>,
	// otherwise it returns the trimmed text of the cell itself.
	getPlayerName := func(cell *goquery.Selection) string {
		full := strings.TrimSpace(cell.Find("span").First().Text())
		if full != "" {
			return full
		}
		return strings.TrimSpace(cell.Text())
	}

	c.OnHTML("table.matchtree", func(e *colly.HTMLElement) {
		// 1) Gather all <tr> rows
		var rows []*goquery.Selection
		e.DOM.Find("tr").Each(func(_ int, sel *goquery.Selection) {
			rows = append(rows, sel)
		})
		if len(rows) < 2 {
			return
		}

		// 2) Read header <th> row to build col→roundLabel
		headerCells := rows[0].Find("th")
		if headerCells.Length() == 0 {
			return
		}
		roundNames := make(map[int]string, headerCells.Length())
		for j := 0; j < headerCells.Length(); j++ {
			label := getText(headerCells, j)
			if label == "" || strings.EqualFold(label, "Season long match play") {
				continue
			}
			// Each header cell j corresponds to td‐column j*2
			roundNames[j*2] = label
		}

		// 3) Build dataRows = all <tr> that have at least one <td>
		var dataRows []*goquery.Selection
		for i := 1; i < len(rows); i++ {
			if rows[i].Find("td").Length() > 0 {
				dataRows = append(dataRows, rows[i])
			}
		}
		if len(dataRows) == 0 {
			return
		}

		// 4) Iterate over columns (0, 2, 4, …) in sorted order
		var cols []int
		for col := range roundNames {
			cols = append(cols, col)
		}
		sort.Ints(cols)

		for _, col := range cols {
			roundLabel := strings.TrimSuffix(roundNames[col], " Matches")

			// ── Inner loop for this column ──
			for i := 0; i < len(dataRows); i++ {
				cells1 := dataRows[i].Find("td")
				if cells1.Length() <= col+1 {
					continue
				}

				// 1) Only treat td[col+1] as a player if it has no <a> and getPlayerName != ""
				nameCell1 := cells1.Eq(col + 1)
				if nameCell1.Find("a").Length() > 0 {
					// This is a “score” row (e.g. <a>4&2</a> or <a>Tied</a>), skip it
					continue
				}
				player1 := getPlayerName(nameCell1)
				if player1 == "" {
					continue
				}

				// 2) Read seedText at td[col] if present, for MatchNum
				seedText := getText(cells1, col)
				matchNum := 0
				if seedText != "" {
					if idx := strings.TrimSuffix(seedText, "."); idx != "" {
						if n, err := strconv.Atoi(idx); err == nil {
							matchNum = n
						}
					}
				}

				// 3) Find the partner row for player2
				var player2 string
				var partnerIdx int
				for j := i + 1; j < len(dataRows); j++ {
					candCells := dataRows[j].Find("td")
					if candCells.Length() <= col+1 {
						continue
					}
					nameCell2 := candCells.Eq(col + 1)
					if nameCell2.Find("a").Length() > 0 {
						// Skip “score” rows
						continue
					}
					candidate := getPlayerName(nameCell2)
					if candidate != "" {
						player2 = candidate
						partnerIdx = j
						break
					}
				}
				if player2 == "" {
					// No opponent found → skip
					continue
				}

				// 4) If opponent is “Bye,” player1 wins
				if strings.EqualFold(player2, "Bye") {
					matches = append(matches, &MatchPlayMatch{
						Year:     year,
						Round:    roundLabel,
						Player1:  player1,
						Player2:  player2,
						Winner:   player1,
						MatchNum: matchNum,
					})
					i = partnerIdx
					continue
				}

				// 5) Two real players: decide winner by looking at td[col+3]
				var winner string

				// Helper to get last token (last name, or “II”, etc.)
				getLastToken := func(full string) string {
					parts := strings.Fields(full)
					if len(parts) == 0 {
						return ""
					}
					return parts[len(parts)-1]
				}

				// Look at the next‐round name cell for player1’s row (col+3)
				nextCellA := cells1.Eq(col + 3)
				nextNameA := ""
				if nextCellA.Find("a").Length() == 0 {
					nextNameA = getPlayerName(nextCellA)
				}
				if nextNameA != "" {
					tokenA := getLastToken(nextNameA)
					if tokenA == getLastToken(player1) {
						winner = player1
					} else if tokenA == getLastToken(player2) {
						winner = player2
					}
				}

				// If no winner yet, check partner’s next‐round cell
				if winner == "" {
					candCells := dataRows[partnerIdx].Find("td")
					if candCells.Length() > col+3 {
						nextCellB := candCells.Eq(col + 3)
						if nextCellB.Find("a").Length() == 0 {
							nextNameB := getPlayerName(nextCellB)
							if nextNameB != "" {
								tokenB := getLastToken(nextNameB)
								if tokenB == getLastToken(player1) {
									winner = player1
								} else if tokenB == getLastToken(player2) {
									winner = player2
								}
							}
						}
					}
				}

				// 6) Append match record
				matches = append(matches, &MatchPlayMatch{
					Year:     year,
					Round:    roundLabel,
					Player1:  player1,
					Player2:  player2,
					Winner:   winner,
					MatchNum: matchNum,
				})

				// 7) Skip the partner row
				i = partnerIdx
			}
			// ── End of inner loop ──
		}
	})

	// Visit and wait
	if err := c.Visit(url); err != nil {
		return err
	}
	c.Wait()

	// Bulk‐insert: delete old for this year, then create new
	if len(matches) > 0 {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("year = ?", year).Delete(&MatchPlayMatch{}).Error; err != nil {
				return err
			}
			return tx.Create(&matches).Error
		})
	}
	return nil
}
