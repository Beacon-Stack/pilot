// Shell — shared application chrome for every Beacon Stack frontend.
//
// Renders a left sidebar (responsive: full / icons-only / mobile drawer),
// an optional mobile top bar with hamburger, and a <main> containing
// react-router's <Outlet />. Apps inject their nav config + branding
// via props.
//
// Viewport tiers, narrowest to widest:
//
//   mobile   <768px   slide-out drawer + hamburger top bar
//   compact  768-1100 sidebar force-collapsed to ~60px icons-only
//   wide     >=1100px sidebar honors the user's saved expanded/collapsed
//
// Per the web-shared convention this file imports only React and
// react-router-dom — no lucide. The four icons it needs (Menu, X,
// ChevronLeft, ChevronRight) are inlined as SVG below.

import { useState, useEffect, type ReactNode, type CSSProperties } from "react";
import { Link, NavLink, Outlet, useLocation } from "react-router-dom";

// ── Public types ─────────────────────────────────────────────────────────────

export interface NavItem {
  to: string;
  // Icon accepts any component that takes `size` and `strokeWidth` props
  // — typically a lucide-react icon. Apps already depend on lucide; the
  // type below is structural so we don't import lucide here.
  icon: React.ElementType;
  label: string;
}

export interface ShellProps {
  // ── Branding ───────────────────────────────────────────────────────────
  // Display name shown next to the app icon. Apps that fetch this from
  // their own /api/system/status pass the resolved value; static apps
  // pass a literal.
  appName: string;
  // Pre-rendered app icon (caller controls size, color, and any
  // surrounding container/circle).
  appIcon: ReactNode;
  // The path the logo links to. Defaults to "/".
  homePath?: string;

  // ── Navigation ─────────────────────────────────────────────────────────
  mainNav: NavItem[];
  // Optional second nav section (rendered under a "Settings" header).
  // Apps without sub-sections (e.g. Haul) omit this.
  settingsNav?: NavItem[];

  // ── Persistence ────────────────────────────────────────────────────────
  // localStorage key for the user's expanded/collapsed preference.
  // Per-app so different apps remember independently.
  collapsedStorageKey: string;

  // ── Footer extras ──────────────────────────────────────────────────────
  // Optional external docs URL — rendered as a "Docs" link in the
  // sidebar footer with an external-link affordance.
  docsUrl?: string;
  // Optional content rendered above the Docs link / collapse button —
  // typically a HealthDot or version display.
  sidebarFooterExtras?: ReactNode;

  // ── Overlay ────────────────────────────────────────────────────────────
  // Optional content rendered after the main <Outlet />. Apps use this
  // for full-screen overlays like a command palette modal.
  overlay?: ReactNode;
}

// ── Internal: viewport mode hook ─────────────────────────────────────────────

type ViewportMode = "mobile" | "compact" | "wide";

function computeViewportMode(): ViewportMode {
  if (typeof window === "undefined") return "wide";
  if (window.innerWidth < 768) return "mobile";
  if (window.innerWidth < 1100) return "compact";
  return "wide";
}

function useViewportMode(): ViewportMode {
  const [mode, setMode] = useState<ViewportMode>(computeViewportMode);
  useEffect(() => {
    const handler = () => setMode(computeViewportMode());
    const mqMobile = window.matchMedia("(max-width: 767px)");
    const mqCompact = window.matchMedia("(max-width: 1099px)");
    mqMobile.addEventListener("change", handler);
    mqCompact.addEventListener("change", handler);
    return () => {
      mqMobile.removeEventListener("change", handler);
      mqCompact.removeEventListener("change", handler);
    };
  }, []);
  return mode;
}

// ── Internal: inline SVG icons ───────────────────────────────────────────────
// Inlined to keep web-shared's dep surface React-only.

