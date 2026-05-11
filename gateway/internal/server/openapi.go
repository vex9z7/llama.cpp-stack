package server

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

const maxInferenceBodyBytes int64 = 32 << 20

func (a *App) Register(api huma.API) {
	registerRaw(api, withResponse(&huma.Operation{OperationID: "getHealth", Method: http.MethodGet, Path: "/health", Summary: "Gateway health", Tags: []string{"system"}}, healthSchema()), a.humaHealth)
	registerRaw(api, withResponse(&huma.Operation{OperationID: "listModels", Method: http.MethodGet, Path: "/v1/models", Summary: "List catalog models", Tags: []string{"models"}}, modelListSchema()), a.humaModels)
	for _, path := range []string{"/v1/chat/completions", "/v1/completions", "/v1/responses", "/v1/embeddings"} {
		registerRaw(api, withProxyDocs(&huma.Operation{OperationID: operationID(path), Method: http.MethodPost, Path: path, Summary: "Proxy OpenAI-compatible inference request", Tags: []string{"inference"}, MaxBodyBytes: maxInferenceBodyBytes, SkipValidateBody: true, Errors: []int{400, 404, 503}}), a.humaInference)
	}
}

func registerRaw(api huma.API, op *huma.Operation, handler func(huma.Context)) {
	api.OpenAPI().AddOperation(op)
	api.Adapter().Handle(op, handler)
}

func withResponse(op *huma.Operation, schema *huma.Schema) *huma.Operation {
	op.Responses = map[string]*huma.Response{"200": jsonResponse("JSON response", schema)}
	return op
}

func withProxyDocs(op *huma.Operation) *huma.Operation {
	op.RequestBody = &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{"application/json": {Schema: modelRequestSchema()}}}
	op.Responses = map[string]*huma.Response{
		"200": jsonResponse("Successful upstream response. Shape depends on the proxied llama.cpp/OpenAI-compatible endpoint.", openObjectSchema()),
		"400": jsonResponse("Gateway validation error", errorSchema()),
		"404": jsonResponse("Catalog model not found", errorSchema()),
		"503": jsonResponse("Download, router registry, or upstream availability error", errorSchema()),
	}
	return op
}

func jsonResponse(desc string, schema *huma.Schema) *huma.Response {
	return &huma.Response{Description: desc, Content: map[string]*huma.MediaType{"application/json": {Schema: schema}}}
}

func modelRequestSchema() *huma.Schema {
	return &huma.Schema{Type: "object", Required: []string{"model"}, AdditionalProperties: true, Properties: map[string]*huma.Schema{"model": {Type: "string"}}}
}

func healthSchema() *huma.Schema {
	return &huma.Schema{Type: "object", Required: []string{"status", "service", "router"}, Properties: map[string]*huma.Schema{
		"status":  {Type: "string"},
		"service": {Type: "string"},
		"router":  {Type: "string"},
	}}
}

func modelListSchema() *huma.Schema {
	return &huma.Schema{Type: "object", Required: []string{"object", "data"}, Properties: map[string]*huma.Schema{
		"object": {Type: "string"},
		"data": {Type: "array", Items: &huma.Schema{Type: "object", Required: []string{"id", "object", "owned_by", "meta"}, Properties: map[string]*huma.Schema{
			"id":       {Type: "string"},
			"object":   {Type: "string"},
			"owned_by": {Type: "string"},
			"meta":     modelMetaSchema(),
		}}},
	}}
}

func modelMetaSchema() *huma.Schema {
	return &huma.Schema{Type: "object", Required: []string{"downloaded", "running", "cold_start", "repo", "quant"}, Properties: map[string]*huma.Schema{
		"downloaded":    {Type: "boolean"},
		"router_status": {Type: "string"},
		"running":       {Type: "boolean"},
		"cold_start":    {Type: "boolean"},
		"repo":          {Type: "string"},
		"quant":         {Type: "string"},
		"kind":          {Type: "string"},
	}}
}

func errorSchema() *huma.Schema {
	return &huma.Schema{Type: "object", Required: []string{"error"}, Properties: map[string]*huma.Schema{
		"error": {Type: "object", Required: []string{"message", "type"}, Properties: map[string]*huma.Schema{
			"message": {Type: "string"},
			"type":    {Type: "string"},
			"code":    {Type: "string"},
		}},
	}}
}

func openObjectSchema() *huma.Schema {
	return &huma.Schema{Type: "object", AdditionalProperties: true}
}

func operationID(path string) string {
	switch path {
	case "/v1/chat/completions":
		return "createChatCompletion"
	case "/v1/completions":
		return "createCompletion"
	case "/v1/responses":
		return "createResponse"
	case "/v1/embeddings":
		return "createEmbedding"
	default:
		return "proxyInference"
	}
}
