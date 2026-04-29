package config

import "encoding/base64"

// Build-time XOR-obfuscated provider keys. These ldflag vars hold
// base64-encoded ciphertext and XOR key pairs produced by
// ./tools/obfuscate at image build time. Never read them directly —
// always go through DefaultTMDBKey() / DefaultTraktClientID() so the
// plaintext never appears at global scope.
//
// An untrusted `strings` on the binary will see the base64 strings
// but not the format/structure that would give away "this is an
// api key." Anyone with a debugger or the source code can still
// recover the plaintext; the goal here is casual-attacker deterrence,
// not real confidentiality.
//
// Pilot's metadata client is TMDB-TV (themoviedb.org's TV API) — TMDB
// covers both movies and TV, so one key works for Pilot and Prism.
var (
	obfuscatedTMDBKey       string
	tmdbKeyXORKey           string
	obfuscatedTraktClientID string
	traktClientIDXORKey     string
)

// DefaultTMDBKey returns the baked-in TMDB API key (de-obfuscated) or
// an empty string if no key was provided at build time. This is the
// only way the key appears in plaintext — never log, never return via
// an API response.
func DefaultTMDBKey() string {
	return deobfuscate(obfuscatedTMDBKey, tmdbKeyXORKey)
}

// DefaultTraktClientID returns the baked-in Trakt client ID.
func DefaultTraktClientID() string {
	return deobfuscate(obfuscatedTraktClientID, traktClientIDXORKey)
}

// deobfuscate XORs the base64-decoded ciphertext with the base64-decoded
// key and returns the plaintext. Empty inputs yield an empty string
// (so unset ldflags flow through cleanly).
func deobfuscate(obfB64, keyB64 string) string {
	if obfB64 == "" || keyB64 == "" {
		return ""
	}
	ct, err := base64.StdEncoding.DecodeString(obfB64)
	if err != nil {
		return ""
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return ""
	}
	if len(ct) != len(key) {
		return ""
	}
	out := make([]byte, len(ct))
	for i := range ct {
		out[i] = ct[i] ^ key[i]
	}
	return string(out)
}
