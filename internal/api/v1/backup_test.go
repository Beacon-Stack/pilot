package v1

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSQLiteMagic_ValidDB(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "valid.db")
	data := make([]byte, 100)
	copy(data, sqliteMagic)
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateSQLiteMagic(p); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidateSQLiteMagic_InvalidHeader(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(p, []byte("this is not sqlite!!"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateSQLiteMagic(p); err == nil {
		t.Error("expected error for invalid header")
	}
}

func TestValidateSQLiteMagic_TooSmall(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tiny.db")
	if err := os.WriteFile(p, []byte("small"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateSQLiteMagic(p); err == nil {
		t.Error("expected error for too-small file")
	}
}

func TestValidateSQLiteMagic_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.db")
	if err := os.WriteFile(p, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateSQLiteMagic(p); err == nil {
		t.Error("expected error for empty file")
	}
}

func TestValidateSQLiteMagic_NonExistent(t *testing.T) {
	if err := validateSQLiteMagic("/nonexistent/path.db"); err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestValidateSQLiteMagic_ExactlyHeader(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "exact.db")
	if err := os.WriteFile(p, sqliteMagic, 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateSQLiteMagic(p); err != nil {
		t.Errorf("expected valid for exact header, got: %v", err)
	}
}
