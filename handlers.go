package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/context"
	"gorm.io/gorm"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func basicSanitize(input string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9 \-]+`)
	safeSlug := reg.ReplaceAllString(input, "")

	// Ensure no leading/trailing dashes
	return strings.Trim(safeSlug, "-")
}

func validateYear(yearStr string) bool {
	y, err := strconv.Atoi(yearStr)
	if err != nil {
		return false
	}
	return y >= 2000 && y <= 2100
}

func (s *Server) POSTLoginHandler(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Check if rate limit has been exceeded
	key := loginRateLimitKey(r, creds.Username)
	ctx, err := s.loginRateLimiter.Peek(r.Context(), key)
	if err != nil {
		http.Error(w, "Rate limiter error", http.StatusInternalServerError)
		return
	}
	if ctx.Reached {
		http.Error(w, "Too many failed login attempts", http.StatusTooManyRequests)
		return
	}

	dbCreds := &DBCredentials{}
	result := s.db.First(dbCreds, "username = ?", creds.Username)
	if result.Error != nil {
		s.loginRateLimiter.Increment(r.Context(), key, 2)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	err = bcrypt.CompareHashAndPassword([]byte(dbCreds.PasswordHash), []byte(creds.Password))
	if err != nil {
		s.loginRateLimiter.Increment(r.Context(), key, 2)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	expiration := time.Now().Add(60 * time.Minute)
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
		Secure:   !s.devMode,
		SameSite: http.SameSiteNoneMode,
		Path:     "/",
	})
	w.WriteHeader(http.StatusOK)
}

func loginRateLimitKey(r *http.Request, username string) string {
	ip := r.RemoteAddr
	return fmt.Sprintf("%s:%s", ip, username)
}

func (s *Server) POSTLogoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   !s.devMode,
		SameSite: http.SameSiteNoneMode,
		Expires:  time.Unix(0, 0), // Expire immediately
		MaxAge:   -1,              // Force deletion
	})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) POSTAuthMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(userContextKey).(*Claims)
	if !ok || claims == nil {
		http.Error(w, "User info not found in context", http.StatusInternalServerError)
		return
	}

	dbCreds := &DBCredentials{}
	result := s.db.First(dbCreds, "username = ?", claims.Username)
	if result.Error != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"authenticated": true,
		"username":      claims.Username,
	})

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

func (s *Server) GETStandings(w http.ResponseWriter, r *http.Request) {
	requestedYear := r.URL.Query().Get("year")

	var years []string
	if err := s.db.Model(&SeasonRank{}).
		Distinct().
		Pluck("year", &years).Error; err != nil {
		http.Error(w, "Failed to fetch years", http.StatusInternalServerError)
		return
	}

	if len(years) == 0 {
		json.NewEncoder(w).Encode("[]")
		return
	}

	// Sort descending
	sort.Sort(sort.Reverse(sort.StringSlice(years)))

	// Choose year: requested or latest
	targetYear := years[0]
	if requestedYear != "" {
		found := false
		for _, y := range years {
			if y == requestedYear {
				targetYear = requestedYear
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "Year not found", http.StatusNotFound)
			return
		}
	}

	// Build additional years list
	additionalYears := make([]string, 0, len(years)-1)
	for _, y := range years {
		if y != targetYear {
			additionalYears = append(additionalYears, y)
		}
	}

	// Load standings
	var season []SeasonRank
	if err := s.db.Where("year = ?", targetYear).Find(&season).Error; err != nil {
		http.Error(w, "Failed to load season standings", http.StatusInternalServerError)
		return
	}
	sort.Slice(season, func(i, j int) bool {
		return parseRank(season[i].Rank) < parseRank(season[j].Rank)
	})

	var wgr []WGRRank
	if err := s.db.Where("year = ?", targetYear).Find(&wgr).Error; err != nil {
		http.Error(w, "Failed to load WGR standings", http.StatusInternalServerError)
		return
	}
	sort.Slice(wgr, func(i, j int) bool {
		return parseRank(wgr[i].Rank) < parseRank(wgr[j].Rank)
	})

	// Respond
	resp := map[string]any{
		"calendarYear":    targetYear,
		"additionalYears": additionalYears,
		"season":          season,
		"wgr":             wgr,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) GETStandingsUrls(w http.ResponseWriter, r *http.Request) {
	var dbStandings []Standings
	result := s.db.Find(&dbStandings)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dbStandings)
}

func (s *Server) GETStandingsUserData(w http.ResponseWriter, r *http.Request) {
	requestedYear := r.URL.Query().Get("year")
	typ := strings.ToLower(chi.URLParam(r, "type"))
	if typ != "season" && typ != "wgr" {
		http.Error(w, "Type must be season or wgr", http.StatusBadRequest)
		return
	}
	user := chi.URLParam(r, "user")
	if user == "" {
		http.Error(w, "User must be provided", http.StatusBadRequest)
		return
	}

	var years []string
	if err := s.db.Model(&Standings{}).
		Distinct().
		Pluck("calendar_year", &years).Error; err != nil {
		http.Error(w, "Failed to fetch years", http.StatusInternalServerError)
		return
	}

	if len(years) == 0 {
		// FIXME: actual return
		json.NewEncoder(w).Encode(map[string]any{
			"calendarYear":    nil,
			"additionalYears": []string{},
			"season":          []SeasonRank{},
			"wgr":             []WGRRank{},
		})
		return
	}

	// Sort descending
	sort.Sort(sort.Reverse(sort.StringSlice(years)))

	// Choose year: requested or latest
	targetYear := years[0]
	if requestedYear != "" {
		found := false
		for _, y := range years {
			if y == requestedYear {
				targetYear = requestedYear
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "Year not found", http.StatusNotFound)
			return
		}
	}

	var standings Standings
	result := s.db.Where("calendar_year = ?", targetYear).First(&standings)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			http.Error(w, "Standings not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	var url string
	if typ == "season" {
		url = standings.SeasonStandingsUrl
	} else {
		url = standings.WgrStandingsUrl
	}
	club, contest, err := parseBlueGolf(url)
	if err != nil {
		http.Error(w, "Error parsing blue golf url", http.StatusInternalServerError)
		return
	}

	queryUrl := fmt.Sprintf("https://nhgaclub.bluegolf.com/bluegolfw/%s/profile/%s/poyprofile.json?award=%s", club, user, contest)
	primeURL := fmt.Sprintf("https://nhgaclub.bluegolf.com/bluegolfw/%s/profile/%s/poyprofile.htm?award=%s", club, user, contest)

	env, err := fetchBlueGolfJSON(r.Context(), queryUrl, primeURL)
	if err != nil {
		log.Printf("BlueGolf diag: %v", err)
		http.Error(w, "Upstream returned non-JSON", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(env.Tournaments)
}

func (s *Server) POSTStandingsUrls(w http.ResponseWriter, r *http.Request) {
	var standings Standings
	if err := json.NewDecoder(r.Body).Decode(&standings); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if !validateYear(standings.CalendarYear) {
		http.Error(w, "Malformed year", http.StatusBadRequest)
		return
	}

	if err := s.db.Create(&standings).Error; err != nil {
		http.Error(w, "Could not save standings", http.StatusInternalServerError)
		return
	}
	if err := updateStandings(s.db, &standings); err != nil {
		http.Error(w, fmt.Sprintf("Error downloading new standings: %s", err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(&standings)
}

func (s *Server) PUTStandingsUrls(w http.ResponseWriter, r *http.Request) {
	var standings Standings
	if err := json.NewDecoder(r.Body).Decode(&standings); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Fetch existing record
	dbStandings := &Standings{}
	result := s.db.First(dbStandings, "calendar_year = ?", standings.CalendarYear)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			http.Error(w, "Standings not found", http.StatusNotFound)
		} else {
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	// Update fields
	dbStandings.SeasonStandingsUrl = standings.SeasonStandingsUrl
	dbStandings.WgrStandingsUrl = standings.WgrStandingsUrl

	if err := s.db.Save(dbStandings).Error; err != nil {
		http.Error(w, "Could not update standings", http.StatusInternalServerError)
		return
	}

	if err := updateStandings(s.db, dbStandings); err != nil {
		http.Error(w, fmt.Sprintf("Error downloading new standings: %s", err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dbStandings)
}

func (s *Server) DELETEStandingsUrls(w http.ResponseWriter, r *http.Request) {
	// Decode the incoming JSON to get the calendar year to delete
	var payload struct {
		CalendarYear string `json:"calendarYear"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if !validateYear(payload.CalendarYear) {
		http.Error(w, "Malformed year", http.StatusBadRequest)
		return
	}

	// Look up the existing record
	dbStandings := &Standings{}
	result := s.db.First(dbStandings, "calendar_year = ?", payload.CalendarYear)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			http.Error(w, "Standings not found", http.StatusNotFound)
		} else {
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	// Delete the record
	if err := s.db.Unscoped().Delete(&Standings{}, "calendar_year = ?", payload.CalendarYear).Error; err != nil {
		http.Error(w, "Could not delete standings", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) POSTRefreshStandings(w http.ResponseWriter, r *http.Request) {
	var latest Standings
	err := s.db.Order("calendar_year DESC").First(&latest).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
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
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Invalid multipart form", http.StatusBadRequest)
		return
	}

	event, err := parseEventFromMultipart(r)
	if err != nil {
		http.Error(w, "Bad event JSON", http.StatusBadRequest)
		return
	}

	if event.Name == "" {
		http.Error(w, "Event name must be set", http.StatusBadRequest)
		return
	}

	// Create first to get eventID
	result := s.db.Create(&event)
	if result.Error != nil {
		http.Error(w, fmt.Sprintf("Error saving new event: %s", result.Error.Error()), http.StatusBadRequest)
		return
	}

	// Save thumbnail
	filename, err := s.saveThumbnail(r, event.EventID)
	if err != nil {
		http.Error(w, "Failed to save thumbnail", http.StatusInternalServerError)
		return
	}
	if filename != "" {
		event.Thumbnail = filename
	}

	s.db.Save(&event)

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

		var latest Standings
		err = s.db.Order("calendar_year DESC").First(&latest).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Error loading standings from db", http.StatusInternalServerError)
			return
		} else if err == nil {
			if err := updateStandings(s.db, &latest); err != nil {
				http.Error(w, fmt.Sprintf("Error downloading new standings: %s", err.Error()), http.StatusBadRequest)
				return
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated) // for POST
	json.NewEncoder(w).Encode(event)
}

func (s *Server) PUTEvent(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")

	var existing Event
	if err := s.db.First(&existing, "event_id = ?", eventID).Error; err != nil {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Invalid multipart form", http.StatusBadRequest)
		return
	}

	updated, err := parseEventFromMultipart(r)
	if err != nil {
		http.Error(w, "Bad event JSON", http.StatusBadRequest)
		return
	}
	updated.EventID = existing.EventID

	filename, err := s.saveThumbnail(r, updated.EventID)
	if err != nil {
		http.Error(w, "Failed to save thumbnail", http.StatusInternalServerError)
		return
	}
	if filename != "" {
		updated.Thumbnail = filename
	}

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

		var latest Standings
		err = s.db.Order("calendar_year DESC").First(&latest).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Error loading standings from db", http.StatusInternalServerError)
			return
		} else if err == nil {
			if err := updateStandings(s.db, &latest); err != nil {
				http.Error(w, fmt.Sprintf("Error downloading new standings: %s", err.Error()), http.StatusBadRequest)
				return
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated) // for PUT
	w.WriteHeader(http.StatusOK)
}

func parseEventFromMultipart(r *http.Request) (*Event, error) {
	jsonPart := r.FormValue("event")
	var event Event
	if err := json.Unmarshal([]byte(jsonPart), &event); err != nil {
		return nil, err
	}
	return &event, nil
}

func (s *Server) saveThumbnail(r *http.Request, eventID string) (string, error) {
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", nil // No image uploaded
		}
		return "", err
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("%s%s", eventID, ext)
	path := filepath.Join(s.imageDir, filename)

	dst, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		return "", err
	}

	return filename, nil
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

	// Delete associated results
	models := []any{
		&NetResult{},
		&GrossResult{},
		&SkinsPlayerResult{},
		&SkinsHolesResult{},
		&TeamResult{},
		&WGRResult{},
	}
	for _, model := range models {
		if err := s.db.Where("event_id = ?", eventID).Delete(model).Error; err != nil {
			http.Error(w, "Failed to delete related results", http.StatusInternalServerError)
			return
		}
	}

	// Delete thumbnail if it exists
	if event.Thumbnail != "" {
		path := filepath.Join(s.imageDir, event.Thumbnail)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			http.Error(w, fmt.Sprintf("Failed to delete thumbnail: %s", err.Error()), http.StatusInternalServerError)
			return
		}
	}

	// Delete the event record
	if err := s.db.Unscoped().Delete(&event).Error; err != nil {
		http.Error(w, "Failed to delete event", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

func (s *Server) GETEventThumbnail(w http.ResponseWriter, r *http.Request) {
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
	if event.Thumbnail == "" {
		http.Error(w, "Thumbnail not set", http.StatusNotFound)
		return
	}

	path := filepath.Join(s.imageDir, event.Thumbnail)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Thumbnail file not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error reading thumbnail", http.StatusInternalServerError)
		}
		return
	}
	defer f.Close()

	// Stat the file so we can get its mod-time.
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, "Error stating thumbnail", http.StatusInternalServerError)
		return
	}

	// Set your desired cache policy:
	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=2592000")

	// Let http.ServeContent detect content-type, emit Last-Modified,
	// honor If-Modified-Since → 304, and stream bytes.
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
}

