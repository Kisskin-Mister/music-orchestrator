package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

func newID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorBody{Error: ErrorDetail{Code: fmt.Sprintf("http_%d", status), Message: msg}})
}
func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}
func stringPtr(s string) *string { return &s }
func intPtr(i int) *int          { return &i }

var safeRe = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func safeStem(parts ...string) string {
	s := safeRe.ReplaceAllString(strings.Join(parts, "-"), "-")
	s = strings.Trim(s, ".-")
	if len(s) > 180 {
		s = s[:180]
	}
	if s == "" {
		return "track"
	}
	return s
}
