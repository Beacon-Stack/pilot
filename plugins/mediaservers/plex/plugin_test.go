package plex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testSectionsXML = `<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
  <Directory key="1" title="Movies" type="movie">
    <Location path="/movies"/>
  </Directory>
  <Directory key="2" title="TV Shows" type="show">
    <Location path="/tv"/>
  </Directory>
</MediaContainer>`

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
		case "/library/sections":
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(testSectionsXML))
		case "/library/sections/1/refresh":
			w.WriteHeader(http.StatusOK)
		case "/library/sections/1/all":
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0"?>
<MediaContainer>
  <Video ratingKey="100" title="Inception" year="2010">
    <Guid id="tmdb://27205"/>
  </Video>
</MediaContainer>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	s := &Server{
		cfg:    Config{URL: srv.URL, Token: "test-token"},
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

func TestTest_BadToken(t *testing.T) {
	_, srv := newTestServer(t)
	defer srv.Close()

	s := &Server{
		cfg:    Config{URL: srv.URL, Token: "bad-token"},
		client: srv.Client(),
	}
	if err := s.Test(context.Background()); err == nil {
		t.Fatal("expected error for bad token")
	}
}

func TestRefreshLibrary_MatchesPath(t *testing.T) {
	s, srv := newTestServer(t)
	defer srv.Close()

	if err := s.RefreshLibrary(context.Background(), "/movies/Inception (2010)/Inception.mkv"); err != nil {
		t.Fatalf("RefreshLibrary() = %v", err)
	}
}

func TestListSections(t *testing.T) {
	s, srv := newTestServer(t)
	defer srv.Close()

	sections, err := s.ListSections(context.Background())
	if err != nil {
		t.Fatalf("ListSections() = %v", err)
	}
	// Only movie sections should be returned.
	if len(sections) != 1 {
		t.Fatalf("len = %d, want 1", len(sections))
	}
	if sections[0].Title != "Movies" {
		t.Errorf("Title = %q, want Movies", sections[0].Title)
	}
}

func TestListMovies(t *testing.T) {
	s, srv := newTestServer(t)
	defer srv.Close()

	movies, err := s.ListMovies(context.Background(), "1")
	if err != nil {
		t.Fatalf("ListMovies() = %v", err)
	}
	if len(movies) != 1 {
		t.Fatalf("len = %d, want 1", len(movies))
	}
	if movies[0].Title != "Inception" {
		t.Errorf("Title = %q", movies[0].Title)
	}
	if movies[0].TmdbID != 27205 {
		t.Errorf("TmdbID = %d, want 27205", movies[0].TmdbID)
	}
}

func TestExtractTmdbID(t *testing.T) {
	tests := []struct {
		name  string
		video plexVideo
		want  int
	}{
		{
			"new agent format",
			plexVideo{Guids: []plexGID{{ID: "tmdb://27205"}}},
			27205,
		},
		{
			"legacy agent format",
			plexVideo{GUID: "com.plexapp.agents.themoviedb://27205?lang=en"},
			27205,
		},
		{
			"no tmdb guid",
			plexVideo{Guids: []plexGID{{ID: "imdb://tt1375666"}}},
			0,
		},
		{
			"empty",
			plexVideo{},
			0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTmdbID(tt.video)
			if got != tt.want {
				t.Errorf("extractTmdbID() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	s := New(Config{URL: "http://plex.local/", Token: "tok"})
	if s.cfg.URL != "http://plex.local" {
		t.Errorf("URL = %q, want no trailing slash", s.cfg.URL)
	}
}