function MenuIcon({ size = 20 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <line x1="4" y1="6" x2="20" y2="6" />
      <line x1="4" y1="12" x2="20" y2="12" />
      <line x1="4" y1="18" x2="20" y2="18" />
    </svg>
  );
}

function XIcon({ size = 18 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <line x1="18" y1="6" x2="6" y2="18" />
      <line x1="6" y1="6" x2="18" y2="18" />
    </svg>
  );
}

function ChevronIcon({ direction, size = 16 }: { direction: "left" | "right"; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      {direction === "left" ? (
        <polyline points="15 18 9 12 15 6" />
      ) : (
        <polyline points="9 18 15 12 9 6" />
      )}
    </svg>
  );
}

function BookOpenIcon({ size = 16 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
      <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
    </svg>
  );
}

// ── Internal: SidebarNavItem ─────────────────────────────────────────────────

function SidebarNavItem({
  item,
  collapsed,
  onClick,
}: {
  item: NavItem;
  collapsed: boolean;
  onClick?: () => void;
}) {
  const Icon = item.icon;
  return (
    <NavLink
      to={item.to}
      end={item.to === "/"}
      title={item.label}
      onClick={onClick}
      style={({ isActive }) => ({
        display: "flex",
        alignItems: "center",
        gap: "10px",
        padding: "0 12px",
        height: "40px",
        borderRadius: "6px",
        textDecoration: "none",
        fontSize: "14px",
        fontWeight: 500,
        whiteSpace: "nowrap",
        overflow: "hidden",
        transition: "background 150ms ease, color 150ms ease",
        borderLeft: isActive
          ? "2px solid var(--color-accent)"
          : "2px solid transparent",
        background: isActive ? "var(--color-accent-muted)" : "transparent",
        color: isActive
          ? "var(--color-accent-hover)"
          : "var(--color-text-secondary)",
        marginLeft: "-2px",
      })}
    >
      <Icon size={18} strokeWidth={1.5} style={{ flexShrink: 0 }} />
      {!collapsed && (
        <span
          style={{
            // Soft-clip with ellipsis so long labels like "Quality
            // Definitions" show "Quality Defi…" instead of hard-cutting.
            overflow: "hidden",
            textOverflow: "ellipsis",
            minWidth: 0,
          }}
        >
          {item.label}
        </span>
      )}
    </NavLink>
  );
}

// ── Internal: Sidebar ────────────────────────────────────────────────────────

