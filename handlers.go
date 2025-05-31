package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) POSTLoginHandler(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	dbCreds := &DBCredentials{}
	result := s.db.First(dbCreds, "username = ?", creds.Username)
	if result.Error != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	err := bcrypt.CompareHashAndPassword([]byte(dbCreds.PasswordHash), []byte(creds.Password))
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	expiration := time.Now().Add(15 * time.Minute)
	claims := &Claims{
		Username: creds.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiration),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(jwtKey)
	if err != nil {
		http.Error(w, "Could not generate token", http.StatusInternalServerError)
		return
	}

	// Set HTTP-only JWT cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    tokenStr,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) POSTChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(userContextKey).(*Claims)
	if !ok || claims == nil {
		http.Error(w, "User info not found in context", http.StatusInternalServerError)
		return
	}

	var pwChangeReq PWChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&pwChangeReq); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	dbCreds := &DBCredentials{}
	result := s.db.First(dbCreds, "username = ?", claims.Username)
	if result.Error != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	err := bcrypt.CompareHashAndPassword([]byte(dbCreds.PasswordHash), []byte(pwChangeReq.CurrentPassword))
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(pwChangeReq.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Could not check password", http.StatusInternalServerError)
		return
	}
	dbCreds.PasswordHash = string(hash)
	if err := s.db.Save(&dbCreds).Error; err != nil {
		http.Error(w, "Could not save password", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) POSTStandingsUrls(w http.ResponseWriter, r *http.Request) {
	var standings Standings
	if err := json.NewDecoder(r.Body).Decode(&standings); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	dbStandings := &Standings{}
	result := s.db.First(dbStandings, "calendar_year = ?", standings.CalendarYear)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	dbStandings.CalendarYear = standings.CalendarYear
	dbStandings.SeasonStandingsUrl = standings.SeasonStandingsUrl
	dbStandings.WgrStandingsUrl = standings.WgrStandingsUrl

	if err := s.db.Save(&dbStandings).Error; err != nil {
		http.Error(w, "Could not save standings", http.StatusInternalServerError)
		return
	}
	if err := updateStandings(s.db, dbStandings); err != nil {
		http.Error(w, fmt.Sprintf("Error downloading new standings: %s", err.Error()), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) POSTRefreshStandings(w http.ResponseWriter, r *http.Request) {
	var latest Standings
	err := s.db.Order("calendar_year DESC").First(&latest).Error
	if err != nil && !!errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "Error loading standings from db", http.StatusInternalServerError)
		return
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "Standings not saved yet", http.StatusBadRequest)
		return
	}

	if err := updateStandings(s.db, &latest); err != nil {
		http.Error(w, fmt.Sprintf("Error downloading new standings: %s", err.Error()), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) POSTEvent(w http.ResponseWriter, r *http.Request) {
	var event Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	result := s.db.Create(&event)
	if result.Error != nil {
		http.Error(w, fmt.Sprintf("Error saving new event: %s", result.Error.Error()), http.StatusBadRequest)
		return
	}

	if event.ResultsUpdated() {
		err := updateResults(s.db, event.EventID,
			event.NetLeaderboardUrl,
			event.GrossLeaderboardUrl,
			event.SkinsLeaderboardUrl,
			event.TeamsLeaderboardUrl,
			event.WgrLeaderboardUrl)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error downloading results: %s", err.Error()), http.StatusBadRequest)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) PUTEvent(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")

	// Load existing
	var existing Event
	if err := s.db.First(&existing, "event_id = ?", eventID).Error; err != nil {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}

	// Decode full object
	var updated Event
	if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	updated.EventID = existing.EventID // ensure ID is preserved

	// Compare fields
	triggerScrape := updated.NetLeaderboardUrl != existing.NetLeaderboardUrl ||
		updated.GrossLeaderboardUrl != existing.GrossLeaderboardUrl ||
		updated.SkinsLeaderboardUrl != existing.SkinsLeaderboardUrl ||
		updated.TeamsLeaderboardUrl != existing.TeamsLeaderboardUrl ||
		updated.WgrLeaderboardUrl != existing.WgrLeaderboardUrl

	// Save update
	if err := s.db.Save(&updated).Error; err != nil {
		http.Error(w, fmt.Sprintf("Update failed: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if triggerScrape {
		selectUrl := func(existing, updated string) string {
			if updated == existing {
				return ""
			}
			return updated
		}

		err := updateResults(s.db, updated.EventID,
			selectUrl(existing.NetLeaderboardUrl, updated.NetLeaderboardUrl),
			selectUrl(existing.GrossLeaderboardUrl, updated.GrossLeaderboardUrl),
			selectUrl(existing.SkinsLeaderboardUrl, updated.SkinsLeaderboardUrl),
			selectUrl(existing.TeamsLeaderboardUrl, updated.TeamsLeaderboardUrl),
			selectUrl(existing.WgrLeaderboardUrl, updated.WgrLeaderboardUrl),
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error downloading results: %s", err.Error()), http.StatusBadRequest)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) DELETEEvent(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		http.Error(w, "Missing event ID", http.StatusBadRequest)
		return
	}

	var event Event
	if err := s.db.First(&event, "event_id = ?", eventID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Event not found", http.StatusNotFound)
		} else {
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	if err := s.db.Delete(&event).Error; err != nil {
		http.Error(w, "Failed to delete event", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent) // 204 No Content
}

func (s *Server) GETEvent(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		http.Error(w, "Missing event ID", http.StatusBadRequest)
		return
	}

	var event Event
	if err := s.db.First(&event, "event_id = ?", eventID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Event not found", http.StatusNotFound)
		} else {
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(event)
}

func (s *Server) GETEvents(w http.ResponseWriter, r *http.Request) {
	var events []Event
	if err := s.db.Order("date DESC").Find(&events).Error; err != nil {
		http.Error(w, "Error fetching events", http.StatusInternalServerError)
		return
	}

	if len(events) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"calendarYear":    nil,
			"additionalYears": []int{},
			"events":          []Event{},
		})
		return
	}

	// Group events by year
	eventMap := make(map[int][]Event)
	years := make(map[int]bool)
	for _, e := range events {
		year := time.Time(e.Date).Year()
		eventMap[year] = append(eventMap[year], e)
		years[year] = true
	}

	// Determine latest year
	var allYears []int
	for y := range years {
		allYears = append(allYears, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(allYears)))

	latestYear := allYears[0]
	additionalYears := allYears[1:]

	resp := map[string]any{
		"calendarYear":    latestYear,
		"additionalYears": additionalYears,
		"events":          eventMap[latestYear],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) GETEventsByYear(w http.ResponseWriter, r *http.Request) {
	yearStr := chi.URLParam(r, "year")
	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 1900 || year > 2100 {
		http.Error(w, "Invalid year", http.StatusBadRequest)
		return
	}

	var events []Event
	if err := s.db.
		Where("strftime('%Y', date) = ?", fmt.Sprintf("%d", year)).
		Order("date ASC").
		Find(&events).Error; err != nil {
		http.Error(w, "Error fetching events", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func (s *Server) GetNetResults(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		http.Error(w, "Missing event ID", http.StatusBadRequest)
		return
	}

	// Fetch net results
	var netResults []NetResult
	if err := s.db.Where("event_id = ?", eventID).Order("rank ASC").Find(&netResults).Error; err != nil {
		http.Error(w, "Error fetching net results", http.StatusInternalServerError)
		return
	}
	sort.Slice(netResults, func(i, j int) bool {
		return parseRank(netResults[i].Rank) < parseRank(netResults[j].Rank)
	})

	// Check existence of other result types
	var grossExists, skinsExists, teamsExists, wgrExists bool

	s.db.Model(&GrossResult{}).Where("event_id = ?", eventID).Limit(1).Select("1").Scan(&grossExists)
	s.db.Model(&SkinsPlayerResult{}).Where("event_id = ?", eventID).Limit(1).Select("1").Scan(&skinsExists)
	s.db.Model(&TeamResult{}).Where("event_id = ?", eventID).Limit(1).Select("1").Scan(&teamsExists)
	s.db.Model(&WGRResult{}).Where("event_id = ?", eventID).Limit(1).Select("1").Scan(&wgrExists)

	resp := map[string]any{
		"hasGross": grossExists,
		"hasSkins": skinsExists,
		"hasTeams": teamsExists,
		"hasWgr":   wgrExists,
		"results":  netResults,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) GetGrossResults(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		http.Error(w, "Missing event ID", http.StatusBadRequest)
		return
	}

	var results []GrossResult
	if err := s.db.Where("event_id = ?", eventID).Order("rank ASC").Find(&results).Error; err != nil {
		http.Error(w, "Error fetching gross results", http.StatusInternalServerError)
		return
	}
	sort.Slice(results, func(i, j int) bool {
		return parseRank(results[i].Rank) < parseRank(results[j].Rank)
	})

	json.NewEncoder(w).Encode(results)
}

func (s *Server) GetSkinsResults(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		http.Error(w, "Missing event ID", http.StatusBadRequest)
		return
	}

	var players []SkinsPlayerResult
	var holes []SkinsHolesResult

	if err := s.db.Where("event_id = ?", eventID).Order("rank ASC").Find(&players).Error; err != nil {
		http.Error(w, "Error fetching skins player results", http.StatusInternalServerError)
		return
	}
	if err := s.db.Where("event_id = ?", eventID).Order("hole ASC").Find(&holes).Error; err != nil {
		http.Error(w, "Error fetching skins hole results", http.StatusInternalServerError)
		return
	}
	sort.Slice(players, func(i, j int) bool {
		return parseRank(players[i].Rank) < parseRank(players[j].Rank)
	})

	resp := map[string]any{
		"players": players,
		"holes":   holes,
	}
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) GetTeamResults(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		http.Error(w, "Missing event ID", http.StatusBadRequest)
		return
	}

	var results []TeamResult
	if err := s.db.Where("event_id = ?", eventID).Order("rank ASC").Find(&results).Error; err != nil {
		http.Error(w, "Error fetching team results", http.StatusInternalServerError)
		return
	}
	sort.Slice(results, func(i, j int) bool {
		return parseRank(results[i].Rank) < parseRank(results[j].Rank)
	})

	json.NewEncoder(w).Encode(results)
}

func (s *Server) GetWgrResults(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		http.Error(w, "Missing event ID", http.StatusBadRequest)
		return
	}

	var results []WGRResult
	if err := s.db.Where("event_id = ?", eventID).Order("rank ASC").Find(&results).Error; err != nil {
		http.Error(w, "Error fetching WGR results", http.StatusInternalServerError)
		return
	}
	sort.Slice(results, func(i, j int) bool {
		return parseRank(results[i].Rank) < parseRank(results[j].Rank)
	})

	json.NewEncoder(w).Encode(results)
}

func parseRank(r string) int {
	r = strings.TrimPrefix(r, "T")
	n, err := strconv.Atoi(r)
	if err != nil {
		return 9999 // fallback to bottom if unparsable
	}
	return n
}
