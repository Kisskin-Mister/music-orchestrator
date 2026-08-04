package main

func OpenAPISchema() map[string]any {
	return map[string]any{
		"openapi": "3.1.0",
		"info":    map[string]any{"title": "Music Orchestrator API", "version": "1.0.0", "description": "Go self-hosted music backend: search, playback, downloads, media serving."},
		"servers": []map[string]string{{"url": "http://localhost:8080"}},
		"paths": map[string]any{
			"/health":                            map[string]any{"get": op("Health", false)},
			"/v1/providers":                      map[string]any{"get": op("List providers", false)},
			"/v1/auth/session":                   map[string]any{"get": op("Get auth/session state", false)},
			"/v1/auth/register":                  map[string]any{"post": op("First-run owner registration", false)},
			"/v1/auth/login":                     map[string]any{"post": op("Login with owner password", false)},
			"/v1/auth/verify":                    map[string]any{"post": op("Verify TOTP code", false)},
			"/v1/auth/logout":                    map[string]any{"post": op("Logout", false)},
			"/v1/auth/me":                        map[string]any{"get": op("Current authenticated identity", true)},
			"/v1/search":                         map[string]any{"get": op("Search tracks", false)},
			"/v1/tracks/{track_id}":              map[string]any{"get": op("Get track", false)},
			"/v1/playback/{track_id}":            map[string]any{"get": op("Resolve playback", false)},
			"/v1/stream/{track_id}":              map[string]any{"get": op("Proxy a compatible extractor audio stream", false)},
			"/v1/downloads":                      map[string]any{"post": op("Download track", true)},
			"/media/{filename}":                  map[string]any{"get": op("Serve downloaded media", false)},
			"/v1/favorites":                      map[string]any{"get": op("List favorites", true), "post": op("Add favorite", true)},
			"/v1/favorites/{track_id}":           map[string]any{"delete": op("Delete favorite", true)},
			"/v1/playlists":                      map[string]any{"get": op("List playlists", true), "post": op("Create playlist", true)},
			"/v1/playlists/{playlist_id}":        map[string]any{"get": op("Get playlist", true), "patch": op("Update playlist", true), "delete": op("Delete playlist", true)},
			"/v1/playlists/{playlist_id}/cover":  map[string]any{"post": op("Upload playlist cover", true)},
			"/v1/playlists/{playlist_id}/tracks": map[string]any{"post": op("Add playlist track", true)},
			"/v1/jobs":                           map[string]any{"get": op("List jobs", true)},
			"/v1/jobs/{job_id}":                  map[string]any{"get": op("Get job", true)},
		},
		"components": map[string]any{"securitySchemes": map[string]any{"ApiKeyAuth": map[string]any{"type": "apiKey", "in": "header", "name": "X-API-Key"}}},
	}
}

func op(summary string, auth bool) map[string]any {
	o := map[string]any{"summary": summary, "responses": map[string]any{"200": map[string]any{"description": "OK"}, "400": map[string]any{"description": "Bad request"}, "500": map[string]any{"description": "Server error"}}}
	if auth {
		o["security"] = []map[string][]string{{"ApiKeyAuth": []string{}}}
	}
	return o
}