function Sidebar({
  collapsed,
  onCollapse,
  onClose,
  isMobile,
  autoCollapsed,
  appName,
  appIcon,
  homePath,
  mainNav,
  settingsNav,
  docsUrl,
  sidebarFooterExtras,
}: {
  collapsed: boolean;
  onCollapse: () => void;
  onClose: () => void;
  isMobile: boolean;
  autoCollapsed: boolean;
  appName: string;
  appIcon: ReactNode;
  homePath: string;
  mainNav: NavItem[];
  settingsNav?: NavItem[];
  docsUrl?: string;
  sidebarFooterExtras?: ReactNode;
}) {
  const width = isMobile ? 240 : collapsed ? 60 : 240;
  const showLabels = !isMobile && !collapsed;

  return (
    <nav
      style={{
        width,
        minWidth: width,
        maxWidth: width,
        background: "var(--color-bg-surface)",
        borderRight: "1px solid var(--color-border-subtle)",
        display: "flex",
        flexDirection: "column",
        transition:
          "width 200ms ease, min-width 200ms ease, max-width 200ms ease",
        overflow: "hidden",
        position: "fixed",
        top: 0,
        left: 0,
        height: "100vh",
        zIndex: 50,
      }}
    >
      {/* Logo */}
      <div
        style={{
          height: "60px",
          display: "flex",
          alignItems: "center",
          padding: "0 14px",
          borderBottom: "1px solid var(--color-border-subtle)",
          flexShrink: 0,
        }}
      >
        <Link
          to={homePath}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 10,
            textDecoration: "none",
            flex: 1,
            minWidth: 0,
          }}
        >
          {appIcon}
          {(showLabels || isMobile) && (
            <span
              style={{
                fontSize: "16px",
                fontWeight: 700,
                color: "var(--color-text-primary)",
                letterSpacing: "-0.01em",
                whiteSpace: "nowrap",
                overflow: "hidden",
                textOverflow: "ellipsis",
              }}
            >
              {appName}
            </span>
          )}
        </Link>
        {isMobile && (
          <button
            onClick={onClose}
            style={{
              background: "none",
              border: "none",
              cursor: "pointer",
              color: "var(--color-text-muted)",
              display: "flex",
              alignItems: "center",
              padding: 4,
              marginLeft: "auto",
            }}
            title="Close menu"
          >
            <XIcon />
          </button>
        )}
      </div>

      {/* Nav items */}
      <div
        style={{
          flex: 1,
          overflowY: "auto",
          overflowX: "hidden",
          padding: "12px 8px",
          display: "flex",
          flexDirection: "column",
          gap: "2px",
        }}
      >
        {mainNav.map((item) => (
          <SidebarNavItem
            key={item.to}
            item={item}
            collapsed={!isMobile && collapsed}
            onClick={isMobile ? onClose : undefined}
          />
        ))}

        {settingsNav && settingsNav.length > 0 && (
          <>
            <div
              style={{
                // Margins must collapse to 0 alongside height so the band
                // disappears entirely when the sidebar is collapsed.
                margin: !isMobile && collapsed ? "0" : "12px 4px 4px",
                fontSize: "11px",
                fontWeight: 500,
                color: "var(--color-text-muted)",
                letterSpacing: "0.08em",
                textTransform: "uppercase",
                whiteSpace: "nowrap",
                overflow: "hidden",
                height: !isMobile && collapsed ? "0" : "auto",
                opacity: !isMobile && collapsed ? 0 : 1,
                transition:
                  "opacity 150ms ease, height 150ms ease, margin 150ms ease",
              }}
            >
              Settings
            </div>

            {settingsNav.map((item) => (
              <SidebarNavItem
                key={item.to}
                item={item}
                collapsed={!isMobile && collapsed}
                onClick={isMobile ? onClose : undefined}
              />
            ))}
          </>
        )}
      </div>

      {/* Bottom area */}
      <div
        style={{
          borderTop: "1px solid var(--color-border-subtle)",
          padding: "8px",
          display: "flex",
          flexDirection: "column",
          gap: "4px",
        }}
      >
        {sidebarFooterExtras}
        {docsUrl && (
          <a
            href={docsUrl}
            target="_blank"
            rel="noopener noreferrer"
            style={{
              display: "flex",
              alignItems: "center",
              gap: "8px",
              padding: "0 12px",
              height: "36px",
              color: "var(--color-text-muted)",
              fontSize: "12px",
              textDecoration: "none",
              borderRadius: "6px",
              transition: "color 150ms ease",
              justifyContent: !isMobile && collapsed ? "center" : "flex-start",
            }}
            title={!isMobile && collapsed ? "Documentation" : undefined}
          >
            <BookOpenIcon />
            {(isMobile || !collapsed) && <span>Docs</span>}
          </a>
        )}
        {!isMobile && !autoCollapsed && (
          <button
            onClick={onCollapse}
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: collapsed ? "center" : "flex-end",
              width: "100%",
              padding: "0 12px",
              height: "36px",
              background: "none",
              border: "none",
              cursor: "pointer",
              color: "var(--color-text-muted)",
              borderRadius: "6px",
              transition: "background 150ms ease, color 150ms ease",
            }}
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            <ChevronIcon direction={collapsed ? "right" : "left"} />
          </button>
        )}
      </div>
    </nav>
  );
}

// ── Public: Shell ────────────────────────────────────────────────────────────

