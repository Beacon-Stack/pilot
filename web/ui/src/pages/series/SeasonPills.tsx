import type { Season } from "@/types";

interface Props {
  seasons: Season[];
  activeSeason: number; // -1 = "All Seasons"
  onSelect: (seasonNumber: number) => void;
  episodeCounts: Map<number, { total: number; downloaded: number }>; // seasonNumber → counts
}

export default function SeasonPills({ seasons, activeSeason, onSelect, episodeCounts }: Props) {
  return (
    <div style={{ display: "flex", gap: 6, overflowX: "auto", paddingBottom: 4, marginBottom: 16 }}>
      {/* All Seasons pill */}
      <Pill
        label="All Seasons"
        active={activeSeason === -1}
        onClick={() => onSelect(-1)}
      />
      {seasons.map((s) => {
        const counts = episodeCounts.get(s.season_number);
        const label = s.season_number === 0 ? "Specials" : `Season ${s.season_number}`;
        return (
          <Pill
            key={s.id}
            label={label}
            active={activeSeason === s.season_number}
            onClick={() => onSelect(s.season_number)}
            statusDot={getStatusDot(counts)}
          />
        );
      })}
    </div>
  );
}

function Pill({ label, active, onClick, statusDot }: {
  label: string;
  active: boolean;
  onClick: () => void;
  statusDot?: string;
}) {
  return (
    <button
      onClick={onClick}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 6,
        padding: "6px 14px",
        borderRadius: 20,
        border: active ? "1px solid var(--color-accent)" : "1px solid var(--color-border-default)",
        background: active ? "var(--color-accent-muted)" : "transparent",
        color: active ? "var(--color-accent)" : "var(--color-text-secondary)",
        fontSize: 13,
        fontWeight: 500,
        cursor: "pointer",
        whiteSpace: "nowrap",
        transition: "all 120ms ease",
      }}
    >
      {statusDot && (
        <span style={{ width: 7, height: 7, borderRadius: "50%", background: statusDot, flexShrink: 0 }} />
      )}
      {label}
    </button>
  );
}

function getStatusDot(counts?: { total: number; downloaded: number }): string | undefined {
  if (!counts || counts.total === 0) return undefined;
  if (counts.downloaded === counts.total) return "var(--color-success)";
  if (counts.downloaded > 0) return "var(--color-warning)";
  return "var(--color-text-muted)";
}
