import { Eye, EyeOff, Search, X } from "lucide-react";

interface Props {
  count: number;
  onMonitor: () => void;
  onUnmonitor: () => void;
  onSearch: () => void;
  onClear: () => void;
}

export default function BulkActionBar({ count, onMonitor, onUnmonitor, onSearch, onClear }: Props) {
  if (count === 0) return null;

  return (
    <div style={{
      position: "fixed",
      bottom: 20,
      left: "50%",
      transform: "translateX(-50%)",
      zIndex: 90,
      display: "flex",
      alignItems: "center",
      gap: 10,
      padding: "8px 16px",
      borderRadius: 10,
      background: "var(--color-bg-elevated)",
      border: "1px solid var(--color-border-default)",
      boxShadow: "0 8px 32px rgba(0,0,0,0.4)",
      fontSize: 13,
    }}>
      <span style={{ fontWeight: 600, color: "var(--color-text-primary)" }}>
        {count} episode{count !== 1 ? "s" : ""}
      </span>

      <div style={{ width: 1, height: 20, background: "var(--color-border-subtle)" }} />

      <button onClick={onMonitor} style={btnStyle}>
        <Eye size={13} /> Monitor
      </button>
      <button onClick={onUnmonitor} style={btnStyle}>
        <EyeOff size={13} /> Unmonitor
      </button>
      <button onClick={onSearch} style={btnStyle}>
        <Search size={13} /> Search
      </button>

      <div style={{ width: 1, height: 20, background: "var(--color-border-subtle)" }} />

      <button onClick={onClear} style={{ ...btnStyle, color: "var(--color-text-muted)" }}>
        <X size={13} /> Clear
      </button>
    </div>
  );
}

const btnStyle: React.CSSProperties = {
  display: "flex", alignItems: "center", gap: 4,
  padding: "4px 10px", borderRadius: 5,
  border: "1px solid var(--color-border-default)",
  background: "none", cursor: "pointer",
  fontSize: 12, fontWeight: 500, color: "var(--color-text-secondary)",
};
