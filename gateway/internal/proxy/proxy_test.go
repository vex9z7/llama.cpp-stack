package proxy

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPClientUsesConfiguredResponseHeaderTimeout(t *testing.T) {
	client := NewHTTPClient(123 * time.Second)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}
	if transport.ResponseHeaderTimeout != 123*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %s", transport.ResponseHeaderTimeout)
	}
}

func TestIsResponseHeaderTimeout(t *testing.T) {
	if !IsResponseHeaderTimeout(errors.New(`Post "http://llama-router:8080/v1/chat/completions": net/http: timeout awaiting response headers`)) {
		t.Fatal("expected response header timeout")
	}
	if IsResponseHeaderTimeout(errors.New("connection refused")) {
		t.Fatal("unexpected response header timeout")
	}
}

func TestNewHTTPClientUsesDefaultResponseHeaderTimeout(t *testing.T) {
	client := NewHTTPClient(0)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}
	if transport.ResponseHeaderTimeout != DefaultResponseHeaderTimeout {
		t.Fatalf("ResponseHeaderTimeout = %s, want %s", transport.ResponseHeaderTimeout, DefaultResponseHeaderTimeout)
	}
}
