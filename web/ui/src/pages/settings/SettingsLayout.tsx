import { NavLink, Outlet } from "react-router-dom";
import { Library, SlidersHorizontal, Server, Paintbrush, SearchCode, ListPlus, ArrowDownToLine } from "lucide-react";

interface SettingsNavItem {
  to: string;
  icon: React.ElementType;
  label: string;
}

const settingsNav: SettingsNavItem[] = [
  { to: "/settings/libraries",        icon: Library,           label: "Libraries" },
  { to: "/settings/quality-profiles", icon: SlidersHorizontal, label: "Quality Profiles" },
  { to: "/settings/indexers",         icon: SearchCode,        label: "Indexers" },
  { to: "/settings/import-lists",     icon: ListPlus,          label: "Import Lists" },
  { to: "/settings/import",           icon: ArrowDownToLine,   label: "Import" },
  { to: "/settings/system",           icon: Server,            label: "System" },
  { to: "/settings/app",              icon: Paintbrush,        label: "App Settings" },
];

export default function SettingsLayout() {
  return (
    <div style={{ display: "flex", minHeight: "100vh" }}>
      {/* Settings sidebar */}
      <div
        style={{
          width: 200,
          minWidth: 200,
          borderRight: "1px solid var(--color-border-subtle)",
          padding: "24px 8px",
        }}
      >
        <div
          style={{
            fontSize: 11,
            fontWeight: 600,
            letterSpacing: "0.08em",
            textTransform: "uppercase",
            color: "var(--color-text-muted)",
            padding: "0 12px",
            marginBottom: 8,
          }}
        >
          Settings
        </div>
        {settingsNav.map((item) => {
          const Icon = item.icon;
          return (
            <NavLink
              key={item.to}
              to={item.to}
              style={({ isActive }) => ({
                display: "flex",
                alignItems: "center",
                gap: 10,
                padding: "0 12px",
                height: 38,
                borderRadius: 6,
                textDecoration: "none",
                fontSize: 13,
                fontWeight: 500,
                color: isActive ? "var(--color-accent-hover)" : "var(--color-text-secondary)",
                background: isActive ? "var(--color-accent-muted)" : "transparent",
                borderLeft: isActive ? "2px solid var(--color-accent)" : "2px solid transparent",
                marginLeft: -2,
                transition: "background 150ms ease, color 150ms ease",
              })}
            >
              <Icon size={16} strokeWidth={1.5} style={{ flexShrink: 0 }} />
              {item.label}
            </NavLink>
          );
        })}
      </div>

      {/* Content area */}
      <div style={{ flex: 1, minWidth: 0, padding: "24px 28px" }}>
        <Outlet />
      </div>
    </div>
  );
}
