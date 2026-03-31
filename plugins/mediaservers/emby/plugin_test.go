package emby

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.URL.Query().Get("api_key")
		if apiKey != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/System/Info":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ServerName":"Emby"}`))
		case "/Library/Refresh":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	s := &Server{
		cfg:    Config{URL: srv.URL, APIKey: "test-key"},
		client: srv.Client(),
	}
	return s, srv
}

func TestTest_Success(t *testing.T) {
	s, srv := newTestServer(t)
	defer srv.Close()

	if err := s.Test(context.Background()); err != nil {
		t.Fatalf("Test() = %v", err)
	}
}

func TestTest_BadKey(t *testing.T) {
	_, srv := newTestServer(t)
	defer srv.Close()

	s := &Server{
		cfg:    Config{URL: srv.URL, APIKey: "wrong"},
		client: srv.Client(),
	}
	if err := s.Test(context.Background()); err == nil {
		t.Fatal("expected error for bad API key")
	}
}

func TestRefreshLibrary_Success(t *testing.T) {
	s, srv := newTestServer(t)
	defer srv.Close()

	if err := s.RefreshLibrary(context.Background(), "/movies/Inception"); err != nil {
		t.Fatalf("RefreshLibrary() = %v", err)
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	s := New(Config{URL: "http://emby.local/", APIKey: "key"})
	if s.cfg.URL != "http://emby.local" {
		t.Errorf("URL = %q, want no trailing slash", s.cfg.URL)
	}
}
