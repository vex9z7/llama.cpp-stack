package openai

import (
	"encoding/json"
	"net/http"
)

type ErrorBody struct {
	Error ErrorObject `json:"error"`
}

type ErrorObject struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

func WriteError(w http.ResponseWriter, status int, typ, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorBody{Error: ErrorObject{Message: msg, Type: typ, Code: code}})
}
