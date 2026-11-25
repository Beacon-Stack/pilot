// Where an episode or a series stands, from the viewer's side. The server
// works this out from the media server, the download queue and the disk;
// this file gives each state its words and colour.

export type Availability =
  | "available"
  | "downloaded"
  | "downloading"
  | "wanted"
  | "upcoming"
  | "unmonitored";

interface AvailabilityStyle {
  label: string;
  hint: string;
  color: string; // a CSS colour, from the theme
}

export const AVAILABILITY: Record<Availability, AvailabilityStyle> = {
  available: { label: "Available", hint: "On your media server, ready to watch", color: "var(--color-success)" },
  downloaded: { label: "Downloaded", hint: "On disk, but not on your media server yet", color: "var(--color-accent)" },
  downloading: { label: "Downloading", hint: "A download is in progress", color: "var(--color-accent)" },
  wanted: { label: "Wanted", hint: "Aired; Pilot is looking for it", color: "var(--color-warning)" },
  upcoming: { label: "Upcoming", hint: "Not aired yet", color: "var(--color-text-muted)" },
  unmonitored: { label: "Not monitored", hint: "Pilot isn't looking for it", color: "var(--color-text-muted)" },
};

// asAvailability returns the state if it is one this UI knows, so a newer
// server's extra states fall back to the old display instead of breaking.
export function asAvailability(value: string | undefined): Availability | undefined {
  return value && value in AVAILABILITY ? (value as Availability) : undefined;
}

// availabilityLabel is the badge text: the state, with the percentage
// while downloading.
export function availabilityLabel(state: Availability, progress?: number): string {
  if (state === "downloading" && progress !== undefined && progress > 0) {
    return `Downloading ${Math.floor(progress * 100)}%`;
  }
  return AVAILABILITY[state].label;
}
