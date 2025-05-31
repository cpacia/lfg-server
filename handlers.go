package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"net/http"
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
	if err := updateStandings(dbStandings, s.db); err != nil {
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

	if err := updateStandings(&latest, s.db); err != nil {
		http.Error(w, fmt.Sprintf("Error downloading new standings: %s", err.Error()), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}
