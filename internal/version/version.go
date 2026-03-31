package version

import "runtime"

// These variables are set at build time via -ldflags.
var (
	Version   = "dev"
	BuildTime = "unknown"
)

// GoVersion returns the Go runtime version.
func GoVersion() string {
	return runtime.Version()
}
