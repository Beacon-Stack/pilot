import { useState } from "react";
import { Link } from "react-router-dom";
import TableScroll from "@beacon-shared/TableScroll";
import PageHeader from "@/components/PageHeader";
import DownloadClientModal from "./DownloadClientModal";
import { DOCS_URLS } from "@/lib/docsUrls";
import { useDownloadClients, useDeleteDownloadClient, useTestDownloadClient } from "@/api/download-clients";
import type { DownloadClientConfig, TestResult } from "@/types";

// Helpers

function actionBtn(color: string, bg: string): React.CSSProperties {
  return {
    background: bg,
    border: "1px solid var(--color-border-default)",
    borderRadius: 5,
    padding: "3px 10px",
    fontSize: 12,
    color,
    cursor: "pointer",
    whiteSpace: "nowrap",
  };
}

// Form state

interface RowActionsProps {
  client: DownloadClientConfig;
  onEdit: () => void;
}

function RowActions({ client, onEdit }: RowActionsProps) {
  const [confirming, setConfirming] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; message?: string } | null>(null);

  const del = useDeleteDownloadClient();
  const test = useTestDownloadClient();

  function handleTest() {
    setTestResult(null);
    test.mutate(client.id, {
      onSuccess: (r: TestResult) => {
        setTestResult(r);
        setTimeout(() => setTestResult(null), 4000);
      },
      onError: (e) => {
        setTestResult({ ok: false, message: e.message });
        setTimeout(() => setTestResult(null), 4000);
      },
    });
  }

  if (confirming) {
    return (
      <div style={{ display: "flex", alignItems: "center", gap: 6, justifyContent: "flex-end" }}>
        <span style={{ fontSize: 12, color: "var(--color-text-secondary)" }}>Delete?</span>
        <button
          onClick={() => del.mutate(client.id, { onSuccess: () => setConfirming(false) })}
          disabled={del.isPending}
          style={actionBtn("var(--color-danger)", "color-mix(in srgb, var(--color-danger) 15%, transparent)")}
        >
          {del.isPending ? "…" : "Yes"}
        </button>
        <button
          onClick={() => setConfirming(false)}
          style={actionBtn("var(--color-text-secondary)", "var(--color-bg-elevated)")}
        >
          No
        </button>
      </div>
    );
  }

  if (testResult !== null) {
    return (
      <div style={{ display: "flex", alignItems: "center", gap: 6, justifyContent: "flex-end" }}>
        <span
          style={{
            fontSize: 12,
            color: testResult.ok ? "var(--color-success)" : "var(--color-danger)",
            maxWidth: 200,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
          title={testResult.message}
        >
          {testResult.ok ? "Connected ✓" : `Failed: ${testResult.message ?? "unknown error"}`}
        </span>
      </div>
    );
  }

  return (
    <div style={{ display: "flex", alignItems: "center", gap: 6, justifyContent: "flex-end" }}>
      <button
        onClick={handleTest}
        disabled={test.isPending}
        style={actionBtn("var(--color-text-secondary)", "var(--color-bg-elevated)")}
      >
        {test.isPending ? "Testing…" : "Test"}
      </button>
      <button onClick={onEdit} style={actionBtn("var(--color-text-secondary)", "var(--color-bg-elevated)")}>
        Edit
      </button>
      <button
        onClick={() => setConfirming(true)}
        style={actionBtn("var(--color-danger)", "color-mix(in srgb, var(--color-danger) 12%, transparent)")}
      >
        Delete
      </button>
    </div>
  );
}

// Badge

function KindBadge({ kind }: { kind: string }) {
  const labels: Record<string, string> = {
    qbittorrent: "qBittorrent", deluge: "Deluge", transmission: "Transmission",
    sabnzbd: "SABnzbd", nzbget: "NZBGet",
  };
  const colors: Record<string, string> = {
    qbittorrent: "var(--color-accent)", deluge: "var(--color-accent)",
    transmission: "#B71C1C", sabnzbd: "#F57C00", nzbget: "#388E3C",
  };
  const c = colors[kind] ?? "var(--color-accent)";
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        borderRadius: 4,
        fontSize: 11,
        fontWeight: 600,
        textTransform: "uppercase",
        letterSpacing: "0.05em",
        background: `color-mix(in srgb, ${c} 12%, transparent)`,
        color: c,
      }}
    >
      {labels[kind] ?? kind}
    </span>
  );
}

// How downloads are handled

// What Pilot does with finished and failed downloads. This is fixed
// behaviour, not a setting.
function DownloadHandlingNote() {
  return (
    <div
      style={{
        marginTop: 24,
        background: "var(--color-bg-surface)",
        border: "1px solid var(--color-border-subtle)",
        borderRadius: 8,
        padding: 16,
        fontSize: 13,
        color: "var(--color-text-secondary)",
        lineHeight: 1.5,
      }}
    >
      <p style={{ margin: "0 0 8px", fontWeight: 600, color: "var(--color-text-primary)" }}>How downloads are handled</p>
      <ul style={{ margin: 0, paddingLeft: 18 }}>
        <li>Download clients are checked every 30 seconds. Finished downloads are imported into the library.</li>
        <li>
          A failed download is blocklisted. If Pilot grabbed it automatically, it searches again (up to 3 times per episode a
          day); releases you picked yourself are left for you to decide.
        </li>
        <li>
          If a download client runs in a container or on another machine and imports fail with a missing path, add a{" "}
          <Link to="/settings/remote-path-mappings" style={{ color: "var(--color-accent)" }}>
            remote path mapping
          </Link>
          .
        </li>
      </ul>
    </div>
  );
}

