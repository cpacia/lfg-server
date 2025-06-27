// Copyright (c) 2024 The illium developers
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"github.com/gocolly/colly"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// standings API response model (only the fields we need)
type standingsResp struct {
	CalendarYear string `json:"calendarYear"`
	Season       []struct {
		Player string `json:"player"`
		Events string `json:"events"`
		Points string `json:"points"`
	} `json:"season"`
}

func CreateField(url string) ([]*Player, error) {
	base := strings.TrimSuffix(url, "leaderboard.htm")

	c := colly.NewCollector(colly.Async(true))
	var (
		mu      sync.Mutex // protects players slice
		wg      sync.WaitGroup
		players []*Player
	)

	// --- first pass: leaderboard rows ---------------------------------
	c.OnHTML("table.table-sortable tbody#lbBody > tr", func(e *colly.HTMLElement) {
		name := strings.TrimSpace(
			e.ChildText("td:nth-child(2) a span.d-none.d-md-inline"),
		)
		if name == "" { // skip spacer / header rows
			return
		}
		playerID := strings.TrimPrefix(e.Attr("id"), "tr_")
		handicapURL := fmt.Sprintf(
			"%splayer/%s/gohandicap.htm",
			base, playerID,
		)

		p := &Player{Name: name}
		mu.Lock()
		players = append(players, p)
		mu.Unlock()

		// --- second pass: that player’s handicap page -----------------
		wg.Add(1)
		hc := c.Clone()

		hc.OnScraped(func(_ *colly.Response) {
			wg.Done() // guarantees Done() exactly once per request
		})

		hc.OnError(func(*colly.Response, error) {
			wg.Done() // failure path (4xx/5xx, bad URL, timeout…)
		})

		hc.OnHTML("body", func(h *colly.HTMLElement) {
			// 1) current index
			if idxTxt := h.ChildText("div.d-inline-flex strong"); idxTxt != "" {
				if idx, err := strconv.ParseFloat(strings.TrimSpace(idxTxt), 32); err == nil {
					p.HandicapIndex = float32(idx)
				}
			}

			// 2) most-recent ≤20 differentials
			h.ForEach("table#hcapdata tbody tr", func(i int, row *colly.HTMLElement) {
				if i >= 20 {
					return
				}
				// primary source is data-sort-value; fall back to inner text
				diffStr := row.ChildAttr("td.bg-dif-col", "data-sort-value")
				if diffStr == "" {
					diffStr = row.ChildText("td.bg-dif-col span.bg-dif")
				}
				if diff, err := strconv.ParseFloat(strings.TrimSpace(diffStr), 32); err == nil {
					p.Differentials = append(p.Differentials, float32(diff))
				}
			})
		})

		_ = hc.Visit(handicapURL) // ignore visit error here; handle globally below
	})

	if err := c.Visit(url); err != nil {
		return nil, err
	}
	c.Wait()  // wait for leaderboard crawl
	wg.Wait() // wait for all handicap pages

	if err := attachEventsAndPoints(players,
		"https://lfg-server-production.up.railway.app/api/standings",
	); err != nil {
		return nil, err
	}

	return players, nil
}

// attachEventsAndPoints fills EventsPlayed & PointsPerEvent in-place.
func attachEventsAndPoints(players []*Player, apiURL string) error {
	// Download & decode JSON
	resp, err := http.Get(apiURL)
	if err != nil {
		return fmt.Errorf("standings GET: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read standings: %w", err)
	}

	var payload standingsResp
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("decode standings JSON: %w", err)
	}

	// Build name → *Player map for quick lookup (case-insensitive)
	m := make(map[string]*Player, len(players))
	for _, p := range players {
		m[strings.ToLower(p.Name)] = p
	}

	// Walk through season array
	for _, row := range payload.Season {
		nameKey := strings.ToLower(strings.TrimSpace(row.Player))
		target, ok := m[nameKey]
		if !ok {
			continue // player not on current leaderboard scrape
		}

		ev, _ := strconv.Atoi(row.Events)
		pts, _ := strconv.ParseFloat(row.Points, 32)

		target.EventsPlayed = ev
		if ev > 0 {
			target.PointsPerEvent = float32(pts) / float32(ev)
		}
	}
	return nil
}
