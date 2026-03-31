package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// writeAtomic writes data to path atomically via temp-file + rename.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".screenarr-config-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("setting temp file permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// WriteConfigKey writes a single dot-notation key (e.g. "auth.api_key") to the
// given YAML config file, creating the file and parent directories if needed.
func WriteConfigKey(configFile, key, value string) (writePath string, err error) {
	path := configFile
	if path == "" {
		if info, err := os.Stat("/config"); err == nil && info.IsDir() {
			path = "/config/config.yaml"
		} else if home, _ := os.UserHomeDir(); home != "" {
			path = filepath.Join(home, ".config", "screenarr", "config.yaml")
		} else {
			path = "/config/config.yaml"
		}
	}

	// Read existing config into a generic map to preserve all other keys.
	data := map[string]interface{}{}
	if raw, readErr := os.ReadFile(path); readErr == nil {
		_ = yaml.Unmarshal(raw, &data)
	}

	// Set the key using dot-notation (one level of nesting).
	parts := strings.SplitN(key, ".", 2)
	if len(parts) == 2 {
		sub, ok := data[parts[0]].(map[string]interface{})
		if !ok {
			sub = map[string]interface{}{}
		}
		sub[parts[1]] = value
		data[parts[0]] = sub
	} else {
		data[key] = value
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	raw, err := yaml.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshaling config: %w", err)
	}

	if err := writeAtomic(path, raw, 0o600); err != nil {
		return "", fmt.Errorf("writing config file: %w", err)
	}

	return path, nil
}
