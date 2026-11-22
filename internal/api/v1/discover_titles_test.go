package v1

import "testing"

func TestDisplayTitle(t *testing.T) {
	cases := []struct {
		title, original, want string
		ok                    bool
	}{
		{"The Body", "The Body", "The Body", true},
		{"Ticket+1-888-805-5591 or US +1-888-805-5591", "The Body", "The Body", true},
		{"Watch free at www.example.com", "原题", "原题", true},
		{"Call 1 (888) 805 5591", "Book now 1-888-805-5591", "", false},
		{"2001: A Space Odyssey", "", "2001: A Space Odyssey", true},
		{"12 12 12", "", "12 12 12", true},
		{"9-1-1", "", "9-1-1", true},
		{"Blade Runner 2049", "", "Blade Runner 2049", true},
	}
	for _, c := range cases {
		got, ok := displayTitle(c.title, c.original)
		if got != c.want || ok != c.ok {
			t.Errorf("displayTitle(%q, %q) = %q, %v; want %q, %v", c.title, c.original, got, ok, c.want, c.ok)
		}
	}
}
