import { Check, Star } from "lucide-react";
import { Link } from "react-router-dom";
import { tmdbPosterURL } from "@/api/series";

// Minimal shape shared by search results (LookupResult) and browse-list
// items (DiscoverListItem) — only the fields the card actually renders.
interface DiscoverCardItem {
  tmdb_id: number;
  title: string;
  year: number;
  poster_path: string;
  overview?: string;
  rating?: number;
}

interface Props {
  result: DiscoverCardItem;
  isAdded: boolean;
}

export default function DiscoverCard({ result, isAdded }: Props) {
  const posterSrc = tmdbPosterURL(result.poster_path, "w342");

  return (
    <Link
      to={`/discover/${result.tmdb_id}`}
      style={{
        display: "flex",
        flexDirection: "column",
        borderRadius: 8,
        border: "1px solid var(--color-border-subtle)",
        background: isAdded ? "var(--color-bg-base)" : "var(--color-bg-surface)",
        cursor: "pointer",
        textAlign: "left",
        textDecoration: "none",
        overflow: "hidden",
        transition: "border-color 120ms ease, box-shadow 120ms ease",
        opacity: isAdded ? 0.65 : 1,
        position: "relative",
      }}
      onMouseEnter={(e) => {
        if (!isAdded) {
          e.currentTarget.style.borderColor = "var(--color-accent)";
          e.currentTarget.style.boxShadow = "0 0 0 1px var(--color-accent)";
        }
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.borderColor = "var(--color-border-subtle)";
        e.currentTarget.style.boxShadow = "none";
      }}
    >
      {/* Poster */}
      <div style={{ position: "relative", aspectRatio: "2/3", background: "var(--color-bg-elevated)", overflow: "hidden" }}>
        {posterSrc ? (
          <img
            src={posterSrc}
            alt={result.title}
            loading="lazy"
            style={{ width: "100%", height: "100%", objectFit: "cover" }}
          />
        ) : (
          <div style={{ width: "100%", height: "100%", display: "flex", alignItems: "center", justifyContent: "center", color: "var(--color-text-muted)", fontSize: 12 }}>
            No Poster
          </div>
        )}
        {result.rating != null && result.rating > 0 && (
          <div
            style={{
              position: "absolute",
              top: 6,
              right: 6,
              display: "flex",
              alignItems: "center",
              gap: 3,
              padding: "2px 6px",
              borderRadius: 4,
              background: "rgba(0,0,0,0.7)",
              backdropFilter: "blur(4px)",
              fontSize: 11,
              fontWeight: 600,
              color: "#fbbf24",
            }}
          >
            <Star size={10} fill="#fbbf24" stroke="none" />
            {result.rating.toFixed(1)}
          </div>
        )}
      </div>

      {/* Info */}
      <div style={{ padding: "10px 12px" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <span style={{ fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", flex: 1 }}>
            {result.title}
          </span>
          {isAdded && (
            <span style={{ display: "inline-flex", alignItems: "center", gap: 3, fontSize: 10, fontWeight: 600, color: "var(--color-success)", background: "color-mix(in srgb, var(--color-success) 12%, transparent)", padding: "2px 6px", borderRadius: 3, flexShrink: 0 }}>
              <Check size={10} /> Added
            </span>
          )}
        </div>
        {result.year > 0 && (
          <div style={{ fontSize: 12, color: "var(--color-text-muted)", marginTop: 2 }}>
            {result.year}
          </div>
        )}
        {result.overview && (
          <p style={{
            margin: "6px 0 0",
            fontSize: 12,
            color: "var(--color-text-muted)",
            lineHeight: 1.4,
            display: "-webkit-box",
            WebkitLineClamp: 2,
            WebkitBoxOrient: "vertical",
            overflow: "hidden",
          }}>
            {result.overview}
          </p>
        )}
      </div>
    </Link>
  );
}
