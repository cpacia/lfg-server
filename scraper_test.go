package main

import (
	"fmt"
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

	assert.Equal(t, seasonRankings[0].User, "croussin")
	assert.Equal(t, wgrRankings[0].User, "croussin")
}

func Test_updateFullResults(t *testing.T) {

	// 1) Build a map: request‐path → filename on disk
	files := map[string]string{
		"/net-results":   "net-results.html",
		"/gross-results": "gross-results.html",
		"/skins-results": "skins-results.html",
		"/team-results":  "team-results.html",
		"/wgr-results":   "wgr-results.html",
	}

	// 2) Create a single HTTP handler that looks up r.URL.Path in that map
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Look for an exact match. If not found, return 404.
		filename, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}

		// Read the file from testdata/
		filePath := filepath.Join("testdata", filename)
		htmlContent, err := os.ReadFile(filePath)
		if err != nil {
			// Fail the test if we can’t read the file
			http.Error(w, "could not read test file", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(htmlContent)
	})

	// 3) Spin up the httptest.Server
	server := httptest.NewServer(handler)
	defer server.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = applyMigrations(db)
	assert.NoError(t, err)

	err = updateResults(db, "2025-impact-fire-open",
		server.URL+"/net-results",
		server.URL+"/gross-results",
		server.URL+"/skins-results",
		server.URL+"/team-results",
		server.URL+"/wgr-results")

	assert.NoError(t, err)
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

	path := filepath.Join("testdata", "skins-results.html")
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

func Test_updateMatchPlayResults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = applyMigrations(db)
	assert.NoError(t, err)

	path := filepath.Join("testdata", "match-play-bracket.html")
	htmlContent, err := os.ReadFile(path)
	assert.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(htmlContent)
	}))
	defer server.Close()

	err = updateMatchPlayResults(db, "2025", server.URL)
	assert.NoError(t, err)

	var matchPlayMatches []*MatchPlayMatch
	err = db.Find(&matchPlayMatches).Error
	assert.NoError(t, err)

	assert.Len(t, matchPlayMatches, 31)
	assert.Equal(t, "2025", matchPlayMatches[1].Year, "2025")
	assert.Equal(t, "Round 1", matchPlayMatches[1].Round)
	assert.Equal(t, "Chris Pacia", matchPlayMatches[1].Player1)
	assert.Equal(t, "Scott Smith", matchPlayMatches[1].Player2)
	assert.Equal(t, "S. Smith", matchPlayMatches[1].Winner)
	assert.Equal(t, 1, matchPlayMatches[1].MatchNum)
	assert.Equal(t, "4&2", matchPlayMatches[1].Score)

	assert.Equal(t, "R. Dichard", matchPlayMatches[16].Player1)
	assert.Equal(t, "S. Smith", matchPlayMatches[16].Player2)

	assert.Equal(t, "C. Shaw", matchPlayMatches[19].Player1)
	assert.Equal(t, "S. Dowd", matchPlayMatches[19].Player2)

	assert.Equal(t, "J. Tokanel", matchPlayMatches[20].Player1)
	assert.Equal(t, "B. Ganci", matchPlayMatches[20].Player2)

	for i, m := range matchPlayMatches {
		fmt.Println("Index: ", i, "Matchnum: ", m.MatchNum, "Year: ", m.Year, "Round: ", m.Round, "Player1: ", m.Player1, "PLayer2: ", m.Player2, "Winner: ", m.Winner, "Score: ", m.Score)
	}
	fmt.Println(len(matchPlayMatches))
}

func Test_ScrapeTeeTimes(t *testing.T) {
	path := filepath.Join("testdata", "tee-times.html")
	htmlContent, err := os.ReadFile(path)
	assert.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(htmlContent)
	}))
	defer server.Close()

	teeTimes, err := ScrapeTeeTimes(server.URL)
	assert.NoError(t, err)

	assert.Len(t, teeTimes, 16)
}

