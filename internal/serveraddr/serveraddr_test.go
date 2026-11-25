package serveraddr

import "testing"

func TestFromPasted(t *testing.T) {
	cases := map[string]string{
		"http://plex.local/":                            "http://plex.local",
		"http://192.168.1.6:32400":                      "http://192.168.1.6:32400",
		"http://192.168.1.6:32400/web/index.html#!/":    "http://192.168.1.6:32400",
		"http://192.168.1.6:32400/web":                  "http://192.168.1.6:32400",
		"http://192.168.1.6:8096/web/index.html#!/home": "http://192.168.1.6:8096",
		"https://media.example.com/jellyfin/web/#/home": "https://media.example.com/jellyfin",
		"https://media.example.com/jellyfin":            "https://media.example.com/jellyfin",
		" http://192.168.1.6:32400/?x=1 ":               "http://192.168.1.6:32400",
	}
	for in, want := range cases {
		if got := FromPasted(in); got != want {
			t.Errorf("FromPasted(%q) = %q, want %q", in, got, want)
		}
	}
}
