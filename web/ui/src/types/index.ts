export interface SystemStatus {
  app_name: string;
  version: string;
  build_time: string;
  go_version: string;
  db_type: string;
  db_path?: string;
  uptime_seconds: number;
  start_time: string;
}

export interface Series {
  id: string;
  tmdb_id: number;
  imdb_id?: string;
  title: string;
  sort_title: string;
  year: number;
  overview: string;
  runtime_minutes?: number;
  genres: string[];
  poster_url?: string;
  fanart_url?: string;
  status: "continuing" | "ended" | "upcoming";
  series_type: "standard" | "daily" | "anime";
  monitor_type: string;
  network?: string;
  monitored: boolean;
  library_id: string;
  quality_profile_id: string;
  path?: string;
  added_at: string;
  updated_at: string;
  episode_count: number;
  episode_file_count: number;
  /** Alternate marketing/translated names from TMDB used for release-title matching. */
  alternate_titles?: string[];
}

export interface Season {
  id: string;
  series_id: string;
  season_number: number;
  monitored: boolean;
  episode_count: number;
  episode_file_count: number;
  total_size_bytes: number;
}

// Cour is the anime presentation projection — multi-cour shows like
// Jujutsu Kaisen have one Season row in TMDB but multiple cours per
// the Anime-Lists XML. The backend computes cours at read time; the
// UI renders them as if they were seasons.
export interface Cour {
  tvdb_season: number;
  name: string;
  monitored: boolean;
  episode_count: number;
  episode_file_count: number;
  total_size_bytes: number;
  episode_ids: string[];
}

export interface Episode {
  id: string;
  series_id: string;
  season_id: string;
  season_number: number;
  episode_number: number;
  absolute_number?: number;
  air_date?: string;
  title: string;
  overview: string;
  monitored: boolean;
  has_file: boolean;
  still_path?: string;
  runtime_minutes?: number;
}

export interface EpisodeFile {
  id: string;
  episode_id: string;
  series_id: string;
  path: string;
  size_bytes: number;
  quality: Quality;
  imported_at: string;
  indexed_at: string;
}

export interface RenamePreviewItem {
  episode_file_id: string;
  existing_path: string;
  new_path: string;
}

export interface Library {
  id: string;
  name: string;
  root_path: string;
  default_quality_profile_id: string;
  naming_format?: string;
  folder_format?: string;
  min_free_space_gb: number;
  tags: string[];
  created_at: string;
  updated_at: string;
}

export interface QualityProfile {
  id: string;
  name: string;
  cutoff: Quality;
  qualities: Quality[];
  upgrade_allowed: boolean;
  upgrade_until?: Quality;
  min_custom_format_score: number;
  upgrade_until_cf_score: number;
  managed_by_pulse?: boolean;
}

export interface Quality {
  name: string;
  resolution: string;
  source: string;
  codec: string;
  hdr: string;
}

export interface SeriesListResponse {
  series: Series[];
  total: number;
  page: number;
  per_page: number;
}

export interface IndexerConfig {
  id: string;
  name: string;
  kind: string; // "newznab" or "torznab"
  enabled: boolean;
  priority: number;
  settings: Record<string, unknown>; // { url: string, api_key: string, ... }
  created_at: string;
  updated_at: string;
}

export interface ReleaseResult {
  title: string;
  guid: string;
  indexer: string;
  indexer_id: string;
  download_url: string;
  info_url: string;
  size: number;
  protocol: string; // "usenet" or "torrent"
  seeds: number;
  peers: number;
  age_days: number;
  quality: {
    name: string;
    resolution: string;
    source: string;
    codec: string;
    hdr: string;
  };
  quality_score: number;
  // pack_type classifies the release by what it covers. Backend values:
  // "season" | "multi_episode" | "episode" | "" (unknown).
  pack_type?: "season" | "multi_episode" | "episode" | "";
  // episode_count is how many episodes the release covers. 0 for season
  // packs (count not derivable from title) and for unknown classifications.
  episode_count?: number;
  multi_indexer?: boolean;
  low_confidence?: boolean;
  // filter_reasons is populated by the backend when a release fails a
  // safety filter (below min_seeders, previously stalled, ...). The UI
  // renders such rows grayed at the bottom with an "override" button.
  filter_reasons?: string[];
}

// ── Download Clients ────────────────────────────────────────────────────────

