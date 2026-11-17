import { useMemo, useState, type CSSProperties } from "react";
import { toast } from "sonner";
import { ArrowUp, Folder } from "lucide-react";
import PageHeader from "@/components/PageHeader";
import { useFsBrowse } from "@/api/filesystem";
import { useEpisodes, useSeasons, useSeriesList } from "@/api/series";
import {
  REJECTION_LABEL,
  useManualImport,
  useManualScan,
  type ManualCandidate,
  type ManualImportResult,
} from "@/api/manual-import";
import { formatBytes } from "@/lib/utils";

const input: CSSProperties = {
  padding: "7px 10px",
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  fontSize: 13,
};
const button: CSSProperties = { ...input, cursor: "pointer" };
const primary: CSSProperties = { ...button, background: "var(--color-accent)", color: "var(--color-accent-fg)", border: "none" };
const cell: CSSProperties = { padding: "8px 10px", borderBottom: "1px solid var(--color-border-subtle)", verticalAlign: "top", fontSize: 13 };

/** One row's editable choice. */
export interface RowChoice {
  include: boolean;
  seriesId: string;
  season: number;
  episodeIds: string[];
}

export function initialChoice(c: ManualCandidate): RowChoice {
  const usable = c.series_id !== "" && c.episodes.length > 0;
  const blocking = c.rejections.some((r) => r === "sample" || r === "already_imported");
  return {
    include: usable && !blocking && !c.rejections.includes("existing_file_is_equal_or_better"),
    seriesId: c.series_id,
    season: c.season,
    episodeIds: c.episodes.map((e) => e.id),
  };
}

function FolderBrowser({ path, onPick }: { path: string; onPick: (p: string) => void }) {
  const { data, isLoading, error } = useFsBrowse(path || "/");
  return (
    <div style={{ border: "1px solid var(--color-border-subtle)", borderRadius: 8, maxHeight: 240, overflowY: "auto" }}>
      {isLoading && <div style={{ padding: 10, fontSize: 12, color: "var(--color-text-muted)" }}>Loading…</div>}
      {error && <div style={{ padding: 10, fontSize: 12, color: "var(--color-danger)" }}>{(error as Error).message}</div>}
      {data?.parent && (
        <button type="button" onClick={() => onPick(data.parent!)} style={{ ...rowBtn }}>
          <ArrowUp size={13} /> ..
        </button>
      )}
      {data?.dirs.map((d) => (
        <button key={d.path} type="button" onClick={() => onPick(d.path)} style={{ ...rowBtn }}>
          <Folder size={13} /> {d.name}
        </button>
      ))}
    </div>
  );
}
const rowBtn: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 6,
  width: "100%",
  padding: "6px 10px",
  background: "none",
  border: "none",
  borderBottom: "1px solid var(--color-border-subtle)",
  color: "var(--color-text-primary)",
  fontSize: 13,
  textAlign: "left",
  cursor: "pointer",
};

function EpisodePicker({ choice, onChange }: { choice: RowChoice; onChange: (c: RowChoice) => void }) {
  const { data: seasons } = useSeasons(choice.seriesId);
  const { data: episodes } = useEpisodes(choice.seriesId, choice.season);
  if (!choice.seriesId) return <span style={{ color: "var(--color-text-muted)" }}>Pick a series</span>;
  return (
    <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
      <select
        aria-label="Season"
        value={choice.season}
        onChange={(e) => onChange({ ...choice, season: Number(e.target.value), episodeIds: [] })}
        style={input}
      >
        {(seasons ?? []).map((s) => (
          <option key={s.id} value={s.season_number}>
            Season {s.season_number}
          </option>
        ))}
      </select>
      <select
        aria-label="Episode"
        value={choice.episodeIds.length === 1 ? choice.episodeIds[0] : choice.episodeIds.length > 1 ? "multi" : ""}
        onChange={(e) => onChange({ ...choice, episodeIds: e.target.value && e.target.value !== "multi" ? [e.target.value] : [], include: !!e.target.value })}
        style={{ ...input, maxWidth: 260 }}
      >
        <option value="">Episode…</option>
        {choice.episodeIds.length > 1 && <option value="multi">{choice.episodeIds.length} episodes</option>}
        {(episodes ?? []).map((ep) => (
          <option key={ep.id} value={ep.id}>
            E{String(ep.episode_number).padStart(2, "0")} · {ep.title}
          </option>
        ))}
      </select>
    </div>
  );
}

