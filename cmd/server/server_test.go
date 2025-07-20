package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Test GetTokenFromCookies returns nil when no cookie is set.
func TestGetTokenFromCookies_NoCookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	if tok := GetTokenFromCookies(req); tok != nil {
		t.Errorf("expected nil token when no cookie, got %v", tok)
	}
}

// Test GetTokenFromCookies returns nil for invalid UUID string.
func TestGetTokenFromCookies_InvalidUUID(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	cookie := &http.Cookie{Name: "session_token", Value: "not-a-uuid"}
	req.AddCookie(cookie)
	if tok := GetTokenFromCookies(req); tok != nil {
		t.Errorf("expected nil token for invalid UUID, got %v", tok)
	}
}

// Test GetTokenFromCookies returns correct UUID pointer for valid cookie.
func TestGetTokenFromCookies_Valid(t *testing.T) {
	id := uuid.New()
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: id.String()})
	tok := GetTokenFromCookies(req)
	if tok == nil {
		t.Fatal("expected non-nil token for valid cookie")
	}
	if *tok != id {
		t.Errorf("expected %v, got %v", id, *tok)
	}
}

// errReadCloser simulates a failing Read and safe Close.
type errReadCloser struct{}

func (errReadCloser) Read(p []byte) (int, error) { return 0, errors.New("read failure") }
func (errReadCloser) Close() error               { return nil }

// Test ReadAndUnmarshal handles read errors appropriately.
func TestReadAndUnmarshal_ReadError(t *testing.T) {
	req := httptest.NewRequest("POST", "/", errReadCloser{})
	rec := httptest.NewRecorder()
	var dst interface{}
	err := ReadAndUnmarshal(rec, req, &dst)
	if err == nil {
		t.Fatal("expected error on read failure, got nil")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d on read failure, got %d", http.StatusBadRequest, rec.Code)
	}
}

// Test ReadAndUnmarshal handles JSON parsing errors.
func TestReadAndUnmarshal_ParseError(t *testing.T) {
	body := strings.NewReader("not-json")
	req := httptest.NewRequest("POST", "/", io.NopCloser(body))
	rec := httptest.NewRecorder()
	var dst struct{ Foo string }
	err := ReadAndUnmarshal(rec, req, &dst)
	if err == nil {
		t.Fatal("expected error on invalid JSON, got nil")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d on parse failure, got %d", http.StatusBadRequest, rec.Code)
	}
}

// Test ReadAndUnmarshal succeeds with valid JSON.
func TestReadAndUnmarshal_Success(t *testing.T) {
	body := strings.NewReader(`{"Foo":"Bar"}`)
	req := httptest.NewRequest("POST", "/", io.NopCloser(body))
	rec := httptest.NewRecorder()
	var dst struct{ Foo string }
	err := ReadAndUnmarshal(rec, req, &dst)
	if err != nil {
		t.Fatalf("unexpected error on valid JSON: %v", err)
	}
	if dst.Foo != "Bar" {
		t.Errorf("expected Foo=Bar, got %q", dst.Foo)
	}
}