func (s *Server) GETEvents(w http.ResponseWriter, r *http.Request) {
	requestedYear := r.URL.Query().Get("year")

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
	yearSet := make(map[int]bool)
	for _, e := range events {
		year := time.Time(e.Date).Year()
		eventMap[year] = append(eventMap[year], e)
		yearSet[year] = true
	}

	// Sort years descending
	var allYears []int
	for y := range yearSet {
		allYears = append(allYears, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(allYears)))

	// Determine target year
	targetYear := allYears[0] // default to most recent
	if requestedYear != "" {
		if y, err := strconv.Atoi(requestedYear); err == nil && yearSet[y] {
			targetYear = y
		} else {
			http.Error(w, "Requested year not found", http.StatusNotFound)
			return
		}
	}

	// Prepare additional years
	additionalYears := make([]int, 0, len(allYears)-1)
	for _, y := range allYears {
		if y != targetYear {
			additionalYears = append(additionalYears, y)
		}
	}

	// Return response
	resp := map[string]any{
		"calendarYear":    targetYear,
		"additionalYears": additionalYears,
		"events":          eventMap[targetYear],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) GETNetResults(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		http.Error(w, "Missing event ID", http.StatusBadRequest)
		return
	}

	var results []NetResult
	if err := s.db.Where("event_id = ?", eventID).Order("rank ASC").Find(&results).Error; err != nil {
		http.Error(w, "Error fetching gross results", http.StatusInternalServerError)
		return
	}
	sort.Slice(results, func(i, j int) bool {
		return parseRank(results[i].Rank) < parseRank(results[j].Rank)
	})

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) GETGrossResults(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) GETSkinsResults(w http.ResponseWriter, r *http.Request) {
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
	sort.Slice(holes, func(i, j int) bool {
		return parseHole(holes[i].Hole) < parseHole(holes[j].Hole)
	})

	resp := map[string]any{
		"players": players,
		"holes":   holes,
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) GETTeamResults(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) GETWgrResults(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) GETCurrentYear(w http.ResponseWriter, r *http.Request) {
	var latest Event
	err := s.db.Order("date DESC").Limit(1).First(&latest).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "No events found", http.StatusNotFound)
		} else {
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	year := time.Time(latest.Date).Year()

	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=2592000")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"calendarYear": year,
	})
}

