// There are no per-page docs yet, so every help link opens Pilot's page
// on the Beacon site.
const DOCS = "https://beaconstack.io/pilot";

export const DOCS_URLS = {
  // Settings pages
  libraries: DOCS,
  qualityProfiles: DOCS,
  qualityDefinitions: DOCS,
  customFormats: DOCS,
  indexers: DOCS,
  downloadClients: DOCS,
  notifications: DOCS,
  mediaServers: DOCS,
  importLists: DOCS,
  blocklist: DOCS,
  mediaManagement: DOCS,
  appSettings: DOCS,
  system: DOCS,

  // Main pages
  wanted: DOCS,
  activity: DOCS,
  calendar: DOCS,
} as const;
