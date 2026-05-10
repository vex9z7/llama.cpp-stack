package server

import "github.com/vex9z7/llama.cpp-stack/gateway/internal/apiadapter"

func adaptRequestBody(path string, body []byte) ([]byte, error) {
	if path == "/v1/responses" {
		return apiadapter.AdaptResponsesRequestBody(body)
	}
	return body, nil
}
