import { useState, Fragment } from "react";
import { ArrowLeft, Check, AlertCircle, Loader2 } from "lucide-react";
import PageHeader from "@/components/PageHeader";
import { useSonarrPreview, useSonarrImport } from "@/api/import";
import type {
  SonarrPreviewResult,
  SonarrImportOptions,
  SonarrImportResult,
  CategoryResult,
} from "@/types";

// ── Shared styles ─────────────────────────────────────────────────────────────

const cardStyle: React.CSSProperties = {
  background: "var(--color-bg-surface)",
  border: "1px solid var(--color-border-subtle)",
  borderRadius: 8,
  padding: 20,
  boxShadow: "var(--shadow-card)",
};

const labelStyle: React.CSSProperties = {
  display: "block",
  fontSize: 12,
  fontWeight: 600,
  color: "var(--color-text-secondary)",
  marginBottom: 6,
  letterSpacing: "0.02em",
};

const inputStyle: React.CSSProperties = {
  width: "100%",
  boxSizing: "border-box",
  background: "var(--color-bg-elevated)",
  border: "1px solid var(--color-border-default)",
  borderRadius: 6,
  padding: "8px 12px",
  fontSize: 13,
  color: "var(--color-text-primary)",
  outline: "none",
};

const primaryBtnStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 8,
  padding: "9px 18px",
  background: "var(--color-accent)",
  border: "none",
  borderRadius: 6,
  fontSize: 13,
  fontWeight: 600,
  color: "#fff",
  cursor: "pointer",
  transition: "opacity 150ms ease",
};

const ghostBtnStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 6,
  padding: "9px 14px",
  background: "none",
  border: "1px solid var(--color-border-default)",
  borderRadius: 6,
  fontSize: 13,
  fontWeight: 500,
  color: "var(--color-text-secondary)",
  cursor: "pointer",
};

const sectionLabelStyle: React.CSSProperties = {
  fontSize: 11,
  fontWeight: 600,
  letterSpacing: "0.08em",
  textTransform: "uppercase",
  color: "var(--color-text-muted)",
  marginBottom: 10,
  marginTop: 0,
};

function formatBytes(gb: number): string {
  if (gb >= 1000) return `${(gb / 1000).toFixed(1)} TB`;
  return `${gb.toFixed(0)} GB`;
}

// ── Stage 1: Connection ───────────────────────────────────────────────────────

interface Stage1Props {
  url: string;
  apiKey: string;
  onUrlChange: (v: string) => void;
  onApiKeyChange: (v: string) => void;
  onConnect: () => void;
  isPending: boolean;
}

function ConnectionStage({
  url,
  apiKey,
  onUrlChange,
  onApiKeyChange,
  onConnect,
  isPending,
}: Stage1Props) {
  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    onConnect();
  }

  return (
    <form onSubmit={handleSubmit} style={{ maxWidth: 480 }}>
      <div style={{ ...cardStyle, display: "flex", flexDirection: "column", gap: 16 }}>
        <div>
          <label style={labelStyle} htmlFor="sonarr-url">
            Sonarr URL
          </label>
          <input
            id="sonarr-url"
            type="url"
            value={url}
            onChange={(e) => onUrlChange(e.target.value)}
            placeholder="http://localhost:8989"
            required
            autoFocus
            style={inputStyle}
          />
        </div>

        <div>
          <label style={labelStyle} htmlFor="sonarr-api-key">
            API Key
          </label>
          <input
            id="sonarr-api-key"
            type="password"
            value={apiKey}
            onChange={(e) => onApiKeyChange(e.target.value)}
            placeholder="Your Sonarr API key"
            required
            style={inputStyle}
          />
          <p style={{ margin: "6px 0 0", fontSize: 12, color: "var(--color-text-muted)" }}>
            Found in Sonarr under Settings &rarr; General &rarr; Security.
          </p>
        </div>

        <div style={{ paddingTop: 4 }}>
          <button
            type="submit"
            disabled={isPending || !url || !apiKey}
            style={{
              ...primaryBtnStyle,
              opacity: isPending || !url || !apiKey ? 0.55 : 1,
              cursor: isPending || !url || !apiKey ? "not-allowed" : "pointer",
            }}
          >
            {isPending && <Loader2 size={14} style={{ animation: "spin 1s linear infinite" }} />}
            {isPending ? "Connecting…" : "Connect"}
          </button>
        </div>
      </div>
    </form>
  );
}

// ── Stage 2: Preview ──────────────────────────────────────────────────────────