// Page

export default function DownloadClientList() {
  const { data, isLoading, error } = useDownloadClients();
  const [modal, setModal] = useState<{ open: boolean; editing: DownloadClientConfig | null }>({
    open: false,
    editing: null,
  });

  function openCreate() { setModal({ open: true, editing: null }); }
  function openEdit(cfg: DownloadClientConfig) { setModal({ open: true, editing: cfg }); }
  function closeModal() { setModal({ open: false, editing: null }); }

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader
        title="Download Clients"
        description="Torrent and Usenet clients used to download releases."
        docsUrl={DOCS_URLS.downloadClients}
        action={
          <button
            onClick={openCreate}
            style={{
              background: "var(--color-accent)",
              color: "var(--color-accent-fg)",
              border: "none",
              borderRadius: 6,
              padding: "8px 16px",
              fontSize: 13,
              fontWeight: 500,
              cursor: "pointer",
              flexShrink: 0,
            }}
            onMouseEnter={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "var(--color-accent-hover)"; }}
            onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "var(--color-accent)"; }}
          >
            + Add Client
          </button>
        }
      />

      {/* Table card */}
      <div
        style={{
          background: "var(--color-bg-surface)",
          border: "1px solid var(--color-border-subtle)",
          borderRadius: 8,
          boxShadow: "var(--shadow-card)",
          overflow: "hidden",
        }}
      >
        {isLoading ? (
          <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 12 }}>
            {[1, 2].map((i) => (
              <div key={i} className="skeleton" style={{ height: 44, borderRadius: 4 }} />
            ))}
          </div>
        ) : error ? (
          <div style={{ padding: 24, fontSize: 13, color: "var(--color-text-muted)" }}>
            Failed to load download clients.
          </div>
        ) : !data?.length ? (
          <div style={{ padding: 48, textAlign: "center" }}>
            <p style={{ margin: 0, fontSize: 14, color: "var(--color-text-secondary)", fontWeight: 500 }}>
              No download clients configured
            </p>
            <p style={{ margin: "6px 0 0", fontSize: 13, color: "var(--color-text-muted)" }}>
              Add qBittorrent, Deluge, SABnzbd, or another client to start downloading.
            </p>
          </div>
        ) : (
          <TableScroll minWidth={700}>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
              <thead>
                <tr style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
                  {["Name", "Type", "Priority", "Status", ""].map((h) => (
                  <th
                    key={h}
                    style={{
                      textAlign: "left",
                      padding: "10px 16px",
                      fontSize: 11,
                      fontWeight: 600,
                      letterSpacing: "0.08em",
                      textTransform: "uppercase",
                      color: "var(--color-text-muted)",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.map((cfg, i) => (
                <tr
                  key={cfg.id}
                  style={{
                    borderBottom: i < data.length - 1 ? "1px solid var(--color-border-subtle)" : "none",
                  }}
                >
                  <td style={{ padding: "0 16px", height: 52, color: "var(--color-text-primary)", fontWeight: 500 }}>
                    {cfg.name}
                  </td>
                  <td style={{ padding: "0 16px", height: 52 }}>
                    <KindBadge kind={cfg.kind} />
                  </td>
                  <td style={{ padding: "0 16px", height: 52, color: "var(--color-text-secondary)" }}>
                    {cfg.priority}
                  </td>
                  <td style={{ padding: "0 16px", height: 52 }}>
                    <span
                      style={{
                        display: "inline-flex",
                        alignItems: "center",
                        gap: 6,
                        fontSize: 12,
                        color: cfg.enabled ? "var(--color-success)" : "var(--color-text-muted)",
                      }}
                    >
                      <span
                        style={{
                          width: 6,
                          height: 6,
                          borderRadius: "50%",
                          background: cfg.enabled ? "var(--color-success)" : "var(--color-text-muted)",
                          flexShrink: 0,
                        }}
                      />
                      {cfg.enabled ? "Enabled" : "Disabled"}
                    </span>
                  </td>
                  <td style={{ padding: "0 16px", height: 52, width: 1 }}>
                    <RowActions client={cfg} onEdit={() => openEdit(cfg)} />
                  </td>
                </tr>
              ))}
            </tbody>
            </table>
          </TableScroll>
        )}
      </div>

      <DownloadHandlingNote />

      {/* Modal */}
      {modal.open && (
        <DownloadClientModal editing={modal.editing} onClose={closeModal} />
      )}
    </div>
  );
}
