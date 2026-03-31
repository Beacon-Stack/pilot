const DOCS_BASE = "https://screenarr.tv/docs";

export const DOCS_URLS = {
  // Settings pages
  libraries: `${DOCS_BASE}/libraries`,
  qualityProfiles: `${DOCS_BASE}/quality-profiles`,
  qualityDefinitions: `${DOCS_BASE}/quality-definitions`,
  customFormats: `${DOCS_BASE}/custom-formats`,
  indexers: `${DOCS_BASE}/indexers`,
  downloadClients: `${DOCS_BASE}/download-clients`,
  notifications: `${DOCS_BASE}/notifications`,
  mediaServers: `${DOCS_BASE}/media-servers`,
  importLists: `${DOCS_BASE}/import-lists`,
  blocklist: `${DOCS_BASE}/blocklist`,
  mediaManagement: `${DOCS_BASE}/media-management`,
  appSettings: `${DOCS_BASE}/app-settings`,
  system: `${DOCS_BASE}/system`,

  // Main pages
  wanted: `${DOCS_BASE}/wanted`,
  activity: `${DOCS_BASE}/activity`,
  calendar: `${DOCS_BASE}/calendar`,
} as const;
