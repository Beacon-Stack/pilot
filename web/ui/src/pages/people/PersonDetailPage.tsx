import { useParams, Link } from "react-router-dom";
import { usePersonDetail } from "@/api/people";
import { tmdbPosterURL } from "@/api/series";

export default function PersonDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: person, isLoading, error } = usePersonDetail(Number(id) || 0);

  if (isLoading) {
    return (
      <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 20 }}>
        <div className="skeleton" style={{ height: 16, width: 80, borderRadius: 4 }} />
        <div style={{ display: "flex", gap: 20, alignItems: "center" }}>
          <div className="skeleton" style={{ width: 80, height: 80, borderRadius: "50%" }} />
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            <div className="skeleton" style={{ height: 22, width: 200, borderRadius: 4 }} />
            <div className="skeleton" style={{ height: 14, width: 100, borderRadius: 4 }} />
          </div>
        </div>
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, minmax(120px, 1fr))",
            gap: 16,
          }}
        >
          {Array.from({ length: 12 }).map((_, i) => (
            <div key={i}>
              <div className="skeleton" style={{ paddingBottom: "150%", borderRadius: 8 }} />
              <div className="skeleton" style={{ height: 12, width: "80%", marginTop: 6, borderRadius: 4 }} />
            </div>
          ))}
        </div>
      </div>
    );
  }

  if (error || !person) {
    return (
      <div style={{ padding: 24 }}>
        <Link to="/discover" style={{ fontSize: 13, color: "var(--color-accent)", textDecoration: "none" }}>
          ← Back
        </Link>
        <p style={{ marginTop: 24, fontSize: 13, color: "var(--color-text-muted)" }}>
          Person not found or failed to load.
        </p>
      </div>
    );
  }

  return (
    <div style={{ padding: 24, maxWidth: 1000 }}>
      {/* Back link */}
      <Link
        to={-1 as unknown as string}
        onClick={(e) => {
          e.preventDefault();
          window.history.back();
        }}
        style={{ fontSize: 13, color: "var(--color-text-muted)", textDecoration: "none", display: "inline-block", marginBottom: 20 }}
        onMouseEnter={(e) => { (e.currentTarget as HTMLAnchorElement).style.color = "var(--color-text-primary)"; }}
        onMouseLeave={(e) => { (e.currentTarget as HTMLAnchorElement).style.color = "var(--color-text-muted)"; }}
      >
        ← Back
      </Link>

      {/* Person header */}
      <div style={{ display: "flex", gap: 20, alignItems: "center", marginBottom: 28 }}>
        {person.profile_path ? (
          <img
            src={`https://image.tmdb.org/t/p/w185${person.profile_path}`}
            alt={person.name}
            style={{ width: 80, height: 80, borderRadius: "50%", objectFit: "cover", boxShadow: "var(--shadow-modal)" }}
          />
        ) : (
          <div
            style={{
              width: 80,
              height: 80,
              borderRadius: "50%",
              background: "var(--color-bg-subtle)",
              border: "1px solid var(--color-border-subtle)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              fontSize: 28,
              fontWeight: 600,
              color: "var(--color-text-muted)",
            }}
          >
            {person.name.charAt(0)}
          </div>
        )}
        <div>
          <h1 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: "var(--color-text-primary)", letterSpacing: "-0.02em" }}>
            {person.name}
          </h1>
          {person.known_for_department && (
            <p style={{ margin: "2px 0 0", fontSize: 13, color: "var(--color-text-muted)" }}>
              {person.known_for_department}
            </p>
          )}
        </div>
      </div>

      {/* Credits heading */}
      <h2 style={{
        margin: "0 0 14px",
        fontSize: 13,
        fontWeight: 600,
        color: "var(--color-text-primary)",
        textTransform: "uppercase",
        letterSpacing: "0.04em",
      }}>
        Known For
        <span style={{ fontWeight: 400, color: "var(--color-text-muted)", marginLeft: 8 }}>
          {person.credits.length} {person.credits.length === 1 ? "show" : "shows"}
        </span>
      </h2>

      {/* Credits grid */}
      {person.credits.length > 0 ? (
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, minmax(130px, 1fr))",
            gap: 18,
          }}
        >
          {person.credits.map((credit) => {
            const poster = tmdbPosterURL(credit.poster_path, "w342");
            return (
              <Link
                key={credit.tmdb_id}
                to={credit.in_library && credit.library_series_id ? `/series/${credit.library_series_id}` : `/discover/${credit.tmdb_id}`}
                style={{ textDecoration: "none", color: "inherit", display: "block" }}
              >
                <div style={{ position: "relative" }}>
                  {poster ? (
                    <img src={poster} alt={credit.title} loading="lazy" style={{ width: "100%", borderRadius: 8, aspectRatio: "2/3", objectFit: "cover" }} />
                  ) : (
                    <div style={{ width: "100%", aspectRatio: "2/3", borderRadius: 8, background: "var(--color-bg-elevated)", display: "flex", alignItems: "center", justifyContent: "center", color: "var(--color-text-muted)", fontSize: 12 }}>
                      No Poster
                    </div>
                  )}
                  {credit.in_library && (
                    <div
                      style={{
                        position: "absolute",
                        top: 6,
                        right: 6,
                        fontSize: 9,
                        padding: "2px 6px",
                        borderRadius: 4,
                        background: "color-mix(in srgb, var(--color-success) 85%, transparent)",
                        color: "#fff",
                        fontWeight: 600,
                      }}
                    >
                      In Library
                    </div>
                  )}
                </div>
                <div style={{ marginTop: 6 }}>
                  <span style={{
                    display: "block",
                    fontSize: 12,
                    fontWeight: 500,
                    color: "var(--color-text-primary)",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}>
                    {credit.title}
                  </span>
                  {credit.year > 0 && (
                    <span style={{ fontSize: 11, color: "var(--color-text-muted)" }}>{credit.year}</span>
                  )}
                </div>
              </Link>
            );
          })}
        </div>
      ) : (
        <p style={{ fontSize: 13, color: "var(--color-text-muted)", padding: "24px 0" }}>
          No shows found.
        </p>
      )}
    </div>
  );
}
