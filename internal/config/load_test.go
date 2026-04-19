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
  driver: postgres
  dsn: "postgres://user:plain@host:5432/db"
`

func writeFixture(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_PasswordFileOverridesDSN(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeFixture(t, dir, "config.yaml", testFixture)
	pwFile := writeFixture(t, dir, "pw.txt", "secretpw\n")

	t.Setenv("PILOT_DATABASE_PASSWORD_FILE", pwFile)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := "postgres://user:secretpw@host:5432/db"
	if got := cfg.Database.DSN.Value(); got != want {
		t.Fatalf("DSN = %q; want %q", got, want)
	}
}

func TestLoad_NoPasswordFile_LeavesDSNIntact(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeFixture(t, dir, "config.yaml", testFixture)

	t.Setenv("PILOT_DATABASE_PASSWORD_FILE", "")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := "postgres://user:plain@host:5432/db"
	if got := cfg.Database.DSN.Value(); got != want {
		t.Fatalf("DSN = %q; want %q", got, want)
	}
}

func TestLoad_InvalidPasswordFilePath_Errors(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeFixture(t, dir, "config.yaml", testFixture)

	t.Setenv("PILOT_DATABASE_PASSWORD_FILE", "/nonexistent/secret")

	if _, err := Load(cfgPath); err == nil {
		t.Fatal("expected error when password file path is invalid")
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
