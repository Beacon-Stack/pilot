package sabnzbd_test

// Regression: request failures must not leak credentials. Go's *url.Error
// includes the full request URL, which for this plugin carries a secret.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/beacon-stack/pilot/plugins/downloaders/sabnzbd"
)

const leakSecret = "s3cret-0123456789abcdef"

// deadURL returns the URL of a server that is no longer listening, so any
// request to it fails with a connection error wrapped in *url.Error.
func deadURL() string {
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL
	srv.Close()
	return u
}

func TestRequestErrorDoesNotLeakSecret(t *testing.T) {
	ctx := context.Background()
	// GetQueue, not Test: Test starts with a keyless version probe.
	_, err := sabnzbd.New(sabnzbd.Config{URL: deadURL(), APIKey: leakSecret}).GetQueue(ctx)
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if strings.Contains(err.Error(), leakSecret) {
		t.Fatalf("error leaked the secret: %v", err)
	}
}