interface Stage2Props {
  preview: SonarrPreviewResult;
  options: SonarrImportOptions;
  onOptionsChange: (o: SonarrImportOptions) => void;
  onBack: () => void;
  onImport: () => void;
  isPending: boolean;
}

const OPTION_KEYS: (keyof SonarrImportOptions)[] = [
  "quality_profiles",
  "libraries",
  "indexers",
  "download_clients",
  "series",
];

const OPTION_LABELS: Record<keyof SonarrImportOptions, string> = {
  quality_profiles: "Quality Profiles",
  libraries: "Libraries (root folders)",
  indexers: "Indexers",
  download_clients: "Download Clients",
  series: "Series",
};

function PreviewStage({
  preview,
  options,
  onOptionsChange,
  onBack,
  onImport,
  isPending,
}: Stage2Props) {
  function toggle(key: keyof SonarrImportOptions) {
    onOptionsChange({ ...options, [key]: !options[key] });
  }

  const noneSelected = OPTION_KEYS.every((k) => !options[k]);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20, maxWidth: 680 }}>
      {/* Summary card */}
      <div style={cardStyle}>
        <p style={sectionLabelStyle}>Sonarr Instance</p>
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, minmax(140px, 1fr))",
            gap: 12,
          }}
        >
          <StatTile label="Version" value={preview.version} mono />
          <StatTile label="Series" value={String(preview.series_count)} />
          <StatTile label="Quality Profiles" value={String(preview.quality_profiles.length)} />
          <StatTile label="Root Folders" value={String(preview.root_folders.length)} />
          <StatTile label="Indexers" value={String(preview.indexers.length)} />
          <StatTile label="Download Clients" value={String(preview.download_clients.length)} />
        </div>
      </div>

      {/* Details: quality profiles */}
      {preview.quality_profiles.length > 0 && (
        <DetailSection title="Quality Profiles">
          {preview.quality_profiles.map((qp) => (
            <span key={qp.id} style={tagStyle}>
              {qp.name}
            </span>
          ))}
        </DetailSection>
      )}

      {/* Details: root folders */}
      {preview.root_folders.length > 0 && (
        <DetailSection title="Root Folders">
          <div style={{ display: "flex", flexDirection: "column", gap: 4, width: "100%" }}>
            {preview.root_folders.map((rf) => (
              <div
                key={rf.path}
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  fontSize: 13,
                  padding: "4px 0",
                  borderBottom: "1px solid var(--color-border-subtle)",
                }}
              >
                <span
                  style={{
                    fontFamily: "var(--font-family-mono)",
                    color: "var(--color-text-primary)",
                    fontSize: 12,
                  }}
                >
                  {rf.path}
                </span>
                <span style={{ fontSize: 12, color: "var(--color-text-muted)", whiteSpace: "nowrap", marginLeft: 16 }}>
                  {formatBytes(rf.free_space_gb)} free
                </span>
              </div>
            ))}
          </div>
        </DetailSection>
      )}

      {/* What to import */}
      <div style={cardStyle}>
        <p style={sectionLabelStyle}>What to Import</p>
        <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
          {OPTION_KEYS.map((key) => (
            <label
              key={key}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 10,
                padding: "9px 10px",
                borderRadius: 6,
                cursor: "pointer",
                userSelect: "none",
                background: options[key] ? "var(--color-accent-muted)" : "transparent",
                transition: "background 120ms ease",
              }}
            >
              <input
                type="checkbox"
                checked={options[key]}
                onChange={() => toggle(key)}
                style={{ accentColor: "var(--color-accent)", width: 14, height: 14 }}
              />
              <span
                style={{
                  fontSize: 13,
                  fontWeight: 500,
                  color: options[key] ? "var(--color-accent-hover)" : "var(--color-text-secondary)",
                }}
              >
                {OPTION_LABELS[key]}
              </span>
            </label>
          ))}
        </div>
      </div>

      {/* Actions */}
      <div style={{ display: "flex", gap: 10 }}>
        <button style={ghostBtnStyle} onClick={onBack} disabled={isPending}>
          <ArrowLeft size={14} />
          Back
        </button>
        <button
          style={{
            ...primaryBtnStyle,
            opacity: isPending || noneSelected ? 0.55 : 1,
            cursor: isPending || noneSelected ? "not-allowed" : "pointer",
          }}
          onClick={onImport}
          disabled={isPending || noneSelected}
        >
          {isPending && <Loader2 size={14} style={{ animation: "spin 1s linear infinite" }} />}
          {isPending ? "Importing…" : "Import"}
        </button>
      </div>
    </div>
  );
}

// ── Stage 3: Results ──────────────────────────────────────────────────────────