func Test_ScrapeTeeTimes2(t *testing.T) {
	path := filepath.Join("testdata", "tee-times2.html")
	htmlContent, err := os.ReadFile(path)
	assert.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(htmlContent)
	}))
	defer server.Close()

	teeTimes, err := ScrapeTeeTimes(server.URL)
	assert.NoError(t, err)

	assert.Len(t, teeTimes, 7)
}

func Test_sortTeeTimes(t *testing.T) {
	teeTimes := []TeeTime{
		{Round: 2, Time: "1:51 PM"},
		{Round: 1, Time: "8:45 AM"},
		{Round: 1, Time: "1:30 PM"},
		{Round: 2, Time: "8:10 AM"},
	}

	sortTeeTimes(teeTimes)

	assert.Equal(t, "8:45 AM", teeTimes[0].Time)
	assert.Equal(t, "1:30 PM", teeTimes[1].Time)
	assert.Equal(t, "8:10 AM", teeTimes[2].Time)
	assert.Equal(t, "1:51 PM", teeTimes[3].Time)
}

func TestScrapeAndPostToServer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = applyMigrations(db)
	assert.NoError(t, err)

	serverUrl := "http://localhost:8080/api/updates"
	eventID := "2025-fox-run-open"

	standings := &Standings{
		CalendarYear:       "2025",
		SeasonStandingsUrl: "https://nhgaclub.bluegolf.com/bluegolfw/nhgaclublivefreegc25/poy/lfgchampiongolferoftheyear/index.htm",
		WgrStandingsUrl:    "https://nhgaclub.bluegolf.com/bluegolfw/nhgaclublivefreegc25/poy/lfgwgr/index.htm",
	}
	netUrl := "https://nhgaclub.bluegolf.com/bluegolfw/nhgaclublivefreegc25/event/nhgaclublivefreegc258/contest/1/leaderboard.htm"
	grossUrl := "https://nhgaclub.bluegolf.com/bluegolfw/nhgaclublivefreegc25/event/nhgaclublivefreegc258/contest/8/leaderboard.htm"
	skinsUrl := "https://nhgaclub.bluegolf.com/bluegolfw/nhgaclublivefreegc25/event/nhgaclublivefreegc258/contest/4/leaderboard.htm"
	teamsUrl := "https://nhgaclub.bluegolf.com/bluegolfw/nhgaclublivefreegc25/event/nhgaclublivefreegc258/contest/11/leaderboard.htm"
	wgrUrl := "https://nhgaclub.bluegolf.com/bluegolfw/nhgaclublivefreegc25/event/nhgaclublivefreegc258/contest/13/leaderboard.htm"

	err = ScrapeAndPostToServer(db, serverUrl, eventID, standings, netUrl, grossUrl, skinsUrl, teamsUrl, wgrUrl)
	assert.NoError(t, err)
}

/*func TestPrintHtml(t *testing.T) {
	// Create a new collector
	c := colly.NewCollector()

	// Set up a callback to print the raw HTML
	c.OnResponse(func(r *colly.Response) {
		fmt.Println(string(r.Body)) // Print the HTML
	})

	// Optional: log errors
	c.OnError(func(_ *colly.Response, err error) {
		log.Println("Something went wrong:", err)
	})

	// Start scraping
	err := c.Visit("https://nhgaclub.bluegolf.com/bluegolfw/nhgaclublivefreegc25/event/nhgaclublivefreegc259/contest/1/leaderboard.htm")
	if err != nil {
		log.Fatal(err)
	}
}*/

