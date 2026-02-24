package handler

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the standard error shape used by handlers.
// Matches TDD: { "error": "<message>", "code": "<ERROR_CODE>" }
type ErrorResponse struct {
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg, Code: code})
}
