package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

const (
	DefaultHost      = "0.0.0.0"
	DefaultPort      = 8383
	DefaultDBDriver  = "postgres"
	DefaultLogLevel  = "info"
	DefaultLogFormat = "json"
)

// Load reads configuration from a YAML file and environment variables.
// If cfgFile is empty, the following paths are searched in order:
//
//	/config/config.yaml              (Docker volume mount)
//	$HOME/.config/pilot/config.yaml
//	/etc/pilot/config.yaml
//	./config.yaml
//
// Missing config file is not an error — defaults and environment variables
// are always applied.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("server.host", DefaultHost)
	v.SetDefault("server.port", DefaultPort)
	v.SetDefault("database.driver", DefaultDBDriver)
	v.SetDefault("log.level", DefaultLogLevel)
	v.SetDefault("log.format", DefaultLogFormat)

	// Config file location
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		home, _ := os.UserHomeDir()
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("/config") // Docker volume mount point
		if home != "" {
			v.AddConfigPath(filepath.Join(home, ".config", "pilot"))
		}
		v.AddConfigPath("/etc/pilot")
		v.AddConfigPath(".")
	}

	// Environment variable overrides.
	v.SetEnvPrefix("PILOT")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Explicit bindings for keys that contain underscores.
	_ = v.BindEnv("auth.api_key", "PILOT_AUTH_API_KEY")
	_ = v.BindEnv("tvdb.api_key", "PILOT_TVDB_API_KEY")
	_ = v.BindEnv("trakt.client_id", "PILOT_TRAKT_CLIENT_ID")
	_ = v.BindEnv("database.path", "PILOT_DATABASE_PATH")
	_ = v.BindEnv("database.dsn", "PILOT_DATABASE_DSN")
	_ = v.BindEnv("pulse.url", "PILOT_PULSE_URL")
	_ = v.BindEnv("pulse.api_key", "PILOT_PULSE_API_KEY")

	if err := v.ReadInConfig(); err != nil {
		// Missing config file is not an error — we use defaults.
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}
	configFileUsed := v.ConfigFileUsed()

	var cfg Config
	if err := v.Unmarshal(&cfg, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			secretDecodeHook,
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	)); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Default SQLite path: if /config exists (Docker volume), use it;
	// otherwise fall back to ~/.config/pilot/ (bare-metal).
	if cfg.Database.Driver == "sqlite" && cfg.Database.Path == "" {
		if info, err := os.Stat("/config"); err == nil && info.IsDir() {
			cfg.Database.Path = "/config/pilot.db"
		} else if home, _ := os.UserHomeDir(); home != "" {
			cfg.Database.Path = filepath.Join(home, ".config", "pilot", "pilot.db")
		} else {
			cfg.Database.Path = "/config/pilot.db"
		}
	}

	// Apply build-time default keys when no user-provided key is present.
	if cfg.TVDB.APIKey.IsEmpty() && DefaultTMDBKey != "" {
		cfg.TVDB.APIKey = Secret(DefaultTMDBKey)
	}
	if cfg.Trakt.ClientID.IsEmpty() && DefaultTraktClientID != "" {
		cfg.Trakt.ClientID = Secret(DefaultTraktClientID)
	}

	cfg.ConfigFile = configFileUsed

	return &cfg, nil
}

// EnsureAPIKey generates a random API key if none is configured.
func EnsureAPIKey(cfg *Config) (generated bool, err error) {
	if !cfg.Auth.APIKey.IsEmpty() {
		return false, nil
	}

	key, err := generateAPIKey()
	if err != nil {
		return false, fmt.Errorf("generating API key: %w", err)
	}

	cfg.Auth.APIKey = Secret(key)
	return true, nil
}

// secretDecodeHook allows mapstructure to convert plain strings into the
// Secret type.
func secretDecodeHook(from reflect.Type, to reflect.Type, data any) (any, error) {
	if to == reflect.TypeOf(Secret("")) && from.Kind() == reflect.String {
		return Secret(data.(string)), nil
	}
	return data, nil
}

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
