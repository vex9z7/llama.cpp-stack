package server

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

func TestOpenAPISurface(t *testing.T) {
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "0.0.0"))
	a := &App{}
	a.Register(api)
	want := map[string]string{
		"/health":              "get",
		"/v1/models":           "get",
		"/v1/chat/completions": "post",
		"/v1/completions":      "post",
		"/v1/responses":        "post",
		"/v1/embeddings":       "post",
	}
	for path, method := range want {
		pathItem := api.OpenAPI().Paths[path]
		if pathItem == nil {
			t.Fatalf("missing OpenAPI path %s", path)
		}
		if operationForMethod(pathItem, method) == nil {
			t.Fatalf("missing OpenAPI operation %s %s", method, path)
		}
	}
	if api.OpenAPI().Paths["/slots"] != nil || api.OpenAPI().Paths["/models/load"] != nil {
		t.Fatalf("router management endpoints must not be part of gateway OpenAPI")
	}
}

func operationForMethod(pathItem *huma.PathItem, method string) *huma.Operation {
	switch method {
	case "get":
		return pathItem.Get
	case "post":
		return pathItem.Post
	default:
		return nil
	}
}
