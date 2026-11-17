// Package redact scrubs credentials out of error messages.
//
// Plugins that put a secret in a request URL (indexer ?apikey=, Emby
// ?api_key=, Gotify ?token=, Telegram /bot<token>/, Discord/Slack webhook
// URLs) leak it whenever the request fails: http.Client wraps transport
// errors in *url.Error, whose message includes the full URL. Those errors
// end up in logs, the log-viewer API, and "Test" results in the UI.
package redact

import (
	"net/url"
	"strings"
)

// Placeholder replaces each secret occurrence.
const Placeholder = "REDACTED"

// minSecretLen guards against redacting tiny values (e.g. "1") that would
// mangle unrelated parts of the message.
const minSecretLen = 4

// Error returns err with every occurrence of each secret — raw, query-
// escaped, and path-escaped — replaced by Placeholder. The original error
// stays reachable through Unwrap, so errors.Is/As (timeouts, context
// cancellation) keep working. Returns err unchanged if nothing matched.
func Error(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	scrubbed := String(msg, secrets...)
	if scrubbed == msg {
		return err
	}
	return &redactedError{msg: scrubbed, err: err}
}

// String replaces every occurrence of each secret in s.
func String(s string, secrets ...string) string {
	for _, secret := range secrets {
		if len(secret) < minSecretLen {
			continue
		}
		for _, form := range []string{secret, url.QueryEscape(secret), url.PathEscape(secret)} {
			s = strings.ReplaceAll(s, form, Placeholder)
		}
	}
	return s
}

type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }
