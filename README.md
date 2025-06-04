# Live Free Golf - Server


## APIs
All APIs are JSON except `POST` and `PUT` `event` which are multipart/form-data (JSON and image). 

Data models can be found [here](https://github.com/cpacia/lfg-server/blob/main/models.go).
```go
r.Post("/login", s.POSTLoginHandler)
r.Post("/logout", s.POSTLogoutHandler)
r.Get("/auth/me", authMiddleware(s.POSTAuthMe))
r.Post("/change-password", authMiddleware(s.POSTChangePasswordHandler))

r.Get("/standings", s.GETStandings) // Default to current year. ?year= for other years
r.Get("/standings-urls", s.GETStandingsUrls)
r.Post("/standings-urls", authMiddleware(s.POSTStandingsUrls))
r.Put("/standings-urls", authMiddleware(s.PUTStandingsUrls))
r.Delete("/standings-urls", authMiddleware(s.DELETEStandingsUrls))
r.Post("/refresh-standings", authMiddleware(s.POSTRefreshStandings))

r.Get("/events", s.GETEvents) // Default to current year. ?year= for other years
r.Get("/events/{eventID}", s.GETEvent)
r.Get("/events/{eventID}/thumbnail", s.GETEventThumbnail)
r.Post("/events", authMiddleware(s.POSTEvent))
r.Put("/events/{eventID}", authMiddleware(s.PUTEvent))
r.Delete("/events/{eventID}", authMiddleware(s.DELETEEvent))

r.Get("/results/net/{eventID}", s.GETNetResults)
r.Get("/results/gross/{eventID}", s.GETGrossResults)
r.Get("/results/skins/{eventID}", s.GETSkinsResults)
r.Get("/results/teams/{eventID}", s.GETTeamResults)
r.Get("/results/wgr/{eventID}", s.GETWgrResults)

r.Get("/disabled-golfers", s.GETDisabledGolfer)
r.Post("/disabled-golfers/{name}", authMiddleware(s.POSTDisabledGolfer))
r.Put("/disabled-golfers/{name}", authMiddleware(s.PUTDisabledGolfer))
r.Delete("/disabled-golfers/{name}", authMiddleware(s.DELETEDisabledGolfer))

r.Get("/colony-cup/{year}", s.GETColonyCupInfo)
r.Post("/colony-cup", authMiddleware(s.POSTColonyCupInfo))
r.Put("/colony-cup/{year}", authMiddleware(s.PUTColonyCupInfo))
r.Delete("/colony-cup/{year}", authMiddleware(s.DELETEColonyCupInfo))

r.Get("/match-play", s.GETMatchPlayInfo)
r.Put("/match-play", authMiddleware(s.PUTMatchPlayInfo))
r.Post("/refresh-match-play-bracket", authMiddleware(s.POSTRefreshMatchPlayBracket))
r.Get("/match-play/results", s.GETMatchPlayResults) // Default to current year. ?year= for other years

r.Get("/current-year", s.GETCurrentYear)
```
