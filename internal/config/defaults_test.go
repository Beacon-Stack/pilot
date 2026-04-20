package config

import (
	"crypto/rand"
	"encoding/base64"
	"testing"
)

// obfuscateForTest mirrors ./tools/obfuscate at test time so we can
// round-trip through DefaultTMDBKey / DefaultTraktClientID without
// re-running the build tool.
func obfuscateForTest(t *testing.T, plaintext string) (obfB64, keyB64 string) {
	t.Helper()
	plain := []byte(plaintext)
	key := make([]byte, len(plain))
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	ct := make([]byte, len(plain))
	for i := range plain {
		ct[i] = plain[i] ^ key[i]
	}
	return base64.StdEncoding.EncodeToString(ct), base64.StdEncoding.EncodeToString(key)
}

func TestDefaultTMDBKey_RoundTrip(t *testing.T) {
	want := "some-tmdb-key-abc123"
	obf, xorkey := obfuscateForTest(t, want)

	orig1, orig2 := obfuscatedTMDBKey, tmdbKeyXORKey
	obfuscatedTMDBKey = obf
	tmdbKeyXORKey = xorkey
	defer func() { obfuscatedTMDBKey, tmdbKeyXORKey = orig1, orig2 }()

	if got := DefaultTMDBKey(); got != want {
		t.Errorf("DefaultTMDBKey() = %q; want %q", got, want)
	}
}

func TestDefaultTraktClientID_RoundTrip(t *testing.T) {
	want := "trakt-client-id-xyz789"
	obf, xorkey := obfuscateForTest(t, want)

	orig1, orig2 := obfuscatedTraktClientID, traktClientIDXORKey
	obfuscatedTraktClientID = obf
	traktClientIDXORKey = xorkey
	defer func() { obfuscatedTraktClientID, traktClientIDXORKey = orig1, orig2 }()

	if got := DefaultTraktClientID(); got != want {
		t.Errorf("DefaultTraktClientID() = %q; want %q", got, want)
	}
}

func TestDefaultTMDBKey_EmptyWhenUnset(t *testing.T) {
	orig1, orig2 := obfuscatedTMDBKey, tmdbKeyXORKey
	obfuscatedTMDBKey = ""
	tmdbKeyXORKey = ""
	defer func() { obfuscatedTMDBKey, tmdbKeyXORKey = orig1, orig2 }()

	if got := DefaultTMDBKey(); got != "" {
		t.Errorf("DefaultTMDBKey() = %q; want empty string when ldflags unset", got)
	}
}

func TestDeobfuscate_HandlesInvalidInput(t *testing.T) {
	// Mismatched lengths should yield empty string rather than panicking.
	if got := deobfuscate("AAAA", "AAAAAA"); got != "" {
		t.Errorf("deobfuscate(mismatched) = %q; want empty", got)
	}

	// Invalid base64 should be handled gracefully.
	if got := deobfuscate("not!!base64!!", "also-not"); got != "" {
		t.Errorf("deobfuscate(invalid b64) = %q; want empty", got)
	}
}

func TestObfuscatedStringsNotPresentInPlaintextForm(t *testing.T) {
	// The XOR-encoded value must not accidentally equal the plaintext.
	// This isn't a security property of real value — it's a smoke test
	// that the obfuscator actually mutated the bytes.
	want := "AAAAAAAAAAAAAAAA"
	obf, xorkey := obfuscateForTest(t, want)

	if obf == base64.StdEncoding.EncodeToString([]byte(want)) {
		t.Error("obfuscated form matched plaintext — XOR is a no-op")
	}

	// And the round-trip still works.
	orig1, orig2 := obfuscatedTMDBKey, tmdbKeyXORKey
	obfuscatedTMDBKey = obf
	tmdbKeyXORKey = xorkey
	defer func() { obfuscatedTMDBKey, tmdbKeyXORKey = orig1, orig2 }()
	if got := DefaultTMDBKey(); got != want {
		t.Errorf("round-trip mismatch: %q != %q", got, want)
	}
}
