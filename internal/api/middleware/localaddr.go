package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// isLocalAddr reports whether ip is on the local machine or a private
// network: loopback, RFC 1918 / RFC 4193 private ranges, or link-local.
// This matches what Sonarr treats as a "local address".
func isLocalAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func parseIP(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"`)
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	if i := strings.IndexByte(s, '%'); i >= 0 { // IPv6 zone
		s = s[:i]
	}
	ip, err := netip.ParseAddr(s)
	return ip, err == nil
}

// clientIsLocal reports whether the request came from a local address.
//
// The direct peer must be local. When it is, it may be a reverse proxy
// (Caddy, nginx, Traefik, cloudflared…) relaying a request from the
// internet, so every client address the proxy reports must be local too.
// An address we can't parse counts as remote. A client can only use these
// headers to make itself look remote, never local, because a remote peer
// fails the first check.
func clientIsLocal(r *http.Request) bool {
	peer, ok := parseIP(r.RemoteAddr)
	if !ok || !isLocalAddr(peer) {
		return false
	}
	var forwarded []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		forwarded = append(forwarded, strings.Split(v, ",")...)
	}
	for _, h := range []string{"X-Real-Ip", "Cf-Connecting-Ip", "True-Client-Ip"} {
		forwarded = append(forwarded, r.Header.Values(h)...)
	}
	for _, v := range r.Header.Values("Forwarded") {
		for _, elem := range strings.Split(v, ",") {
			for _, pair := range strings.Split(elem, ";") {
				k, val, found := strings.Cut(strings.TrimSpace(pair), "=")
				if found && strings.EqualFold(k, "for") {
					forwarded = append(forwarded, val)
				}
			}
		}
	}
	for _, f := range forwarded {
		if strings.TrimSpace(f) == "" {
			continue
		}
		ip, ok := parseIP(f)
		if !ok || !isLocalAddr(ip) {
			return false
		}
	}
	return true
}
