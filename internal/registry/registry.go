// Package registry holds plugin factories and instantiates plugin instances
// from stored JSON configuration. Built-in plugins self-register via init().
package registry

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/beacon-stack/pilot/pkg/plugin"
)

// IndexerFactory constructs an Indexer from a JSON settings blob.
type IndexerFactory func(settings json.RawMessage) (plugin.Indexer, error)

// DownloaderFactory constructs a DownloadClient from a JSON settings blob.
type DownloaderFactory func(settings json.RawMessage) (plugin.DownloadClient, error)

// MediaServerFactory constructs a MediaServer from a JSON settings blob.
type MediaServerFactory func(settings json.RawMessage) (plugin.MediaServer, error)

// ImportListFactory constructs an ImportList from a JSON settings blob.
type ImportListFactory func(settings json.RawMessage) (plugin.ImportList, error)

// SanitizerFunc redacts sensitive fields from a plugin settings blob so it is
// safe to include in API responses and logs. It must never return nil.
// Plugins register one via RegisterIndexerSanitizer / RegisterDownloaderSanitizer. If no sanitizer is registered the registry falls
// back to returning an empty JSON object so credentials are never exposed.
type SanitizerFunc func(settings json.RawMessage) json.RawMessage

// emptySanitizer is the safe fallback when no plugin sanitizer is registered.
var emptySanitizer SanitizerFunc = func(_ json.RawMessage) json.RawMessage {
	return json.RawMessage("{}")
}

// Registry maps plugin kind strings to their factory functions.
// Use Default for the application-wide singleton.
type Registry struct {
	mu                    sync.RWMutex
	indexers              map[string]IndexerFactory
	indexerSanitizers     map[string]SanitizerFunc
	downloaders           map[string]DownloaderFactory
	downloaderSanitizers  map[string]SanitizerFunc
	mediaServers          map[string]MediaServerFactory
	mediaServerSanitizers map[string]SanitizerFunc
	importLists           map[string]ImportListFactory
	importListSanitizers  map[string]SanitizerFunc
}

// New returns an empty, ready-to-use Registry.
func New() *Registry {
	return &Registry{
		indexers:              make(map[string]IndexerFactory),
		indexerSanitizers:     make(map[string]SanitizerFunc),
		downloaders:           make(map[string]DownloaderFactory),
		downloaderSanitizers:  make(map[string]SanitizerFunc),
		mediaServers:          make(map[string]MediaServerFactory),
		mediaServerSanitizers: make(map[string]SanitizerFunc),
		importLists:           make(map[string]ImportListFactory),
		importListSanitizers:  make(map[string]SanitizerFunc),
	}
}

// Default is the application-wide plugin registry.
// Built-in plugins register themselves here in their init() functions.
var Default = New()

// RegisterIndexer adds a factory for the given kind string.
// Panics if kind is already registered (caught at startup, not runtime).
func (r *Registry) RegisterIndexer(kind string, factory IndexerFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.indexers[kind]; exists {
		panic(fmt.Sprintf("registry: indexer kind %q already registered", kind))
	}
	r.indexers[kind] = factory
}

// RegisterIndexerSanitizer registers a settings sanitizer for the given indexer
// kind. Call this from the same init() as RegisterIndexer.
func (r *Registry) RegisterIndexerSanitizer(kind string, fn SanitizerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.indexerSanitizers[kind] = fn
}

// SanitizeIndexerSettings returns a redacted copy of the settings blob safe for
// API responses and logs. Falls back to "{}" if no sanitizer is registered.
func (r *Registry) SanitizeIndexerSettings(kind string, settings json.RawMessage) json.RawMessage {
	r.mu.RLock()
	fn, ok := r.indexerSanitizers[kind]
	r.mu.RUnlock()
	if !ok {
		return emptySanitizer(settings)
	}
	return fn(settings)
}

// NewIndexer constructs an Indexer from the given kind and JSON settings.
// Returns an error if the kind is unknown or settings are invalid.
func (r *Registry) NewIndexer(kind string, settings json.RawMessage) (plugin.Indexer, error) {
	r.mu.RLock()
	factory, ok := r.indexers[kind]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown indexer kind %q", kind)
	}
	return factory(settings)
}

