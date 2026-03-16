package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInspectHandler_MethodNotAllowed(t *testing.T) {
	h := NewHandlers(Config{BaseDir: t.TempDir()}, nil)
	req := httptest.NewRequest(http.MethodGet, "/inspect", nil)
	rec := httptest.NewRecorder()
	h.Inspect(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestInspectHandler_BadJSON(t *testing.T) {
	h := NewHandlers(Config{BaseDir: t.TempDir()}, nil)
	req := httptest.NewRequest(http.MethodPost, "/inspect", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()
	h.Inspect(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHealthHandler_MethodNotAllowed(t *testing.T) {
	h := NewHandlers(Config{BaseDir: t.TempDir()}, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.Health(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestHealthHandler_BadJSON(t *testing.T) {
	h := NewHandlers(Config{BaseDir: t.TempDir()}, nil)
	req := httptest.NewRequest(http.MethodPost, "/health", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()
	h.Health(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d got %d", http.StatusBadRequest, rec.Code)
	}
}
