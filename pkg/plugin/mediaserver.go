package plugin

import "context"

// MediaServer is implemented by media server plugins (Plex, Emby, Jellyfin).
// After a series file is imported, the dispatcher calls RefreshLibrary so the
// media server picks up the new file without waiting for a scheduled scan.
type MediaServer interface {
	// Name returns the human-readable name of the media server plugin.
	Name() string

	// RefreshLibrary tells the media server to re-scan the library section
	// that contains seriesPath. Implementations may fall back to a full
	// library refresh if path-scoped scanning is unavailable.
	RefreshLibrary(ctx context.Context, seriesPath string) error

	// Test verifies that the media server is reachable and the credentials
	// are valid.
	Test(ctx context.Context) error
}