/*type player struct {
	name      string
	scorecard string
	scores    [18]int
	score     int
	through   int
}

func TestEvent_BeforeCreate(t *testing.T) {

	c := colly.NewCollector()
	c.Async = true
	url := `https://nhgaclub.bluegolf.com/bluegolfw/nhgaclublivefreegc25/event/nhgaclublivefreegc255/contest/1/leaderboard.htm`

	var players []*player

	c.OnHTML("table.table-sortable tbody#lbBody > tr", func(e *colly.HTMLElement) {
		rank := strings.TrimSpace(e.ChildText("td:nth-child(1)"))
		name := strings.TrimSpace(e.ChildText("td:nth-child(2) a span.d-none.d-md-inline"))

		playerID := strings.TrimPrefix(e.Attr("id"), "tr_")

		if name == "" || rank == "" {
			// Skip header/invalid rows
			return
		}

		base := strings.TrimSuffix(url, "leaderboard.htm")
		scorecardURL := fmt.Sprintf("%scontestant/%s/scorecard.htm", base, playerID)

		players = append(players, &player{
			name:      name,
			scorecard: scorecardURL,
		})
	})

	if err := c.Visit(url); err != nil {
		t.Fatal(err)
	}

	c.Wait()

	for _, p := range players {

		var netScores []int
		c.OnHTML("tr.scores.bg-enhanced-net", func(e *colly.HTMLElement) {

			// Guard: only use rows whose first cell is literally “Net Score”
			if strings.TrimSpace(e.ChildText("td:first-child")) != "Net Score" {
				return
			}

			// Loop over every following <td>. The first is index 0 here, so
			// td:nth-child(n+2) skips the “Net Score” label itself.
			e.ForEach("td:nth-child(n+2)", func(i int, el *colly.HTMLElement) {

				// The BlueGolf table has extra “Out”, “In”, “Total” cells after
				// hole 18; stop after we’ve collected the 18 numbers we need.
				if len(netScores) >= 19 {
					return
				}

				text := strings.TrimSpace(el.Text) // "4" or "4." (dot icon)
				text = strings.Trim(text, ".")     // drop trailing “.” if present

				if v, err := strconv.Atoi(text); err == nil {
					netScores = append(netScores, v)
				}
			})
		})

		if err := c.Visit(p.scorecard); err != nil {
			t.Fatal(err)
		}

		c.Wait()
		if len(netScores) == 0 {
			break
		}
		copy(p.scores[0:9], netScores[0:9])
		copy(p.scores[9:18], netScores[10:])
	}

	playerMap := make(map[string]*player)
	for _, p := range players {
		playerMap[p.name] = p
	}

	groups := [][]string{
		{"Dan Moriarty", "Rob  Tokanel", "Jim Tokanel"},
		{"Beth Castantini", "Connor Shaw", "Scott Benedict"},
		{"Andrew Paul", "Ricky Dichard", "David Christian", "Ben Bourque"},
		{"John Theriault", "James Wirbal", "Chris Pacia", "Noah Dubois"},
		{"Chris Roussin", "Justin Rosamilio", "Pat Blair", "Marc Graham"},
		{"Benjamin Friend", "Joseph Gagne", "Brian Ganci", "John Morin"},
		{"Curt Dutilley", "Garrett Robinson", "Scott Smith", "Robert Wenrich"},
	}
	par := [18]int{5, 4, 3, 5, 4, 4, 4, 3, 4, 4, 3, 4, 4, 3, 4, 4, 5, 4}

	for i := 1; i <= 18+7; i++ {
		for x := 0; x < 7; x++ {
			for _, name := range groups[x] {
				if i-x < 1 || i-x > 18 {
					continue
				}
				score := playerMap[name].scores[i-x-1]
				playerMap[name].score += score - par[i-x-1]
				playerMap[name].through = i - x
				//fmt.Println(name, score, i-x, playerMap[name].score, playerMap[name].scores)
			}
		}

		sort.Slice(players, func(i, j int) bool {
			return players[i].score < players[j].score
		})

		for _, p := range players {
			if p.through > 0 {
				out := fmt.Sprintf("%s, ", p.name)
				if p.score == 0 {
					out += "E, "
				} else if p.score > 0 {
					out += fmt.Sprintf("+%d, ", p.score)
				} else {
					out += fmt.Sprintf("%d, ", p.score)
				}
				if p.through == 18 {
					out += "F"
				} else {
					out += fmt.Sprintf("%d", p.through)
				}
				fmt.Println(out)
			}
		}
		fmt.Println("***********************")
		fmt.Println()
	}
}*/
