package polaris

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientAuthAndErrors(t *testing.T) {
	var tokenRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/catalog/v1/oauth/tokens" {
			tokenRequests++
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("scope") != "PRINCIPAL_ROLE:ALL" || r.Form.Get("client_secret") != "fixture-secret" {
				t.Errorf("incorrect client credential form")
			}
			w.Write([]byte(`{"access_token":"fixture-token","expires_in":300}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("Polaris-Realm") != "test" {
			t.Errorf("missing authentication or realm headers")
		}
		if r.URL.Path == "/api/management/v1/denied" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"message":"fixture-secret fixture-token"}`))
			return
		}
		w.Write([]byte(`{"name":"ok"}`))
	}))
	defer server.Close()
	c, err := newClient(server.URL, "test", "", "fixture-client", "fixture-secret")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if _, err = c.request(context.Background(), http.MethodGet, "/ok", nil, &out, 200); err != nil || out["name"] != "ok" {
		t.Fatalf("request failed: %v, response: %v", err, out)
	}
	_, err = c.request(context.Background(), http.MethodGet, "/denied", nil, nil, 200)
	if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "fixture-") {
		t.Fatalf("expected secret-safe forbidden diagnostic, got %v", err)
	}
	if tokenRequests != 1 {
		t.Fatalf("expected cached OAuth token, got %d requests", tokenRequests)
	}
}

func TestClientRejectsUnsafeBaseURL(t *testing.T) {
	for _, base := range []string{"http://polaris.example.com", "file:///tmp/polaris", "https://user:secret@example.com", "https://example.com?secret=1", "https://example.com/#fragment", "https:///"} {
		if _, err := newClient(base, "", "token", "", ""); err == nil {
			t.Errorf("accepted unsafe URL")
		}
	}
}