interface Stage3Props {
  result: SonarrImportResult;
  onDone: () => void;
}

const RESULT_KEYS: (keyof Omit<SonarrImportResult, "errors">)[] = [
  "quality_profiles",
  "libraries",
  "indexers",
  "download_clients",
  "series",
];

const RESULT_LABELS: Record<keyof Omit<SonarrImportResult, "errors">, string> = {
  quality_profiles: "Quality Profiles",
  libraries: "Libraries",
  indexers: "Indexers",
  download_clients: "Download Clients",
  series: "Series",
};

function ResultStage({ result, onDone }: Stage3Props) {
  const hasErrors = result.errors && result.errors.length > 0;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20, maxWidth: 560 }}>
      <div style={cardStyle}>
        <p style={sectionLabelStyle}>Import Summary</p>
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "1fr repeat(3, 72px)",
            gap: "0 8px",
          }}
        >
          {/* Header row */}
          <span style={{ fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", padding: "4px 0 8px" }} />
          {(["Imported", "Skipped", "Failed"] as const).map((h) => (
            <span
              key={h}
              style={{
                fontSize: 11,
                fontWeight: 600,
                color: "var(--color-text-muted)",
                textAlign: "center",
                padding: "4px 0 8px",
              }}
            >
              {h}
            </span>
          ))}

          {/* Data rows */}
          {RESULT_KEYS.map((key, idx) => {
            const cat: CategoryResult = result[key];
            const isLast = idx === RESULT_KEYS.length - 1;
            return (
              <Fragment key={key}>
                <div
                  style={{
                    fontSize: 13,
                    color: "var(--color-text-primary)",
                    padding: "9px 0",
                    borderTop: "1px solid var(--color-border-subtle)",
                    borderBottom: isLast ? "none" : undefined,
                  }}
                >
                  {RESULT_LABELS[key]}
                </div>
                <CountCell value={cat.imported} type="imported" isLast={isLast} />
                <CountCell value={cat.skipped} type="skipped" isLast={isLast} />
                <CountCell value={cat.failed} type="failed" isLast={isLast} />
              </Fragment>
            );
          })}
        </div>
      </div>

      {/* Errors */}
      {hasErrors && (
        <div
          style={{
            background: "var(--color-danger-muted, rgba(239,68,68,0.08))",
            border: "1px solid var(--color-danger)",
            borderRadius: 8,
            padding: "14px 16px",
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              marginBottom: 10,
              color: "var(--color-danger)",
              fontSize: 13,
              fontWeight: 600,
            }}
          >
            <AlertCircle size={14} />
            {result.errors.length} error{result.errors.length !== 1 ? "s" : ""} during import
          </div>
          <ul style={{ margin: 0, padding: "0 0 0 18px", display: "flex", flexDirection: "column", gap: 4 }}>
            {result.errors.map((err, i) => (
              <li
                key={i}
                style={{ fontSize: 12, color: "var(--color-danger)", fontFamily: "var(--font-family-mono)" }}
              >
                {err}
              </li>
            ))}
          </ul>
        </div>
      )}

      <div>
        <button
          style={{ ...primaryBtnStyle }}
          onClick={onDone}
        >
          <Check size={14} />
          Done
        </button>
      </div>
    </div>
  );
}

// ── Small reusable pieces ─────────────────────────────────────────────────────

const tagStyle: React.CSSProperties = {
  display: "inline-block",
  background: "var(--color-bg-elevated)",
  border: "1px solid var(--color-border-subtle)",
  borderRadius: 4,
  padding: "3px 8px",
  fontSize: 12,
  color: "var(--color-text-secondary)",
};

function StatTile({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div
      style={{
        background: "var(--color-bg-elevated)",
        border: "1px solid var(--color-border-subtle)",
        borderRadius: 6,
        padding: "10px 12px",
      }}
    >
      <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginBottom: 4 }}>{label}</div>
      <div
        style={{
          fontSize: 15,
          fontWeight: 600,
          color: "var(--color-text-primary)",
          fontFamily: mono ? "var(--font-family-mono)" : undefined,
        }}
      >
        {value}
      </div>
    </div>
  );
}

function DetailSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{ ...cardStyle }}>
      <p style={sectionLabelStyle}>{title}</p>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>{children}</div>
    </div>
  );
}