export default function Shell({
  appName,
  appIcon,
  homePath = "/",
  mainNav,
  settingsNav,
  collapsedStorageKey,
  docsUrl,
  sidebarFooterExtras,
  overlay,
}: ShellProps) {
  const [userCollapsed, setUserCollapsed] = useState(() => {
    return localStorage.getItem(collapsedStorageKey) === "true";
  });
  const [mobileOpen, setMobileOpen] = useState(false);
  const mode = useViewportMode();
  const isMobile = mode === "mobile";

  // In compact mode (768–1100px) the sidebar is force-collapsed
  // regardless of saved preference — the right pane needs every pixel.
  const collapsed = mode === "compact" ? true : userCollapsed;

  useEffect(() => {
    if (!isMobile) setMobileOpen(false);
  }, [isMobile]);

  useEffect(() => {
    localStorage.setItem(collapsedStorageKey, String(userCollapsed));
  }, [userCollapsed, collapsedStorageKey]);

  // Close the mobile drawer + scroll to top on every route change.
  const location = useLocation();
  useEffect(() => {
    window.scrollTo(0, 0);
    setMobileOpen(false);
  }, [location.pathname]);

  const desktopWidth = collapsed ? 60 : 240;

  return (
    <div style={{ display: "flex", minHeight: "100vh" }}>
      {/* Mobile overlay backdrop */}
      {isMobile && mobileOpen && (
        <div
          onClick={() => setMobileOpen(false)}
          style={{
            position: "fixed",
            inset: 0,
            background: "rgba(0,0,0,0.5)",
            zIndex: 49,
          }}
        />
      )}

      {/* Sidebar wrapper — slides in/out on mobile */}
      <div
        style={{
          transform: isMobile
            ? mobileOpen
              ? "translateX(0)"
              : "translateX(-100%)"
            : "none",
          transition: "transform 200ms ease",
        }}
      >
        <Sidebar
          collapsed={collapsed}
          onCollapse={() => setUserCollapsed((c) => !c)}
          onClose={() => setMobileOpen(false)}
          isMobile={isMobile}
          autoCollapsed={mode === "compact"}
          appName={appName}
          appIcon={appIcon}
          homePath={homePath}
          mainNav={mainNav}
          settingsNav={settingsNav}
          docsUrl={docsUrl}
          sidebarFooterExtras={sidebarFooterExtras}
        />
      </div>

      {/* Main content */}
      <main
        style={{
          flex: 1,
          marginLeft: isMobile ? 0 : desktopWidth,
          transition: "margin-left 200ms ease",
          minWidth: 0,
        }}
      >
        {/* Mobile top bar */}
        {isMobile && (
          <div
            style={{
              position: "sticky",
              top: 0,
              zIndex: 40,
              height: 52,
              background: "var(--color-bg-surface)",
              borderBottom: "1px solid var(--color-border-subtle)",
              display: "flex",
              alignItems: "center",
              padding: "0 16px",
              gap: 12,
            }}
          >
            <button
              onClick={() => setMobileOpen(true)}
              style={{
                background: "none",
                border: "none",
                cursor: "pointer",
                color: "var(--color-text-secondary)",
                display: "flex",
                alignItems: "center",
                padding: 4,
                borderRadius: 6,
              }}
              title="Open menu"
            >
              <MenuIcon />
            </button>
            <Link
              to={homePath}
              style={MOBILE_TOP_TITLE_LINK_STYLE}
            >
              {appIcon}
              <span
                style={{
                  fontSize: "15px",
                  fontWeight: 700,
                  color: "var(--color-text-primary)",
                  letterSpacing: "-0.01em",
                }}
              >
                {appName}
              </span>
            </Link>
          </div>
        )}

        <Outlet />
      </main>

      {overlay}
    </div>
  );
}

const MOBILE_TOP_TITLE_LINK_STYLE: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 8,
  textDecoration: "none",
};