// RegisterDownloader adds a factory for the given download client kind string.
// Panics if kind is already registered (caught at startup, not runtime).
func (r *Registry) RegisterDownloader(kind string, factory DownloaderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.downloaders[kind]; exists {
		panic(fmt.Sprintf("registry: downloader kind %q already registered", kind))
	}
	r.downloaders[kind] = factory
}

// RegisterDownloaderSanitizer registers a settings sanitizer for the given
// download client kind.
func (r *Registry) RegisterDownloaderSanitizer(kind string, fn SanitizerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.downloaderSanitizers[kind] = fn
}

// SanitizeDownloaderSettings returns a redacted copy of the settings blob safe
// for API responses and logs. Falls back to "{}" if no sanitizer is registered.
func (r *Registry) SanitizeDownloaderSettings(kind string, settings json.RawMessage) json.RawMessage {
	r.mu.RLock()
	fn, ok := r.downloaderSanitizers[kind]
	r.mu.RUnlock()
	if !ok {
		return emptySanitizer(settings)
	}
	return fn(settings)
}

// NewDownloader constructs a DownloadClient from the given kind and JSON settings.
// Returns an error if the kind is unknown or settings are invalid.
func (r *Registry) NewDownloader(kind string, settings json.RawMessage) (plugin.DownloadClient, error) {
	r.mu.RLock()
	factory, ok := r.downloaders[kind]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown downloader kind %q", kind)
	}
	return factory(settings)
}

// RegisterMediaServer adds a factory for the given media server kind string.
// Panics if kind is already registered (caught at startup, not runtime).
func (r *Registry) RegisterMediaServer(kind string, factory MediaServerFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.mediaServers[kind]; exists {
		panic(fmt.Sprintf("registry: media server kind %q already registered", kind))
	}
	r.mediaServers[kind] = factory
}

// RegisterMediaServerSanitizer registers a settings sanitizer for the given
// media server kind.
func (r *Registry) RegisterMediaServerSanitizer(kind string, fn SanitizerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mediaServerSanitizers[kind] = fn
}

// SanitizeMediaServerSettings returns a redacted copy of the settings blob safe
// for API responses and logs. Falls back to "{}" if no sanitizer is registered.
func (r *Registry) SanitizeMediaServerSettings(kind string, settings json.RawMessage) json.RawMessage {
	r.mu.RLock()
	fn, ok := r.mediaServerSanitizers[kind]
	r.mu.RUnlock()
	if !ok {
		return emptySanitizer(settings)
	}
	return fn(settings)
}

// NewMediaServer constructs a MediaServer from the given kind and JSON settings.
// Returns an error if the kind is unknown or settings are invalid.
func (r *Registry) NewMediaServer(kind string, settings json.RawMessage) (plugin.MediaServer, error) {
	r.mu.RLock()
	factory, ok := r.mediaServers[kind]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown media server kind %q", kind)
	}
	return factory(settings)
}

// RegisterImportList adds a factory for the given import list kind string.
// Panics if kind is already registered (caught at startup, not runtime).
func (r *Registry) RegisterImportList(kind string, factory ImportListFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.importLists[kind]; exists {
		panic(fmt.Sprintf("registry: import list kind %q already registered", kind))
	}
	r.importLists[kind] = factory
}

// RegisterImportListSanitizer registers a settings sanitizer for the given
// import list kind.
func (r *Registry) RegisterImportListSanitizer(kind string, fn SanitizerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.importListSanitizers[kind] = fn
}

// SanitizeImportListSettings returns a redacted copy of the settings blob safe
// for API responses and logs. Falls back to "{}" if no sanitizer is registered.
func (r *Registry) SanitizeImportListSettings(kind string, settings json.RawMessage) json.RawMessage {
	r.mu.RLock()
	fn, ok := r.importListSanitizers[kind]
	r.mu.RUnlock()
	if !ok {
		return emptySanitizer(settings)
	}
	return fn(settings)
}

// NewImportList constructs an ImportList from the given kind and JSON settings.
// Returns an error if the kind is unknown or settings are invalid.
func (r *Registry) NewImportList(kind string, settings json.RawMessage) (plugin.ImportList, error) {
	r.mu.RLock()
	factory, ok := r.importLists[kind]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown import list kind %q", kind)
	}
	return factory(settings)
}
