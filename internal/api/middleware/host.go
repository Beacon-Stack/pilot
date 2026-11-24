package middleware

import (
	"net"
	"net/netip"
	"strings"
)

// Host checking stops DNS rebinding: a web page whose own domain is made
// to resolve to this server's LAN address. The browser then treats the
// page's requests as same-origin and sends them from a local address, so
// the cross-site check passes and "local addresses need no login" lets
// them in. The one thing the attacker can't fake is the Host header: it
// carries their domain. A request is only trusted as local when Host is
// a name that can't be an attacker's public domain.

// homeSuffixes are name endings that only resolve on a home or office
// network: they aren't public top-level domains anyone can register
// under.
var homeSuffixes = []string{".localhost", ".local", ".lan", ".home", ".internal", ".home.arpa", ".localdomain"}

// hostName returns the host part of a Host header, lower-cased, without
// port, brackets or a trailing dot.
func hostName(hostport string) string {
	h := strings.TrimSpace(hostport)
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	return strings.ToLower(strings.TrimSuffix(h, "."))
}

// localHostName reports whether a Host header names this server in a way
// a rebinding page can't: an IP address, localhost, a single-word name
// ("pilot", "nas"), a home-network name ("nas.lan"), or one the owner
// listed in server.allowed_hosts.
func localHostName(hostport string, allowed []string) bool {
	h := hostName(hostport)
	if h == "" {
		return false
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return true // an address can't be rebound
	}
	if h == "localhost" || !strings.Contains(h, ".") {
		return true
	}
	for _, s := range homeSuffixes {
		if strings.HasSuffix(h, s) {
			return true
		}
	}
	for _, a := range allowed {
		a = hostName(a)
		if a == "" {
			continue
		}
		if h == a {
			return true
		}
		// "*.example.com" or ".example.com" allows every name under it.
		if suffix, ok := strings.CutPrefix(a, "*"); ok && strings.HasPrefix(suffix, ".") && strings.HasSuffix(h, suffix) {
			return true
		}
		if strings.HasPrefix(a, ".") && strings.HasSuffix(h, a) {
			return true
		}
	}
	return false
}
