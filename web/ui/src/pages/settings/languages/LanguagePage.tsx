import { useEffect, useState, type CSSProperties } from "react";
import { toast } from "sonner";
import PageHeader from "@/components/PageHeader";
import { languageLabel, useLanguagePreference, useSaveLanguagePreference } from "@/api/language";

const card: CSSProperties = {
  background: "var(--color-bg-surface)",
  border: "1px solid var(--color-border-subtle)",
  borderRadius: 8,
  padding: 20,
  display: "flex",
  flexDirection: "column",
  gap: 18,
};

const legendStyle: CSSProperties = { display: "flex", flexDirection: "column", gap: 6, fontSize: 12, fontWeight: 500, color: "var(--color-text-secondary)", padding: 0, marginBottom: 10 };
const rowLabel: CSSProperties = { display: "flex", alignItems: "center", gap: 6, fontSize: 13, color: "var(--color-text-primary)", cursor: "pointer" };
const hint: CSSProperties = { fontSize: 11, fontWeight: 400, color: "var(--color-text-muted)" };

/** Keeps the wanted languages in the server's display order. */
export function orderedSelection(available: string[], selected: Set<string>): string[] {
  return available.filter((l) => selected.has(l));
}

export default function LanguagePage() {
  const { data, isLoading } = useLanguagePreference();
  const save = useSaveLanguagePreference();
  const [selected, setSelected] = useState<Set<string> | null>(null);

  useEffect(() => {
    if (data) setSelected(new Set(data.languages));
  }, [data]);

  function toggle(lang: string) {
    setSelected((s) => {
      const next = new Set(s);
      if (next.has(lang)) next.delete(lang);
      else next.add(lang);
      return next;
    });
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!data || !selected) return;
    try {
      await save.mutateAsync(orderedSelection(data.available, selected));
      toast.success("Language preference saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Could not save the language preference");
    }
  }

  return (
    <div style={{ padding: 32, maxWidth: 720 }}>
      <PageHeader title="Languages" description="Which audio languages a release may be in. Pilot only grabs releases in a language you choose." />
      {(isLoading || !data || !selected) && <div className="skeleton" style={{ height: 220, borderRadius: 8 }} />}
      {data && selected && (
        <form onSubmit={onSubmit} style={card}>
          <fieldset style={{ border: "none", margin: 0, padding: 0 }}>
            <legend style={legendStyle}>
              Wanted languages
              <span style={hint}>
                {selected.size === 0 ? "None chosen: any language is accepted." : "Releases in other languages are skipped by automatic grabs and grayed out in manual search."}
              </span>
            </legend>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(130px, 1fr))", gap: "8px 14px" }}>
              {data.available.map((lang) => (
                <label key={lang} style={rowLabel}>
                  <input type="checkbox" checked={selected.has(lang)} onChange={() => toggle(lang)} />
                  {languageLabel(lang)}
                </label>
              ))}
            </div>
          </fieldset>

          <p style={{ ...hint, margin: 0 }}>
            The language is read from the release name. A release with no language tag counts as English, a MULTi release (several audio tracks)
            matches any language, and NORDiC matches each Nordic language.
          </p>

          <div>
            <button
              type="submit"
              disabled={save.isPending}
              style={{ padding: "8px 16px", borderRadius: 6, border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)", fontSize: 13, fontWeight: 500, cursor: "pointer" }}
            >
              {save.isPending ? "Saving…" : "Save"}
            </button>
          </div>
        </form>
      )}
    </div>
  );
}