function CountCell({
  value,
  type,
  isLast,
}: {
  value: number;
  type: "imported" | "skipped" | "failed";
  isLast: boolean;
}) {
  const color =
    type === "imported" && value > 0
      ? "var(--color-success, #22c55e)"
      : type === "failed" && value > 0
      ? "var(--color-danger)"
      : "var(--color-text-muted)";

  return (
    <div
      style={{
        fontSize: 13,
        fontWeight: value > 0 && type !== "skipped" ? 600 : 400,
        color,
        textAlign: "center",
        padding: "9px 0",
        borderTop: "1px solid var(--color-border-subtle)",
        borderBottom: isLast ? "none" : undefined,
      }}
    >
      {value}
    </div>
  );
}

// ── Main component ────────────────────────────────────────────────────────────

type Stage = "connect" | "preview" | "results";

const defaultOptions: SonarrImportOptions = {
  quality_profiles: true,
  libraries: true,
  indexers: true,
  download_clients: true,
  series: true,
};

export default function ImportPage() {
  const [stage, setStage] = useState<Stage>("connect");
  const [url, setUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [preview, setPreview] = useState<SonarrPreviewResult | null>(null);
  const [importResult, setImportResult] = useState<SonarrImportResult | null>(null);
  const [options, setOptions] = useState<SonarrImportOptions>(defaultOptions);

  const previewMutation = useSonarrPreview();
  const importMutation = useSonarrImport();

  async function handleConnect() {
    const data = await previewMutation.mutateAsync({ url, api_key: apiKey });
    setPreview(data);
    setOptions(defaultOptions);
    setStage("preview");
  }

  async function handleImport() {
    const data = await importMutation.mutateAsync({ url, api_key: apiKey, options });
    setImportResult(data);
    setStage("results");
  }

  function handleDone() {
    setStage("connect");
    setUrl("");
    setApiKey("");
    setPreview(null);
    setImportResult(null);
    setOptions(defaultOptions);
    previewMutation.reset();
    importMutation.reset();
  }

  const steps: { key: Stage; label: string }[] = [
    { key: "connect", label: "Connect" },
    { key: "preview", label: "Preview" },
    { key: "results", label: "Results" },
  ];

  const stageIndex = steps.findIndex((s) => s.key === stage);

  return (
    <div style={{ padding: "32px", maxWidth: 720 }}>
      <PageHeader
        title="Sonarr Import"
        description="One-time migration from an existing Sonarr instance."
      />

      {/* Step indicator */}
      <div style={{ display: "flex", alignItems: "center", gap: 0, marginBottom: 28 }}>
        {steps.map((step, idx) => {
          const isActive = step.key === stage;
          const isPast = idx < stageIndex;
          return (
            <div key={step.key} style={{ display: "flex", alignItems: "center" }}>
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 8,
                  padding: "4px 0",
                }}
              >
                <div
                  style={{
                    width: 22,
                    height: 22,
                    borderRadius: "50%",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    fontSize: 11,
                    fontWeight: 700,
                    background: isActive
                      ? "var(--color-accent)"
                      : isPast
                      ? "var(--color-accent-muted)"
                      : "var(--color-bg-elevated)",
                    border: isActive
                      ? "none"
                      : isPast
                      ? "1px solid var(--color-accent)"
                      : "1px solid var(--color-border-default)",
                    color: isActive
                      ? "#fff"
                      : isPast
                      ? "var(--color-accent)"
                      : "var(--color-text-muted)",
                    flexShrink: 0,
                  }}
                >
                  {isPast ? <Check size={11} strokeWidth={3} /> : idx + 1}
                </div>
                <span
                  style={{
                    fontSize: 13,
                    fontWeight: isActive ? 600 : 400,
                    color: isActive
                      ? "var(--color-text-primary)"
                      : isPast
                      ? "var(--color-accent)"
                      : "var(--color-text-muted)",
                  }}
                >
                  {step.label}
                </span>
              </div>
              {idx < steps.length - 1 && (
                <div
                  style={{
                    width: 32,
                    height: 1,
                    background: isPast ? "var(--color-accent)" : "var(--color-border-subtle)",
                    margin: "0 10px",
                    flexShrink: 0,
                  }}
                />
              )}
            </div>
          );
        })}
      </div>

      {/* Stage content */}
      {stage === "connect" && (
        <ConnectionStage
          url={url}
          apiKey={apiKey}
          onUrlChange={setUrl}
          onApiKeyChange={setApiKey}
          onConnect={handleConnect}
          isPending={previewMutation.isPending}
        />
      )}

      {stage === "preview" && preview && (
        <PreviewStage
          preview={preview}
          options={options}
          onOptionsChange={setOptions}
          onBack={() => setStage("connect")}
          onImport={handleImport}
          isPending={importMutation.isPending}
        />
      )}

      {stage === "results" && importResult && (
        <ResultStage result={importResult} onDone={handleDone} />
      )}
    </div>
  );
}
