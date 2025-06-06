# Live Free Golf - Server


## APIs
All APIs are JSON except `POST` and `PUT` `event` which are multipart/form-data (JSON and image). 

Data models can be found [here](https://github.com/cpacia/lfg-server/blob/main/models.go).
```go
r.Post("/api/login", s.POSTLoginHandler)
r.Post("/api/logout", s.POSTLogoutHandler)
r.Get("/api/auth/me", authMiddleware(s.POSTAuthMe))
r.Post("/api/change-password", authMiddleware(s.POSTChangePasswordHandler))
r.Get("/api/data-directory", authMiddleware(s.GETDataDirectory))

r.Get("/api/standings", s.GETStandings)
r.Get("/api/standings-urls", s.GETStandingsUrls)
r.Post("/api/standings-urls", authMiddleware(s.POSTStandingsUrls))
r.Put("/api/standings-urls", authMiddleware(s.PUTStandingsUrls))
r.Delete("/api/standings-urls", authMiddleware(s.DELETEStandingsUrls))
r.Post("/api/refresh-standings", authMiddleware(s.POSTRefreshStandings))

r.Get("/api/events", s.GETEvents)
r.Get("/api/events/{eventID}", s.GETEvent)
r.Get("/api/events/{eventID}/thumbnail", s.GETEventThumbnail)
r.Post("/api/events", authMiddleware(s.POSTEvent))
r.Put("/api/events/{eventID}", authMiddleware(s.PUTEvent))
r.Delete("/api/events/{eventID}", authMiddleware(s.DELETEEvent))

r.Get("/api/results/net/{eventID}", s.GETNetResults)
r.Get("/api/results/gross/{eventID}", s.GETGrossResults)
r.Get("/api/results/skins/{eventID}", s.GETSkinsResults)
r.Get("/api/results/teams/{eventID}", s.GETTeamResults)
r.Get("/api/results/wgr/{eventID}", s.GETWgrResults)

r.Get("/api/disabled-golfers", s.GETDisabledGolfer)
r.Post("/api/disabled-golfers/{name}", authMiddleware(s.POSTDisabledGolfer))
r.Put("/api/disabled-golfers/{name}", authMiddleware(s.PUTDisabledGolfer))
r.Delete("/api/disabled-golfers/{name}", authMiddleware(s.DELETEDisabledGolfer))

r.Get("/api/colony-cup", s.GETColonyCupInfo)
r.Get("/api/colony-cup/all", s.GETAllColonyCupInfo)
r.Post("/api/colony-cup", authMiddleware(s.POSTColonyCupInfo))
r.Put("/api/colony-cup", authMiddleware(s.PUTColonyCupInfo))
r.Delete("/api/colony-cup", authMiddleware(s.DELETEColonyCupInfo))

r.Get("/api/match-play", s.GETMatchPlayInfo)
r.Put("/api/match-play", authMiddleware(s.PUTMatchPlayInfo))
r.Post("/api/match-play", authMiddleware(s.POSTMatchPlayInfo))
r.Delete("/api/match-play", authMiddleware(s.DELETEMatchPlayInfo))
r.Post("/api/refresh-match-play-bracket", authMiddleware(s.POSTRefreshMatchPlayBracket))
r.Get("/api/match-play/results", s.GETMatchPlayResults)

r.Get("/api/current-year", s.GETCurrentYear)

r.Get("/current-year", s.GETCurrentYear)
```
