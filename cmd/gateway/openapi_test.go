package main

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

func TestOpenAPISurface(t *testing.T) {
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "0.0.0"))
	a := &app{}
	a.register(api)
	want := []string{"/health", "/v1/models", "/v1/chat/completions", "/v1/completions", "/v1/responses", "/v1/embeddings"}
	for _, path := range want {
		if api.OpenAPI().Paths[path] == nil {
			t.Fatalf("missing OpenAPI path %s", path)
		}
	}
	if api.OpenAPI().Paths["/slots"] != nil || api.OpenAPI().Paths["/models/load"] != nil {
		t.Fatalf("router management endpoints must not be part of gateway OpenAPI")
	}
}
