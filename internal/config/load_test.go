package config

import (
	"os"
	"path/filepath"
	"testing"
)

const testFixture = `
server:
  port: 8383
database:
  path: "/tmp/pilot-test.db"
`

func writeFixture(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_ReadsDatabasePath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeFixture(t, dir, "config.yaml", testFixture)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Database.Path; got != "/tmp/pilot-test.db" {
		t.Fatalf("Database.Path = %q; want %q", got, "/tmp/pilot-test.db")
	}
}

func TestLoad_PulseAPIKeyFileOverridesInlineKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeFixture(t, dir, "config.yaml", testFixture)
	keyFile := writeFixture(t, dir, "pulse.txt", "secret-key\n")

	t.Setenv("PILOT_PULSE_API_KEY", "inline-loses")
	t.Setenv("PILOT_PULSE_API_KEY_FILE", keyFile)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Pulse.APIKey.Value(); got != "secret-key" {
		t.Fatalf("Pulse.APIKey = %q; want secret-key (file must override the inline env value)", got)
	}
}

func TestLoad_InvalidPulseAPIKeyFilePath_Errors(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeFixture(t, dir, "config.yaml", testFixture)

	t.Setenv("PILOT_PULSE_API_KEY_FILE", "/nonexistent/pulse-api-key")

	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error when pulse api_key_file path is invalid")
	}
}
