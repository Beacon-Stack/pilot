import { useState, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { useCalendar } from "@/api/calendar";
import type { CalendarEpisode } from "@/types";

// ── Helpers ───────────────────────────────────────────────────────────────────

function episodeBorderColor(ep: CalendarEpisode): string {
  if (!ep.monitored) return "var(--color-border-default)";
  if (ep.has_file) return "var(--color-success)";
  return "var(--color-danger)";
}

function isSameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

function formatEpisodeCode(season: number, episode: number): string {
  return `S${String(season).padStart(2, "0")}E${String(episode).padStart(2, "0")}`;
}

function toDateKey(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

const MONTH_NAMES = [
  "January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December",
];
const DAY_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

// ── Episode chip ──────────────────────────────────────────────────────────────

function EpisodeChip({
  episode,
  onClick,
}: {
  episode: CalendarEpisode;
  onClick: () => void;
}) {
  const border = episodeBorderColor(episode);

  return (
    <button
      onClick={onClick}
      title={`${episode.series_title} — ${formatEpisodeCode(episode.season_number, episode.episode_number)} ${episode.title}`}
      style={{
        display: "flex",
        alignItems: "center",
        gap: 4,
        width: "100%",
        background: "var(--color-bg-surface)",
        border: `1px solid ${border}`,
        borderLeft: `3px solid ${border}`,
        borderRadius: 4,
        padding: "2px 4px",
        cursor: "pointer",
        textAlign: "left",
        minWidth: 0,
      }}
      onMouseEnter={(e) => {
        (e.currentTarget as HTMLButtonElement).style.background =
          "var(--color-bg-elevated)";
      }}
      onMouseLeave={(e) => {
        (e.currentTarget as HTMLButtonElement).style.background =
          "var(--color-bg-surface)";
      }}
    >
      {episode.series_poster_url ? (
        <img
          src={episode.series_poster_url?.startsWith("/") ? `https://image.tmdb.org/t/p/w92${episode.series_poster_url}` : episode.series_poster_url}
          alt=""
          style={{
            width: 14,
            height: 20,
            borderRadius: 2,
            objectFit: "cover",
            flexShrink: 0,
          }}
        />
      ) : (
        <div
          style={{
            width: 14,
            height: 20,
            borderRadius: 2,
            background: "var(--color-bg-elevated)",
            flexShrink: 0,
          }}
        />
      )}
      <span
        style={{
          fontSize: 10,
          color: "var(--color-text-primary)",
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
          lineHeight: 1.3,
        }}
      >
        {episode.series_title}{" "}
        <span style={{ color: "var(--color-text-muted)" }}>
          {formatEpisodeCode(episode.season_number, episode.episode_number)}
        </span>
      </span>
    </button>
  );
}

// ── Day cell ──────────────────────────────────────────────────────────────────

function DayCell({
  date,
  episodes,
  isToday,
  isCurrentMonth,
  onEpisodeClick,
}: {
  date: Date;
  episodes: CalendarEpisode[];
  isToday: boolean;
  isCurrentMonth: boolean;
  onEpisodeClick: (seriesId: string) => void;
}) {
  return (
    <div
      style={{
        minHeight: 90,
        background: isCurrentMonth
          ? "var(--color-bg-surface)"
          : "transparent",
        border: "1px solid var(--color-border-subtle)",
        borderRadius: 4,
        padding: "4px 5px",
        display: "flex",
        flexDirection: "column",
        gap: 3,
      }}
    >
      {/* Day number */}
      <div
        style={{
          fontSize: 11,
          fontWeight: isToday ? 700 : 400,
          color: isToday
            ? "var(--color-accent-fg)"
            : isCurrentMonth
            ? "var(--color-text-secondary)"
            : "var(--color-text-muted)",
          lineHeight: 1,
          ...(isToday && {
            display: "inline-flex",
            alignItems: "center",
            justifyContent: "center",
            width: 18,
            height: 18,
            background: "var(--color-accent)",
            borderRadius: "50%",
          }),
        }}
      >
        {date.getDate()}
      </div>

      {/* Episode chips */}
      {episodes.slice(0, 4).map((ep) => (
        <EpisodeChip
          key={ep.id}
          episode={ep}
          onClick={() => onEpisodeClick(ep.series_id)}
        />
      ))}
      {episodes.length > 4 && (
        <span
          style={{
            fontSize: 9,
            color: "var(--color-text-muted)",
            paddingLeft: 2,
          }}
        >
          +{episodes.length - 4} more
        </span>
      )}
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function CalendarPage() {
  const navigate = useNavigate();

  const now = new Date();
  const [year, setYear] = useState(now.getFullYear());
  const [month, setMonth] = useState(now.getMonth()); // 0-indexed

  // Build start/end dates for the API query (full month).
  const startDate = `${year}-${String(month + 1).padStart(2, "0")}-01`;
  const lastDayNum = new Date(year, month + 1, 0).getDate();
  const endDate = `${year}-${String(month + 1).padStart(2, "0")}-${String(lastDayNum).padStart(2, "0")}`;

  const { data: episodes, isLoading } = useCalendar(startDate, endDate);
  const allEpisodes = episodes ?? [];

  // Map "YYYY-MM-DD" -> episodes on that day.
  const episodesByDate = useMemo(() => {
    const map = new Map<string, CalendarEpisode[]>();
    for (const ep of allEpisodes) {
      const key = ep.air_date.slice(0, 10);
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(ep);
    }
    return map;
  }, [allEpisodes]);

  // Calendar grid geometry.
  const firstDay = new Date(year, month, 1);
  const lastDay = new Date(year, month + 1, 0);
  const startPad = firstDay.getDay(); // 0=Sun
  const totalCells = startPad + lastDay.getDate();
  const rows = Math.ceil(totalCells / 7);

  const today = new Date();

  function prevMonth() {
    if (month === 0) {
      setYear((y) => y - 1);
      setMonth(11);
    } else {
      setMonth((m) => m - 1);
    }
  }

  function nextMonth() {
    if (month === 11) {
      setYear((y) => y + 1);
      setMonth(0);
    } else {
      setMonth((m) => m + 1);
    }
  }

  function goToday() {
    setYear(now.getFullYear());
    setMonth(now.getMonth());
  }

  return (
    <div style={{ padding: 24, maxWidth: 1100 }}>
      {/* Header */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 12,
          marginBottom: 20,
        }}
      >
        <h1
          style={{
            margin: 0,
            fontSize: 20,
            fontWeight: 700,
            color: "var(--color-text-primary)",
            letterSpacing: "-0.02em",
            flex: 1,
          }}
        >
          Calendar
        </h1>

        <button
          onClick={goToday}
          style={{
            background: "var(--color-bg-elevated)",
            border: "1px solid var(--color-border-default)",
            borderRadius: 6,
            padding: "5px 12px",
            fontSize: 12,
            color: "var(--color-text-secondary)",
            cursor: "pointer",
          }}
        >
          Today
        </button>

        <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
          <button
            onClick={prevMonth}
            style={{
              background: "var(--color-bg-elevated)",
              border: "1px solid var(--color-border-default)",
              borderRadius: 6,
              padding: "5px 10px",
              fontSize: 14,
              color: "var(--color-text-secondary)",
              cursor: "pointer",
              lineHeight: 1,
            }}
          >
            ‹
          </button>

          <span
            style={{
              fontSize: 15,
              fontWeight: 600,
              color: "var(--color-text-primary)",
              minWidth: 140,
              textAlign: "center",
            }}
          >
            {MONTH_NAMES[month]} {year}
          </span>

          <button
            onClick={nextMonth}
            style={{
              background: "var(--color-bg-elevated)",
              border: "1px solid var(--color-border-default)",
              borderRadius: 6,
              padding: "5px 10px",
              fontSize: 14,
              color: "var(--color-text-secondary)",
              cursor: "pointer",
              lineHeight: 1,
            }}
          >
            ›
          </button>
        </div>
      </div>

      {/* Legend */}
      <div
        style={{
          display: "flex",
          gap: 16,
          marginBottom: 14,
          alignItems: "center",
        }}
      >
        {[
          { color: "var(--color-success)", label: "Downloaded" },
          { color: "var(--color-danger)", label: "Missing" },
          { color: "var(--color-border-default)", label: "Unmonitored" },
        ].map(({ color, label }) => (
          <div
            key={label}
            style={{ display: "flex", alignItems: "center", gap: 5 }}
          >
            <div
              style={{
                width: 12,
                height: 12,
                borderRadius: 2,
                background: color,
                flexShrink: 0,
              }}
            />
            <span style={{ fontSize: 11, color: "var(--color-text-muted)" }}>
              {label}
            </span>
          </div>
        ))}
        {isLoading && (
          <span
            style={{
              fontSize: 11,
              color: "var(--color-text-muted)",
              marginLeft: "auto",
            }}
          >
            Loading…
          </span>
        )}
      </div>

      {/* Day-of-week headers */}
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(7, 1fr)",
          gap: 4,
          marginBottom: 4,
        }}
      >
        {DAY_NAMES.map((d) => (
          <div
            key={d}
            style={{
              fontSize: 11,
              fontWeight: 600,
              color: "var(--color-text-muted)",
              textAlign: "center",
              padding: "4px 0",
              textTransform: "uppercase",
              letterSpacing: "0.05em",
            }}
          >
            {d}
          </div>
        ))}
      </div>

      {/* Calendar grid */}
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(7, 1fr)",
          gap: 4,
        }}
      >
        {Array.from({ length: rows * 7 }, (_, i) => {
          const dayNum = i - startPad + 1;
          const isCurrentMonth =
            dayNum >= 1 && dayNum <= lastDay.getDate();
          const cellDate = new Date(year, month, dayNum);
          const dateKey = toDateKey(cellDate);
          const cellEpisodes = isCurrentMonth
            ? (episodesByDate.get(dateKey) ?? [])
            : [];
          const isToday =
            isCurrentMonth && isSameDay(cellDate, today);

          return (
            <DayCell
              key={i}
              date={cellDate}
              episodes={cellEpisodes}
              isToday={isToday}
              isCurrentMonth={isCurrentMonth}
              onEpisodeClick={(seriesId) =>
                navigate(`/series/${seriesId}`)
              }
            />
          );
        })}
      </div>
    </div>
  );
}
