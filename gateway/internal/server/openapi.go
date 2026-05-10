package server

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

const maxInferenceBodyBytes int64 = 32 << 20

func (a *App) Register(api huma.API) {
	registerRaw(api, withJSONResponse(&huma.Operation{OperationID: "getHealth", Method: http.MethodGet, Path: "/health", Summary: "Gateway health", Tags: []string{"system"}}), a.humaHealth)
	registerRaw(api, withJSONResponse(&huma.Operation{OperationID: "listModels", Method: http.MethodGet, Path: "/v1/models", Summary: "List catalog models", Tags: []string{"models"}}), a.humaModels)
	for _, path := range []string{"/v1/chat/completions", "/v1/completions", "/v1/responses", "/v1/embeddings"} {
		registerRaw(api, withProxyDocs(&huma.Operation{OperationID: operationID(path), Method: http.MethodPost, Path: path, Summary: "Proxy OpenAI-compatible inference request", Tags: []string{"inference"}, MaxBodyBytes: maxInferenceBodyBytes, SkipValidateBody: true, Errors: []int{400, 404, 503}}), a.humaInference)
	}
}

func registerRaw(api huma.API, op *huma.Operation, handler func(huma.Context)) {
	api.OpenAPI().AddOperation(op)
	api.Adapter().Handle(op, handler)
}

func withJSONResponse(op *huma.Operation) *huma.Operation {
	op.Responses = map[string]*huma.Response{"200": jsonResponse("JSON response")}
	return op
}

func withProxyDocs(op *huma.Operation) *huma.Operation {
	op.RequestBody = &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{"application/json": {Schema: &huma.Schema{Type: "object", Required: []string{"model"}, AdditionalProperties: true, Properties: map[string]*huma.Schema{"model": {Type: "string"}}}}}}
	op.Responses = map[string]*huma.Response{
		"200": jsonResponse("Successful upstream response. Shape depends on the proxied llama.cpp/OpenAI-compatible endpoint."),
		"400": jsonResponse("Gateway validation error"),
		"404": jsonResponse("Catalog model not found"),
		"503": jsonResponse("Download, router reload, or upstream availability error"),
	}
	return op
}

func jsonResponse(desc string) *huma.Response {
	return &huma.Response{Description: desc, Content: map[string]*huma.MediaType{"application/json": {Schema: &huma.Schema{Type: "object", AdditionalProperties: true}}}}
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
