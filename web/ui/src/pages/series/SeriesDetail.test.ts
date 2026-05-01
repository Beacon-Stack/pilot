import { describe, it, expect } from "vitest";
import { parseEpisodeFromReleaseTitle } from "./SeriesDetail";

// parseEpisodeFromReleaseTitle is the recovery helper for grab_history
// rows that lack episode_id (older grabs predating the
// ManualSearchModal fix). False positives badge the wrong episode, so
// the helper is conservative on purpose.
describe("parseEpisodeFromReleaseTitle", () => {
  it("matches SxxExx form", () => {
    expect(parseEpisodeFromReleaseTitle("Show.Name.S01E48.1080p.WEB.x265-GRP")).toEqual({
      season: 1,
      episode: 48,
    });
  });

  it("matches SxExx form (single-digit season)", () => {
    expect(parseEpisodeFromReleaseTitle("Show Name S03E12 720p")).toEqual({
      season: 3,
      episode: 12,
    });
  });

  it("matches anime fansub absolute form with parens", () => {
    expect(
      parseEpisodeFromReleaseTitle("[SubsPlease] Jujutsu Kaisen - 48 (1080p) [319A622F].mkv"),
    ).toEqual({ episode: 48 });
  });

  it("matches anime fansub absolute form with brackets", () => {
    expect(
      parseEpisodeFromReleaseTitle("[Erai-raws] Some Show - 12 [1080p][hash].mkv"),
    ).toEqual({ episode: 12 });
  });

  it("strips the v2 version suffix", () => {
    expect(
      parseEpisodeFromReleaseTitle("[Group] Show - 5v2 (1080p)"),
    ).toEqual({ episode: 5 });
  });

  it("returns null for unrecognised titles", () => {
    expect(parseEpisodeFromReleaseTitle("just-some-random-name.mkv")).toBeNull();
  });

  it("returns null for season-pack titles (no episode number)", () => {
    expect(
      parseEpisodeFromReleaseTitle("Show.Name.S01.Complete.1080p.WEBRip"),
    ).toBeNull();
  });

  it("prefers SxxExx over a stray dash-number elsewhere in title", () => {
    expect(
      parseEpisodeFromReleaseTitle("Show.S02E05.Something - 99 (1080p)"),
    ).toEqual({ season: 2, episode: 5 });
  });
});
