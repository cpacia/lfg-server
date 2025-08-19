package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
	"gorm.io/gorm"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func updateStandings(db *gorm.DB, s *Standings) error {
	if s.SeasonStandingsUrl != "" {
		err := updateStandingsGeneric(db, s.SeasonStandingsUrl, s.CalendarYear, func(year, player, rank, events, points, user string) *SeasonRank {
			return &SeasonRank{
				Year:   year,
				Player: player,
				Rank:   rank,
				Events: events,
				Points: points,
				User:   user,
			}
		})
		if err != nil {
			return err
		}
	}
	if s.WgrStandingsUrl != "" {
		err := updateStandingsGeneric(db, s.WgrStandingsUrl, s.CalendarYear, func(year, player, rank, events, points, user string) *WGRRank {
			return &WGRRank{
				Year:   year,
				Player: player,
				Rank:   rank,
				Events: events,
				Points: points,
				User:   user,
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

func updateStandingsGeneric[T any](db *gorm.DB, url string, year string, newRow func(string, string, string, string, string, string) T) error {
	c := colly.NewCollector(
		// Optional: make it look like Chrome
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
			"AppleWebKit/537.36 (KHTML, like Gecko) " +
			"Chrome/115.0.0.0 Safari/537.36"),
	)
	c.Async = true

	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9")
		r.Headers.Set("Cache-Control", "no-cache")
		// You can spoof Referer or others if needed:
		// r.Headers.Set("Referer", "https://www.google.com/")
		fmt.Println("Visiting", r.URL.String())
	})

	rows := make([]T, 0, 30)
	c.OnHTML("table.table-sortable tbody > tr", func(e *colly.HTMLElement) {
		tds := e.DOM.ChildrenFiltered("td")
		if tds.Length() != 4 {
			// This is not one of our four‑column rows—skip it.
			return
		}

		user := strings.TrimSpace(e.Attr("data-user"))
		rank := strings.TrimSpace(tds.Eq(0).Text())
		player := strings.TrimSpace(tds.Eq(1).Find(".plr-data a").Text())
		events := strings.TrimSpace(tds.Eq(2).Text())
		points := strings.TrimSpace(tds.Eq(3).Text())

		parts := strings.SplitN(points, ".", 2)
		integerPoints := parts[0]
		rows = append(rows, newRow(year, player, rank, events, integerPoints, user))
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
	c := colly.NewCollector(
		// Optional: make it look like Chrome
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
			"AppleWebKit/537.36 (KHTML, like Gecko) " +
			"Chrome/115.0.0.0 Safari/537.36"),
	)
	c.Async = true

	rows := make([]T, 0, 30)

	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9")
		r.Headers.Set("Cache-Control", "no-cache")
		// You can spoof Referer or others if needed:
		// r.Headers.Set("Referer", "https://www.google.com/")
		fmt.Println("Visiting", r.URL.String())
	})

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

		// This is likely a stableford
		if strings.HasPrefix(points, "$") {
			strokes = "-"
			points = strings.TrimSpace(e.ChildText("td:nth-child(5)"))
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
	c := colly.NewCollector(
		// Optional: make it look like Chrome
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
			"AppleWebKit/537.36 (KHTML, like Gecko) " +
			"Chrome/115.0.0.0 Safari/537.36"),
	)
	c.Async = true

	playerRows := make([]*SkinsPlayerResult, 0, 30)
	holeRows := make([]*SkinsHolesResult, 0, 30)

	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9")
		r.Headers.Set("Cache-Control", "no-cache")
		// You can spoof Referer or others if needed:
		// r.Headers.Set("Referer", "https://www.google.com/")
		fmt.Println("Visiting", r.URL.String())
	})

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
	c := colly.NewCollector(
		// Optional: make it look like Chrome
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
			"AppleWebKit/537.36 (KHTML, like Gecko) " +
			"Chrome/115.0.0.0 Safari/537.36"),
	)
	c.Async = true

	var matches []*MatchPlayMatch

	// Trims text or returns "" if idx is out of range
	getText := func(cells *goquery.Selection, idx int) string {
		if idx < 0 || idx >= cells.Length() {
			return ""
		}
		return strings.TrimSpace(cells.Eq(idx).Text())
	}

	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9")
		r.Headers.Set("Cache-Control", "no-cache")
		// You can spoof Referer or others if needed:
		// r.Headers.Set("Referer", "https://www.google.com/")
		fmt.Println("Visiting", r.URL.String())
	})

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

		// Round 1
		matchNum := 0
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
		}

		// Round 2
		matchNum = 0
		for i := 0; i+2 < len(dataRows); i += 4 {
			player1 := dataRows[i].
				Find("td").Eq(3). // fourth <td>
				Text()

			player2 := dataRows[i+2].
				Find("td").Eq(3). // fourth <td>
				Text()

			winner := dataRows[i+1].
				Find("td").Eq(5). // fourth <td>
				Text()

			score := dataRows[i+2].
				Find("td").Eq(5).  // second <td>
				Find("a").First(). // first <span> inside it
				Text()

			if score == "Tied" {
				score = ""
			}
			score = strings.TrimPrefix(score, " ")

			matches = append(matches, &MatchPlayMatch{
				Year:     year,
				Round:    roundNames[2],
				Player1:  player1,
				Player2:  player2,
				Winner:   winner,
				Score:    score,
				MatchNum: matchNum,
			})
			matchNum++
		}

		// Quaterfinals
		matchNum = 0
		for i := 1; i+2 < len(dataRows); i += 8 {
			player1 := dataRows[i].
				Find("td").Eq(5). // fourth <td>
				Text()

			player2 := dataRows[i+4].
				Find("td").Eq(5). // fourth <td>
				Text()

			// FIXME: figure out correct index when more data is populated
			winner := dataRows[i+1].
				Find("td").Eq(7). // fourth <td>
				Text()

			// FIXME: figure out correct index when more data is populated
			score := dataRows[i+2].
				Find("td").Eq(5).  // second <td>
				Find("a").First(). // first <span> inside it
				Text()

			if score == "Tied" {
				score = ""
			}
			score = strings.TrimPrefix(score, " ")

			matches = append(matches, &MatchPlayMatch{
				Year:     year,
				Round:    roundNames[3],
				Player1:  player1,
				Player2:  player2,
				Winner:   winner,
				Score:    score,
				MatchNum: matchNum,
			})
			matchNum++
		}

		// Semifinals
		matchNum = 0
		for i := 0; i+2 < len(dataRows); i += 16 {
			// FIXME: figure out correct index when more data is populated
			player1 := dataRows[i].
				Find("td").Eq(7). // fourth <td>
				Text()

			// FIXME: figure out correct index when more data is populated
			player2 := dataRows[i+4].
				Find("td").Eq(7). // fourth <td>
				Text()

			// FIXME: figure out correct index when more data is populated
			winner := dataRows[i+1].
				Find("td").Eq(7). // fourth <td>
				Text()

			// FIXME: figure out correct index when more data is populated
			score := dataRows[i+2].
				Find("td").Eq(7).  // second <td>
				Find("a").First(). // first <span> inside it
				Text()
			fmt.Println(i, player1, player2, winner, score)

			if score == "Tied" {
				score = ""
			}
			score = strings.TrimPrefix(score, " ")

			matches = append(matches, &MatchPlayMatch{
				Year:     year,
				Round:    roundNames[4],
				Player1:  player1,
				Player2:  player2,
				Winner:   winner,
				Score:    score,
				MatchNum: matchNum,
			})
			matchNum++
		}
		// Semifinals
		matchNum = 0
		for i := 0; i+2 < len(dataRows); i += 32 {
			// FIXME: figure out correct index when more data is populated
			player1 := dataRows[i].
				Find("td").Eq(7). // fourth <td>
				Text()

			// FIXME: figure out correct index when more data is populated
			player2 := dataRows[i+4].
				Find("td").Eq(7). // fourth <td>
				Text()

			// FIXME: figure out correct index when more data is populated
			winner := dataRows[i+1].
				Find("td").Eq(7). // fourth <td>
				Text()

			// FIXME: figure out correct index when more data is populated
			score := dataRows[i+2].
				Find("td").Eq(7).  // second <td>
				Find("a").First(). // first <span> inside it
				Text()
			fmt.Println(i, player1, player2, winner, score)

			if score == "Tied" {
				score = ""
			}
			score = strings.TrimPrefix(score, " ")

			matches = append(matches, &MatchPlayMatch{
				Year:     year,
				Round:    roundNames[5],
				Player1:  player1,
				Player2:  player2,
				Winner:   winner,
				Score:    score,
				MatchNum: matchNum,
			})
			matchNum++
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

func ScrapeTeeTimes(startURL string) ([]TeeTime, error) {
	c := colly.NewCollector(
		// Optional: make it look like Chrome
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
			"AppleWebKit/537.36 (KHTML, like Gecko) " +
			"Chrome/115.0.0.0 Safari/537.36"),
	)
	c.Async = true

	var (
		out     []TeeTime
		mu      sync.Mutex
		seenURL = map[string]bool{}
		seenRow = map[string]bool{}
	)

	normalizeURL := func(raw string) string { /* ... same as before ... */ return raw }
	normalizeHole := func(h string) string { /* ... same as before ... */ return h }
	normalizeTime := func(t string) string { /* ... same as before ... */ return t }
	normalizePlayers := func(ps []string) []string { /* ... same as before ... */ return ps }

	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9")
		r.Headers.Set("Cache-Control", "no-cache")
		// You can spoof Referer or others if needed:
		// r.Headers.Set("Referer", "https://www.google.com/")
		fmt.Println("Visiting", r.URL.String())
	})

	// --- NEW: capture round per page and store in Request context
	c.OnHTML("#rndSelect", func(e *colly.HTMLElement) {
		block := e.DOM
		// collect all numbers in the block text
		txtNums := map[int]bool{}
		numRe := regexp.MustCompile(`\d+`)
		for _, s := range numRe.FindAllString(block.Text(), -1) {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				txtNums[n] = true
			}
		}
		// collect all numbers that are inside links (those are "other rounds")
		anchorNums := map[int]bool{}
		block.Find("a").Each(func(_ int, a *goquery.Selection) {
			if n, err := strconv.Atoi(strings.TrimSpace(a.Text())); err == nil {
				anchorNums[n] = true
			}
		})

		round := 0
		// current round is the number in the text that is NOT a link
		for n := range txtNums {
			if !anchorNums[n] {
				round = n
				break
			}
		}
		// fallback for the common two-round case: if only one link "2", current is 1, etc.
		if round == 0 && len(anchorNums) == 1 && len(txtNums) == 2 {
			for n := range anchorNums {
				if n == 1 {
					round = 2
				} else if n == 2 {
					round = 1
				}
			}
		}
		// last-resort: query params
		if round == 0 {
			if v := e.Request.URL.Query().Get("round"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					round = n
				}
			} else if v := e.Request.URL.Query().Get("rnd"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					round = n
				}
			}
		}

		e.Request.Ctx.Put("round", strconv.Itoa(round))
	})

	// ROW PARSER (unchanged except: round comes from ctx)
	c.OnHTML("table.table-bordered.table-striped.table-sm tbody > tr", func(e *colly.HTMLElement) {
		tds := e.DOM.ChildrenFiltered("td")
		if tds.Length() < 3 {
			return
		}

		round := 0
		if s := e.Request.Ctx.Get("round"); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				round = n
			}
		}

		timeText := strings.TrimSpace(tds.Eq(0).Find("span").First().Text())
		if timeText == "" {
			timeText = strings.TrimSpace(tds.Eq(0).Text())
		}
		timeText = normalizeTime(timeText)
		if timeText == "" {
			return
		}

		holeText := strings.TrimSpace(tds.Eq(1).Text())
		if holeText == "" {
			mobileHole := strings.TrimSpace(tds.Eq(0).Find(".d-md-none").Text())
			holeText = strings.TrimPrefix(mobileHole, "#")
		}
		holeText = normalizeHole(holeText)

		var players []string
		td3 := tds.Eq(2)
		td3.Find("div[id^=pairing_] td.p-0.border-0.align-middle").Each(func(_ int, s *goquery.Selection) {
			if name := strings.TrimSpace(s.Text()); name != "" {
				players = append(players, name)
			}
		})
		if len(players) == 0 {
			if summary := strings.TrimSpace(td3.ChildrenFiltered("div").First().Text()); summary != "" {
				for _, part := range strings.Split(summary, ",") {
					if name := strings.TrimSpace(part); name != "" {
						players = append(players, name)
					}
				}
			}
		}
		players = normalizePlayers(players)

		key := timeText + "|" + holeText + "|" + strings.Join(players, ",")
		mu.Lock()
		if !seenRow[key] {
			seenRow[key] = true
			out = append(out, TeeTime{Round: round, Time: timeText, Hole: holeText, Players: players})
		}
		mu.Unlock()
	})

	// FOLLOW OTHER ROUND(S) (same logic as your last version)
	c.OnHTML("#rndSelect a[href], .round-select a[href], a[href*='round='], a[href*='rnd=']", func(e *colly.HTMLElement) {
		href := strings.TrimSpace(e.Attr("href"))
		if href == "" {
			return
		}
		next := normalizeURL(e.Request.AbsoluteURL(href))
		cur := normalizeURL(e.Request.URL.String())
		if next == "" || next == cur {
			return
		}
		mu.Lock()
		if !seenURL[next] {
			seenURL[next] = true
			mu.Unlock()
			_ = c.Visit(next)
			return
		}
		mu.Unlock()
	})

	start := normalizeURL(startURL)
	seenURL[start] = true
	if err := c.Visit(start); err != nil {
		return nil, err
	}
	c.Wait()

	if len(out) == 0 {
		return nil, fmt.Errorf("no tee times parsed from URL: %s", startURL)
	}
	sortTeeTimes(out)
	return out, nil
}

func sortTeeTimes(teeTimes []TeeTime) {
	layout := "3:04 PM" // matches "8:45 AM", "1:51 PM", etc.

	sort.Slice(teeTimes, func(i, j int) bool {
		// First compare by round
		if teeTimes[i].Round != teeTimes[j].Round {
			return teeTimes[i].Round < teeTimes[j].Round
		}

		// Parse times to compare within round
		ti, err1 := time.Parse(layout, teeTimes[i].Time)
		tj, err2 := time.Parse(layout, teeTimes[j].Time)

		// If parsing fails, just fall back to string compare
		if err1 != nil || err2 != nil {
			return teeTimes[i].Time < teeTimes[j].Time
		}

		return ti.Before(tj)
	})
}
