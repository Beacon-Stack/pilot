package redact

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestError_ScrubsRawAndEscapedForms(t *testing.T) {
	secret := "k3y+with/special=chars"
	err := errors.New(`Get "http://idx/api?apikey=k3y%2Bwith%2Fspecial%3Dchars": dial tcp: refused; raw=k3y+with/special=chars; path=k3y+with%2Fspecial=chars`)
	got := Error(err, secret).Error()
	if strings.Contains(got, "k3y") {
		t.Fatalf("secret survived redaction: %s", got)
	}
}

func TestError_PreservesUnwrap(t *testing.T) {
	base := context.DeadlineExceeded
	wrapped := Error(&wrapErr{msg: "timeout calling ?apikey=supersecret", err: base}, "supersecret")
	if !errors.Is(wrapped, context.DeadlineExceeded) {
		t.Fatal("errors.Is lost the original cause")
	}
}

func TestError_NoMatchReturnsOriginal(t *testing.T) {
	err := errors.New("nothing secret here")
	if Error(err, "supersecret") != err {
		t.Fatal("unmatched error should be returned as-is")
	}
	if Error(nil, "x") != nil {
		t.Fatal("nil in, nil out")
	}
}

func TestError_IgnoresTinySecrets(t *testing.T) {
	err := errors.New("HTTP 1 of 1")
	if got := Error(err, "1").Error(); got != "HTTP 1 of 1" {
		t.Fatalf("tiny secret mangled the message: %s", got)
	}
}

// End-to-end: a real http.Client failure carries the full URL in a
// *url.Error. This is the exact leak the package exists for.
func TestError_RealURLError(t *testing.T) {
	const key = "0123456789abcdef-real-key"
	client := &http.Client{Timeout: time.Second}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"http://127.0.0.1:1/api?t=caps&apikey="+key, nil)
	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Skip("expected connection failure on port 1")
	}
	if !strings.Contains(err.Error(), key) {
		t.Fatalf("precondition: Go's url.Error should include the key; got %s", err)
	}
	if got := Error(err, key).Error(); strings.Contains(got, key) {
		t.Fatalf("key leaked: %s", got)
	}
}

type wrapErr struct {
	msg string
	err error
}

func (e *wrapErr) Error() string { return e.msg }
func (e *wrapErr) Unwrap() error { return e.err }
