// Package serveraddr cleans up server addresses typed or pasted into
// settings.
package serveraddr

import "strings"

// FromPasted turns whatever was put in a media server's URL field into
// the server's base address. People copy the address from their browser,
// which is the web app's page (http://host:32400/web/index.html#!/...),
// not the server itself; requests built on that get the web app's HTML
// back instead of data.
func FromPasted(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "#"); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.Index(raw, "?"); i >= 0 {
		raw = raw[:i]
	}
	raw = strings.TrimRight(raw, "/")
	if i := strings.Index(raw, "/web/"); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSuffix(raw, "/web")
}
