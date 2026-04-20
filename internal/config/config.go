package config

// Config holds all application configuration.
// Values are loaded from config.yaml and can be overridden by
// PILOT_* environment variables (e.g. PILOT_SERVER_PORT=8383).
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Log      LogConfig      `mapstructure:"log"`
	Auth     AuthConfig     `mapstructure:"auth"`
	TVDB     TVDBConfig     `mapstructure:"tvdb"`
	Trakt    TraktConfig    `mapstructure:"trakt"`
	Pulse    PulseConfig    `mapstructure:"pulse"`

	// ConfigFile is the path of the config file that was loaded, if any.
	// Empty when running on defaults/env vars only.
	ConfigFile string `mapstructure:"-"`
}

// PulseConfig holds optional Beacon Pulse integration settings.
type PulseConfig struct {
	URL    string `mapstructure:"url"`
	APIKey Secret `mapstructure:"api_key"`
	// APIKeyFile points at a file (typically /run/secrets/*) containing
	// Pulse's API key. When non-empty, its contents replace APIKey at
	// load time.
	APIKeyFile string `mapstructure:"api_key_file"`
}

// ServerConfig controls the HTTP server.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// DatabaseConfig selects and configures the database driver.
type DatabaseConfig struct {
	// Driver is "sqlite" (default) or "postgres".
	Driver string `mapstructure:"driver"`
	// Path is the SQLite database file path. Ignored for Postgres.
	Path string `mapstructure:"path"`
	// DSN is the Postgres connection string. Ignored for SQLite.
	DSN Secret `mapstructure:"dsn"`
	// PasswordFile is a path to a file containing the Postgres password,
	// typically a Docker secret mounted at /run/secrets/*. When non-empty,
	// its contents replace the password component of DSN at load time.
	PasswordFile string `mapstructure:"password_file"`
}

// LogConfig controls log output format and verbosity.
type LogConfig struct {
	// Level is one of: debug, info, warn, error. Default: info.
	Level string `mapstructure:"level"`
	// Format is one of: json, text. Default: json.
	Format string `mapstructure:"format"`
}

// AuthConfig holds the Pilot API key used to authenticate requests.
type AuthConfig struct {
	APIKey Secret `mapstructure:"api_key"`
}

// TVDBConfig holds TheTVDB API credentials (placeholder — not wired up yet).
type TVDBConfig struct {
	APIKey Secret `mapstructure:"api_key"`
}

// TraktConfig holds the Trakt API key used for import list plugins.
type TraktConfig struct {
	ClientID Secret `mapstructure:"client_id"`
}

// DefaultTMDBKey and DefaultTraktClientID are implemented as accessor
// functions (see defaults.go) that de-obfuscate XOR-masked ldflag
// values at call time. The raw bytes are never accessible by name;
// the public surface is the getter functions only.