func parseRank(r string) int {
	r = strings.TrimPrefix(r, "T")
	n, err := strconv.Atoi(r)
	if err != nil {
		return 9999 // fallback to bottom if unparsable
	}
	return n
}
func parseHole(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 9999 // fallback to bottom if unparsable
	}
	return n
}

func (s *Server) POSTDisabledGolfer(w http.ResponseWriter, r *http.Request) {
	var golfer DisabledGolfer
	if err := json.NewDecoder(r.Body).Decode(&golfer); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	golfer.Name = basicSanitize(golfer.Name)

	if err := s.db.Create(&golfer).Error; err != nil {
		http.Error(w, fmt.Sprintf("Error saving golfer: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(golfer)
}

func (s *Server) PUTDisabledGolfer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "Missing name", http.StatusBadRequest)
		return
	}

	var updated DisabledGolfer
	if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	updated.Name = name

	var existing DisabledGolfer
	if err := s.db.First(&existing, "name = ?", name).Error; err != nil {
		http.Error(w, "Golfer not found", http.StatusNotFound)
		return
	}

	if err := s.db.Save(&updated).Error; err != nil {
		http.Error(w, "Error updating golfer", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(updated)
}

func (s *Server) GETDisabledGolfer(w http.ResponseWriter, r *http.Request) {
	var golfers []DisabledGolfer
	if err := s.db.Find(&golfers).Error; err != nil {
		http.Error(w, "Failed to fetch disabled golfer results", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(&golfers)
}

func (s *Server) DELETEDisabledGolfer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "Missing name", http.StatusBadRequest)
		return
	}

	if err := s.db.Unscoped().Delete(&DisabledGolfer{}, "name = ?", name).Error; err != nil {
		http.Error(w, "Error deleting golfer", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) POSTColonyCupInfo(w http.ResponseWriter, r *http.Request) {
	var info ColonyCupInfo
	if err := json.NewDecoder(r.Body).Decode(&info); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if !validateYear(info.Year) {
		http.Error(w, "Malformed year", http.StatusBadRequest)
		return
	}

	if err := s.db.Create(&info).Error; err != nil {
		http.Error(w, fmt.Sprintf("Error creating entry: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(info)
}

func (s *Server) GETColonyCupInfo(w http.ResponseWriter, r *http.Request) {
	var infos []ColonyCupInfo
	if err := s.db.Order("year DESC").Limit(2).Find(&infos).Error; err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if len(infos) == 0 {
		http.Error(w, "No records found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infos)
}

func (s *Server) GETAllColonyCupInfo(w http.ResponseWriter, r *http.Request) {
	var infos []ColonyCupInfo
	if err := s.db.Order("year DESC").Find(&infos).Error; err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infos)
}

func (s *Server) PUTColonyCupInfo(w http.ResponseWriter, r *http.Request) {
	var updated ColonyCupInfo
	if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	var existing ColonyCupInfo
	if err := s.db.First(&existing).Error; err != nil {
		http.Error(w, "Record not found", http.StatusNotFound)
		return
	}
	existing.Year = updated.Year
	existing.WinningTeam = updated.WinningTeam

	if err := s.db.Save(&updated).Error; err != nil {
		http.Error(w, "Error updating record", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(updated)
}

func (s *Server) DELETEColonyCupInfo(w http.ResponseWriter, r *http.Request) {
	// Decode the incoming JSON to get the calendar year to delete
	var payload struct {
		Year string `json:"year"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Look up the existing record
	colonyCupInfo := &ColonyCupInfo{}
	result := s.db.First(colonyCupInfo, "year = ?", payload.Year)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			http.Error(w, "Colony cup year not found", http.StatusNotFound)
		} else {
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	// Delete the record
	if err := s.db.Unscoped().Delete(&ColonyCupInfo{}, "year = ?", payload.Year).Error; err != nil {
		http.Error(w, "Could not delete colony cup year", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GETMatchPlayInfo(w http.ResponseWriter, r *http.Request) {
	var dbMatchPlayInfos []MatchPlayInfo
	result := s.db.Find(&dbMatchPlayInfos)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dbMatchPlayInfos)
}

func (s *Server) POSTMatchPlayInfo(w http.ResponseWriter, r *http.Request) {
	var input MatchPlayInfo
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	if !validateYear(input.Year) {
		http.Error(w, "Malformed year", http.StatusBadRequest)
		return
	}

	if err := s.db.Create(&input).Error; err != nil {
		http.Error(w, "Could not save standings", http.StatusInternalServerError)
		return
	}

	if input.BracketUrl != "" {
		if err := updateMatchPlayResults(s.db, input.Year, input.BracketUrl); err != nil {
			fmt.Println("*******", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(input)
}

func (s *Server) PUTMatchPlayInfo(w http.ResponseWriter, r *http.Request) {
	var input MatchPlayInfo
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Fetch existing record
	existing := &MatchPlayInfo{}
	result := s.db.First(existing, "year = ?", input.Year)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			http.Error(w, "Match play not found", http.StatusNotFound)
		} else {
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	if input.BracketUrl != existing.BracketUrl && input.BracketUrl != "" {
		if err := updateMatchPlayResults(s.db, input.Year, input.BracketUrl); err != nil {
			fmt.Println("*******", err)
		}
	}

	// Update fields
	existing.Year = input.Year
	existing.RegistrationOpen = input.RegistrationOpen
	existing.BracketUrl = input.BracketUrl
	existing.ShopifyUrl = input.ShopifyUrl

	if err := s.db.Save(&existing).Error; err != nil {
		http.Error(w, "Failed to update record", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(existing)
}

func (s *Server) DELETEMatchPlayInfo(w http.ResponseWriter, r *http.Request) {
	// Decode the incoming JSON to get the calendar year to delete
	var payload struct {
		Year string `json:"year"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Look up the existing record
	matchPlayInfo := &MatchPlayInfo{}
	result := s.db.First(matchPlayInfo, "year = ?", payload.Year)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			http.Error(w, "Match play year not found", http.StatusNotFound)
		} else {
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	// Delete the record
	if err := s.db.Unscoped().Delete(&MatchPlayInfo{}, "year = ?", payload.Year).Error; err != nil {
		http.Error(w, "Could not delete match play year", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) POSTRefreshMatchPlayBracket(w http.ResponseWriter, r *http.Request) {
	var existing MatchPlayInfo
	err := s.db.Order("year DESC").First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "Error loading match play from db", http.StatusInternalServerError)
		return
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "Match Play not saved yet", http.StatusBadRequest)
		return
	}

	if existing.BracketUrl != "" {
		if err := updateMatchPlayResults(s.db, existing.Year, existing.BracketUrl); err != nil {
			http.Error(w, fmt.Sprintf("Error downloading new bracket: %s", err.Error()), http.StatusBadRequest)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) GETMatchPlayResults(w http.ResponseWriter, r *http.Request) {
	requestedYear := r.URL.Query().Get("year")

	var years []string
	if err := s.db.Model(&MatchPlayMatch{}).Distinct().Pluck("year", &years).Error; err != nil {
		http.Error(w, "Failed to fetch match play years", http.StatusInternalServerError)
		return
	}
	if len(years) == 0 {
		json.NewEncoder(w).Encode(map[string]any{
			"calendarYear":    nil,
			"additionalYears": []string{},
			"results":         []MatchPlayMatch{},
		})
		return
	}

	// Sort years descending
	sort.Sort(sort.Reverse(sort.StringSlice(years)))

	// Decide target year
	targetYear := years[0] // default to most recent
	if requestedYear != "" {
		found := false
		for _, y := range years {
			if y == requestedYear {
				targetYear = requestedYear
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "Requested year not found", http.StatusNotFound)
			return
		}
	}

	// Compute additional years
	additionalYears := make([]string, 0, len(years)-1)
	for _, y := range years {
		if y != targetYear {
			additionalYears = append(additionalYears, y)
		}
	}

	// Fetch match data
	var matches []MatchPlayMatch
	if err := s.db.Where("year = ?", targetYear).
		Find(&matches).Error; err != nil {
		http.Error(w, "Failed to fetch match play results", http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"calendarYear":    targetYear,
		"additionalYears": additionalYears,
		"results":         matches,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) GETMatchPlayPlayers(w http.ResponseWriter, r *http.Request) {
	var players []MatchPlayPlayer
	if err := s.db.Find(&players).Error; err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(players)
}

func (s *Server) POSTMatchPlayPlayer(w http.ResponseWriter, r *http.Request) {
	var player MatchPlayPlayer
	if err := json.NewDecoder(r.Body).Decode(&player); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if player.Player == "" || player.Handicap == "" {
		http.Error(w, "Missing player or handicap", http.StatusBadRequest)
		return
	}
	player.Player = basicSanitize(player.Player)

	if err := s.db.Create(&player).Error; err != nil {
		http.Error(w, "Database insert error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(player)
}

func (s *Server) PUTMatchPlayPlayer(w http.ResponseWriter, r *http.Request) {
	var input MatchPlayPlayer
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if input.Player == "" {
		http.Error(w, "Missing player name", http.StatusBadRequest)
		return
	}

	var existing MatchPlayPlayer
	if err := s.db.First(&existing, "player = ?", input.Player).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Player not found", http.StatusNotFound)
		} else {
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	existing.Player = input.Player
	existing.Handicap = input.Handicap

	if err := s.db.Save(&existing).Error; err != nil {
		http.Error(w, "Update failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(existing)
}

func (s *Server) DELETEMatchPlayPlayer(w http.ResponseWriter, r *http.Request) {
	var req MatchPlayPlayer
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Player == "" {
		http.Error(w, "Missing player name", http.StatusBadRequest)
		return
	}

	// Hard‐delete all rows matching the given player name
	if err := s.db.
		Where("player = ?", req.Player).
		Delete(&MatchPlayPlayer{}).
		Error; err != nil {
		http.Error(w, "Delete failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GETDataDirectory(w http.ResponseWriter, r *http.Request) {
	directoryToZip := s.dataDir

	// Make sure the directory exists
	info, err := os.Stat(directoryToZip)
	if err != nil || !info.IsDir() {
		http.Error(w, "Directory not found", http.StatusNotFound)
		return
	}

	// Name of the downloaded .zip file
	zipName := "lfg-server-data.zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+zipName+`"`)

	// Create a zip.Writer that writes directly to the http.ResponseWriter
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	// Walk the directory tree
	filepath.Walk(directoryToZip, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			// Abort on any file-system error
			return err
		}

		// Compute the path within the ZIP file (make it relative)
		relPath, err := filepath.Rel(filepath.Dir(directoryToZip), path)
		if err != nil {
			return err
		}
		// We don't want the ZIP entries to begin with “../…”, trim any leading separators
		relPath = strings.TrimPrefix(relPath, string(filepath.Separator))

		// If this is a directory, create a folder entry (with trailing slash)
		if fi.IsDir() {
			if relPath == "" {
				// Skip the root directory itself; zip.Writer will create directories implicitly
				return nil
			}
			_, err := zipWriter.Create(relPath + "/")
			return err
		}

		// For files, open and copy into a new zip entry
		inFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer inFile.Close()

		// Create the file header in the zip (preserving file permissions)
		header, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		header.Name = relPath
		// Use deflate compression
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		_, err = io.Copy(writer, inFile)
		return err
	})
}

func parseBlueGolf(raw string) (club, contest string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}

	// Split into path segments
	segs := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")

	// Find the segment after "bluegolfw" and after "poy"
	var iBlue, iPoy = -1, -1
	for i, s := range segs {
		if s == "bluegolfw" {
			iBlue = i
		} else if s == "poy" {
			iPoy = i
		}
	}

	if iBlue < 0 || iBlue+1 >= len(segs) || iPoy < 0 || iPoy+1 >= len(segs) {
		return "", "", fmt.Errorf("unexpected path format: %q", u.Path)
	}

	return segs[iBlue+1], segs[iPoy+1], nil
}

// fetchBlueGolfJSON gets the JSON (handling cookies, headers, redirects, and IPv4-only path).
func fetchBlueGolfJSON(ctx context.Context, queryURL, primeURL string) (*tournamentsEnvelope, error) {
	jar, _ := cookiejar.New(nil)

	// Force IPv4 (edges sometimes differ on v6)
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp4", addr)
		},
		ForceAttemptHTTP2: true,
	}

	client := &http.Client{
		Timeout:   20 * time.Second,
		Jar:       jar,
		Transport: transport,
		// Show 30x so we can inspect Location
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}

	ua := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

	// 1) Prime cookies (ignore status; just read to EOF)
	if primeURL != "" {
		preq, _ := http.NewRequestWithContext(ctx, http.MethodGet, primeURL, nil)
		preq.Header.Set("User-Agent", ua)
		preq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		preq.Header.Set("Referer", "https://www.bluegolf.com/")
		if presp, err := client.Do(preq); err == nil {
			io.Copy(io.Discard, presp.Body)
			presp.Body.Close()
		}
	}

	type attempt struct{ withXHR bool }
	tries := []attempt{{withXHR: true}, {withXHR: false}}

	var lastDiag error

	for _, t := range tries {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
		req.Header.Set("Referer", primeURL)
		req.Header.Set("Origin", "https://nhgaclub.bluegolf.com")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		if t.withXHR {
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
		}

		resp, err := client.Do(req)
		if err != nil {
			lastDiag = fmt.Errorf("request error: %w", err)
			continue
		}

		// Handle a single manual redirect hop, capturing Location
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			bodyPeek, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()

			if loc == "" {
				lastDiag = fmt.Errorf("redirect %d without Location; body: %s", resp.StatusCode, string(bodyPeek))
				continue
			}
			u, _ := resp.Request.URL.Parse(loc)
			req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			req2.Header = req.Header.Clone()
			resp2, err2 := client.Do(req2)
			if err2 != nil {
				lastDiag = fmt.Errorf("follow redirect to %s failed: %w", u.String(), err2)
				continue
			}
			resp = resp2
		}

		ct := strings.ToLower(resp.Header.Get("Content-Type"))
		bodyPeek, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		resp.Body.Close()

		// If mislabeled, detect JSON by first char
		isJSON := strings.Contains(ct, "application/json") || strings.Contains(ct, "text/json")
		trim := bytes.TrimSpace(bodyPeek)
		if !isJSON && len(trim) > 0 && (trim[0] == '{' || trim[0] == '[') {
			isJSON = true
		}
		if !isJSON || resp.StatusCode/100 != 2 {
			lastDiag = fmt.Errorf("non-JSON: status=%d ct=%q url=%s\npeek:\n%s",
				resp.StatusCode, ct, queryURL, string(bodyPeek))
			continue
		}

		// Decode
		var env tournamentsEnvelope
		if err := json.NewDecoder(bytes.NewReader(bodyPeek)).Decode(&env); err != nil {
			lastDiag = fmt.Errorf("decode error (ct=%q status=%d url=%s): %w\npeek:\n%s",
				ct, resp.StatusCode, queryURL, err, string(bodyPeek))
			continue
		}
		return &env, nil
	}

	return nil, fmt.Errorf("BlueGolf fetch failed: %v", lastDiag)
}
