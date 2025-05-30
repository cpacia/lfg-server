package main

import (
	"encoding/json"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
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
	tokenStr, err := token.SignedString([]byte(jwtKey))
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
