import { describe, it, expect } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { createElement } from "react";
import AllSeasonsView, { buildSeasonSummaries } from "./AllSeasonsView";
import type { Season } from "@/types";

function makeSeason(overrides: Partial<Season> = {}): Season {
  return {
    id: "s1",
    series_id: "series-1",
    season_number: 1,
    monitored: true,
    episode_count: 0,
    episode_file_count: 0,
    total_size_bytes: 0,
    ...overrides,
  };
}

describe("buildSeasonSummaries", () => {
  it("maps episode_count and episode_file_count onto totals", () => {
    const seasons: Season[] = [
      makeSeason({ id: "s1", season_number: 1, episode_count: 10, episode_file_count: 2 }),
      makeSeason({ id: "s2", season_number: 2, episode_count: 8, episode_file_count: 0 }),
    ];

    const summaries = buildSeasonSummaries(seasons);

    expect(summaries).toHaveLength(2);
    expect(summaries[0]).toMatchObject({
      season: seasons[0],
      totalEpisodes: 10,
      downloadedEpisodes: 2,
    });
    expect(summaries[1]).toMatchObject({
      season: seasons[1],
      totalEpisodes: 8,
      downloadedEpisodes: 0,
    });
  });

  it("maps total_size_bytes onto totalSize", () => {
    const [summary] = buildSeasonSummaries([
      makeSeason({ episode_count: 10, episode_file_count: 2, total_size_bytes: 2_500_000_000 }),
    ]);
    expect(summary.totalSize).toBe(2_500_000_000);
  });

  it("handles a brand-new series with zero episodes", () => {
    const [summary] = buildSeasonSummaries([
      makeSeason({ episode_count: 0, episode_file_count: 0 }),
    ]);
    expect(summary.totalEpisodes).toBe(0);
    expect(summary.downloadedEpisodes).toBe(0);
  });
});

describe("AllSeasonsView rendering", () => {
  it("renders per-season episode counts from summaries", () => {
    const seasons: Season[] = [
      makeSeason({ id: "s1", season_number: 1, episode_count: 10, episode_file_count: 2 }),
      makeSeason({ id: "s2", season_number: 2, episode_count: 8, episode_file_count: 0 }),
    ];
    const summaries = buildSeasonSummaries(seasons);

    render(
      createElement(AllSeasonsView, {
        summaries,
        onSelectSeason: () => {},
        onToggleMonitor: () => {},
      })
    );

    expect(screen.getByText("2/10 episodes")).toBeInTheDocument();
    expect(screen.getByText("0/8 episodes")).toBeInTheDocument();
    expect(screen.getByText("Season 1")).toBeInTheDocument();
    expect(screen.getByText("Season 2")).toBeInTheDocument();
  });

  it("labels season 0 as Specials", () => {
    const seasons: Season[] = [
      makeSeason({ id: "s0", season_number: 0, episode_count: 3, episode_file_count: 1 }),
    ];
    render(
      createElement(AllSeasonsView, {
        summaries: buildSeasonSummaries(seasons),
        onSelectSeason: () => {},
        onToggleMonitor: () => {},
      })
    );
    expect(screen.getByText("Specials")).toBeInTheDocument();
    expect(screen.getByText("1/3 episodes")).toBeInTheDocument();
  });

  it("shows empty state when there are no seasons", () => {
    const { container } = render(
      createElement(AllSeasonsView, {
        summaries: [],
        onSelectSeason: () => {},
        onToggleMonitor: () => {},
      })
    );
    expect(within(container).getByText("No seasons.")).toBeInTheDocument();
  });
});