export default function ManualImportPage() {
  const [path, setPath] = useState("");
  const [browsing, setBrowsing] = useState(false);
  const [mode, setMode] = useState<"copy" | "move">("copy");
  const [candidates, setCandidates] = useState<ManualCandidate[] | null>(null);
  const [choices, setChoices] = useState<Record<string, RowChoice>>({});
  const [results, setResults] = useState<Record<string, ManualImportResult>>({});
  const scan = useManualScan();
  const doImport = useManualImport();
  const { data: seriesList } = useSeriesList();
  const series = useMemo(() => [...(seriesList?.series ?? [])].sort((a, b) => a.sort_title.localeCompare(b.sort_title)), [seriesList]);

  async function onScan(e: React.FormEvent) {
    e.preventDefault();
    try {
      const r = await scan.mutateAsync({ path: path.trim() });
      setCandidates(r.candidates);
      setChoices(Object.fromEntries(r.candidates.map((c) => [c.path, initialChoice(c)])));
      setResults({});
      setBrowsing(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Scan failed");
    }
  }

  const chosen = (candidates ?? []).filter((c) => {
    const ch = choices[c.path];
    return ch?.include && ch.seriesId && ch.episodeIds.length > 0 && !results[c.path]?.ok;
  });

  async function onImport() {
    try {
      const r = await doImport.mutateAsync({
        mode,
        items: chosen.map((c) => ({ path: c.path, series_id: choices[c.path].seriesId, episode_ids: choices[c.path].episodeIds, quality: c.quality })),
      });
      setResults((cur) => ({ ...cur, ...Object.fromEntries(r.results.map((x) => [x.path, x])) }));
      const ok = r.results.filter((x) => x.ok).length;
      const failed = r.results.length - ok;
      if (failed === 0) toast.success(`Imported ${ok} file${ok === 1 ? "" : "s"}`);
      else toast.error(`Imported ${ok}, ${failed} failed`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Import failed");
    }
  }

  function setChoice(p: string, c: RowChoice) {
    setChoices((cur) => ({ ...cur, [p]: c }));
  }

  return (
    <div style={{ padding: "24px 28px", maxWidth: 1200 }}>
      <PageHeader
        title="Manual Import"
        description="Import video files from any folder on the server. Review what Pilot matched, fix anything wrong, then import."
      />

      <form onSubmit={onScan} style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 12 }}>
        <input
          aria-label="Folder"
          value={path}
          onChange={(e) => setPath(e.target.value)}
          placeholder="/downloads/complete"
          style={{ ...input, flex: "1 1 320px", fontFamily: "var(--font-family-mono)" }}
        />
        <button type="button" style={button} onClick={() => setBrowsing((b) => !b)}>
          {browsing ? "Hide folders" : "Browse…"}
        </button>
        <button type="submit" style={primary} disabled={!path.trim() || scan.isPending}>
          {scan.isPending ? "Scanning…" : "Scan"}
        </button>
      </form>
      {browsing && (
        <div style={{ marginBottom: 16 }}>
          <FolderBrowser path={path} onPick={setPath} />
        </div>
      )}

      {candidates && candidates.length === 0 && (
        <p style={{ fontSize: 13, color: "var(--color-text-muted)" }}>No video files found in that folder.</p>
      )}

      {candidates && candidates.length > 0 && (
        <>
          <div style={{ overflowX: "auto", border: "1px solid var(--color-border-subtle)", borderRadius: 8 }}>
            <table style={{ width: "100%", borderCollapse: "collapse", minWidth: 760 }}>
              <thead>
                <tr style={{ textAlign: "left", fontSize: 11, color: "var(--color-text-muted)", textTransform: "uppercase" }}>
                  <th style={cell}></th>
                  <th style={cell}>File</th>
                  <th style={cell}>Series</th>
                  <th style={cell}>Episode</th>
                  <th style={cell}>Quality</th>
                </tr>
              </thead>
              <tbody>
                {candidates.map((c) => {
                  const ch = choices[c.path];
                  const res = results[c.path];
                  const name = c.path.split("/").pop() ?? c.path;
                  return (
                    <tr key={c.path} style={{ opacity: res?.ok ? 0.5 : 1 }}>
                      <td style={cell}>
                        <input
                          type="checkbox"
                          aria-label={`Import ${name}`}
                          checked={!!ch?.include}
                          disabled={res?.ok}
                          onChange={(e) => setChoice(c.path, { ...ch, include: e.target.checked })}
                        />
                      </td>
                      <td style={{ ...cell, maxWidth: 360 }}>
                        <div style={{ overflowWrap: "anywhere", fontFamily: "var(--font-family-mono)", fontSize: 12 }}>{name}</div>
                        <div style={{ fontSize: 11, color: "var(--color-text-muted)" }}>{formatBytes(c.size)}</div>
                        {c.rejections.map((r) => (
                          <div key={r} style={{ fontSize: 11, color: "var(--color-warning)" }}>
                            {REJECTION_LABEL[r] ?? r}
                          </div>
                        ))}
                        {res?.ok && <div style={{ fontSize: 11, color: "var(--color-success)" }}>Imported</div>}
                        {res && !res.ok && <div style={{ fontSize: 11, color: "var(--color-danger)" }}>{res.error}</div>}
                      </td>
                      <td style={cell}>
                        <select
                          aria-label={`Series for ${name}`}
                          value={ch?.seriesId ?? ""}
                          onChange={(e) => setChoice(c.path, { ...ch, seriesId: e.target.value, episodeIds: [], season: ch?.season || 1 })}
                          style={{ ...input, maxWidth: 220 }}
                        >
                          <option value="">Series…</option>
                          {series.map((s) => (
                            <option key={s.id} value={s.id}>
                              {s.title}
                            </option>
                          ))}
                        </select>
                      </td>
                      <td style={cell}>{ch && <EpisodePicker choice={ch} onChange={(next) => setChoice(c.path, next)} />}</td>
                      <td style={cell}>{c.quality.name || "Unknown"}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          <div style={{ display: "flex", justifyContent: "flex-end", alignItems: "center", gap: 10, marginTop: 12, flexWrap: "wrap" }}>
            <label style={{ fontSize: 13, color: "var(--color-text-secondary)", display: "flex", alignItems: "center", gap: 6 }}>
              Mode
              <select aria-label="Import mode" value={mode} onChange={(e) => setMode(e.target.value as "copy" | "move")} style={input}>
                <option value="copy">Copy (keep the originals)</option>
                <option value="move">Move (remove the originals)</option>
              </select>
            </label>
            <button type="button" style={primary} disabled={chosen.length === 0 || doImport.isPending} onClick={() => void onImport()}>
              {doImport.isPending ? "Importing…" : `Import ${chosen.length} file${chosen.length === 1 ? "" : "s"}`}
            </button>
          </div>
        </>
      )}
    </div>
  );
}