export interface DownloadClientConfig {
  id: string;
  name: string;
  kind: string;
  enabled: boolean;
  priority: number;
  settings: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface DownloadClientRequest {
  name: string;
  kind: string;
  enabled?: boolean;
  priority?: number;
  settings: Record<string, unknown>;
}

export interface TestResult {
  ok: boolean;
  message?: string;
}

export interface DownloadHandling {
  enable_completed: boolean;
  check_interval_minutes: number;
  redownload_failed: boolean;
  redownload_failed_interactive: boolean;
}

export interface RemotePathMapping {
  id: string;
  host: string;
  remote_path: string;
  local_path: string;
}

export interface CreateRemotePathMappingRequest {
  host: string;
  remote_path: string;
  local_path: string;
}

// ── Notifications ───────────────────────────────────────────────────────────

export interface NotificationConfig {
  id: string;
  name: string;
  kind: string;
  enabled: boolean;
  on_events: string[];
  settings: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface NotificationRequest {
  name: string;
  kind: string;
  enabled: boolean;
  settings: Record<string, unknown>;
  on_events?: string[];
}

// ── Media Servers ────────────────────────────────────────────────────────────

export interface MediaServerConfig {
  id: string;
  name: string;
  kind: string;
  enabled: boolean;
  settings: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface MediaServerRequest {
  name: string;
  kind: string;
  enabled: boolean;
  settings: Record<string, unknown>;
}

// ── Blocklist ────────────────────────────────────────────────────────────────

export interface BlocklistEntry {
  id: string;
  series_id?: string;
  series_title?: string;
  episode_info?: string;
  release_guid: string;
  release_title: string;
  indexer_id?: string;
  protocol: string;
  size: number;
  added_at: string;
  notes?: string;
}

export interface BlocklistPage {
  items: BlocklistEntry[];
  total: number;
  page: number;
  per_page: number;
}

// ── Quality Definitions ──────────────────────────────────────────────────────

export interface QualityDefinition {
  id: string;
  name: string;
  resolution: string;
  source: string;
  codec: string;
  hdr: string;
  min_size: number;
  max_size: number;
  preferred_size: number;
  sort_order: number;
}

export interface QualityDefinitionUpdate {
  id: string;
  min_size: number;
  max_size: number;
  preferred_size: number;
}

// ── Custom Formats ───────────────────────────────────────────────────────────

export interface CustomFormat {
  id: string;
  name: string;
  specifications?: unknown[];
}

export interface CustomFormatPreset {
  id: string;
  name: string;
  description: string;
  category: string;
  default_score: number;
}

// ── Queue ────────────────────────────────────────────────────────────────────

export interface QueueItem {
  grab_id: string;
  series_id: string;
  episode_id?: string;
  release_title: string;
  protocol: string;
  size: number;
  downloaded_bytes: number;
  status: string; // "queued" | "downloading" | "completed" | "paused" | "failed" | "removed"
  client_item_id: string;
  download_client_id: string;
  grabbed_at: string;
}

// ── Calendar ─────────────────────────────────────────────────────────────────

export interface CalendarEpisode {
  id: string;
  series_id: string;
  series_title: string;
  series_poster_url?: string;
  season_number: number;
  episode_number: number;
  title: string;
  air_date: string; // "2024-01-15"
  has_file: boolean;
  monitored: boolean;
}

// ── Wanted ────────────────────────────────────────────────────────────────────

export interface WantedEpisode {
  id: string;
  series_id: string;
  series_title: string;
  season_number: number;
  episode_number: number;
  title: string;
  air_date?: string;
  has_file: boolean;
  monitored: boolean;
}

export interface WantedResponse {
  episodes: WantedEpisode[];
  total: number;
  page: number;
  per_page: number;
}

// ── Media Management ─────────────────────────────────────────────────────────

export interface MediaManagement {
  rename_episodes: boolean;
  standard_episode_format: string;
  daily_episode_format: string;
  anime_episode_format: string;
  series_folder_format: string;
  season_folder_format: string;
  colon_replacement: "delete" | "dash" | "space-dash" | "smart";
  import_extra_files: boolean;
  extra_file_extensions: string;
  unmonitor_deleted_episodes: boolean;
}

// ── Import Lists ─────────────────────────────────────────────────────────
export interface ImportListConfig {
  id: string;
  name: string;
  kind: string;
  enabled: boolean;
  settings: Record<string, unknown>;
  search_on_add: boolean;
  monitor: boolean;
  monitor_type: string;
  quality_profile_id: string;
  library_id: string;
  created_at: string;
  updated_at: string;
}

export interface ImportListRequest {
  name: string;
  kind: string;
  enabled: boolean;
  settings: Record<string, unknown>;
  search_on_add: boolean;
  monitor: boolean;
  monitor_type: string;
  quality_profile_id: string;
  library_id: string;
}

export interface ImportExclusion {
  id: string;
  tmdb_id: number;
  title: string;
  year: number;
  created_at: string;
}

export interface ImportListPreviewItem {
  tmdb_id: number;
  title: string;
  year: number;
  poster_path?: string;
}

export interface ImportListSyncResult {
  lists_processed: number;
  series_added: number;
  series_skipped: number;
  errors: string[];
}

// ── Sonarr Import ────────────────────────────────────────────────────────
export interface SonarrPreviewResult {
  version: string;
  series_count: number;
  quality_profiles: { id: number; name: string }[];
  root_folders: { path: string; free_space_gb: number }[];
  indexers: { id: number; name: string; kind: string }[];
  download_clients: { id: number; name: string; kind: string }[];
}

export interface SonarrImportOptions {
  quality_profiles: boolean;
  libraries: boolean;
  indexers: boolean;
  download_clients: boolean;
  series: boolean;
}

export interface SonarrImportResult {
  quality_profiles: CategoryResult;
  libraries: CategoryResult;
  indexers: CategoryResult;
  download_clients: CategoryResult;
  series: CategoryResult;
  errors: string[];
}

export interface CategoryResult {
  imported: number;
  skipped: number;
  failed: number;
}

// ── Custom Format Scoring ───────────────────────────────────────────────────

export interface ScoreDimension {
  name: string;
  score: number;
  max: number;
  matched: boolean;
  got: string;
  want: string;
}

export interface ScoreBreakdown {
  total: number;
  dimensions: ScoreDimension[];
}

// ── Filesystem Browsing ─────────────────────────────────────────────────────

export interface FsDirEntry {
  name: string;
  path: string;
}

export interface FsBrowseResult {
  path: string;
  parent: string | null;
  dirs: FsDirEntry[];
}
