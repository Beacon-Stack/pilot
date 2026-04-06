// ── Theme system ──────────────────────────────────────────────────────────────
// Themes are stored in localStorage and applied by setting CSS custom properties
// on document.documentElement. The default values live in index.css; themes
// override them at runtime without any server round-trip.

export type ThemeMode = "dark" | "light" | "system";

export interface ThemePreset {
  id: string;
  label: string;
  mode: "dark" | "light";
  /** Preview swatch colors */
  preview: { bg: string; surface: string; accent: string; text: string };
  vars: {
    "bg-base": string;
    "bg-surface": string;
    "bg-elevated": string;
    "bg-subtle": string;
    "border-subtle": string;
    "border-default": string;
    "border-strong": string;
    accent: string;
    "accent-hover": string;
    "accent-muted": string;
    "accent-fg": string;
    "text-primary": string;
    "text-secondary": string;
    "text-muted": string;
    success: string;
    warning: string;
    danger: string;
  };
}

// ── Presets ───────────────────────────────────────────────────────────────────

export const THEME_PRESETS: ThemePreset[] = [
  // ── Dark themes ──────────────────────────────────────────────────────────
  {
    id: "pilot",
    label: "Pilot",
    mode: "dark",
    preview: { bg: "#0d0d12", surface: "#13131a", accent: "#7c6af7", text: "#f0f0f5" },
    vars: {
      "bg-base": "#0d0d12",
      "bg-surface": "#13131a",
      "bg-elevated": "#1c1c27",
      "bg-subtle": "#252535",
      "border-subtle": "rgba(255,255,255,0.078)",
      "border-default": "rgba(255,255,255,0.133)",
      "border-strong": "rgba(255,255,255,0.267)",
      accent: "#7c6af7",
      "accent-hover": "#9283f9",
      "accent-muted": "rgba(124,106,247,0.12)",
      "accent-fg": "#ffffff",
      "text-primary": "#f0f0f5",
      "text-secondary": "#9898b0",
      "text-muted": "#5a5a72",
      success: "#34d399",
      warning: "#fbbf24",
      danger: "#f87171",
    },
  },
  {
    id: "catppuccin-mocha",
    label: "Catppuccin Mocha",
    mode: "dark",
    preview: { bg: "#1e1e2e", surface: "#181825", accent: "#cba6f7", text: "#cdd6f4" },
    vars: {
      "bg-base": "#1e1e2e",
      "bg-surface": "#181825",
      "bg-elevated": "#313244",
      "bg-subtle": "#45475a",
      "border-subtle": "rgba(203,166,247,0.10)",
      "border-default": "rgba(203,166,247,0.16)",
      "border-strong": "rgba(203,166,247,0.30)",
      accent: "#cba6f7",
      "accent-hover": "#b4befe",
      "accent-muted": "rgba(203,166,247,0.12)",
      "accent-fg": "#1e1e2e",
      "text-primary": "#cdd6f4",
      "text-secondary": "#bac2de",
      "text-muted": "#7f849c",
      success: "#a6e3a1",
      warning: "#f9e2af",
      danger: "#f38ba8",
    },
  },
  {
    id: "catppuccin-macchiato",
    label: "Catppuccin Macchiato",
    mode: "dark",
    preview: { bg: "#1e2030", surface: "#181926", accent: "#c6a0f6", text: "#cad3f5" },
    vars: {
      "bg-base": "#1e2030",
      "bg-surface": "#181926",
      "bg-elevated": "#2d3044",
      "bg-subtle": "#363a4f",
      "border-subtle": "rgba(198,160,246,0.10)",
      "border-default": "rgba(198,160,246,0.16)",
      "border-strong": "rgba(198,160,246,0.30)",
      accent: "#c6a0f6",
      "accent-hover": "#b7bdf8",
      "accent-muted": "rgba(198,160,246,0.12)",
      "accent-fg": "#1e2030",
      "text-primary": "#cad3f5",
      "text-secondary": "#b8c0e0",
      "text-muted": "#6e738d",
      success: "#a6da95",
      warning: "#eed49f",
      danger: "#ed8796",
    },
  },
  {
    id: "dracula",
    label: "Dracula",
    mode: "dark",
    preview: { bg: "#1e1f29", surface: "#282a36", accent: "#bd93f9", text: "#f8f8f2" },
    vars: {
      "bg-base": "#1e1f29",
      "bg-surface": "#282a36",
      "bg-elevated": "#343746",
      "bg-subtle": "#44475a",
      "border-subtle": "rgba(189,147,249,0.10)",
      "border-default": "rgba(189,147,249,0.16)",
      "border-strong": "rgba(189,147,249,0.30)",
      accent: "#bd93f9",
      "accent-hover": "#caa9fa",
      "accent-muted": "rgba(189,147,249,0.12)",
      "accent-fg": "#282a36",
      "text-primary": "#f8f8f2",
      "text-secondary": "#cfcfe2",
      "text-muted": "#6272a4",
      success: "#50fa7b",
      warning: "#f1fa8c",
      danger: "#ff5555",
    },
  },
  {
    id: "nord",
    label: "Nord",
    mode: "dark",
    preview: { bg: "#232831", surface: "#2e3440", accent: "#88c0d0", text: "#eceff4" },
    vars: {
      "bg-base": "#232831",
      "bg-surface": "#2e3440",
      "bg-elevated": "#3b4252",
      "bg-subtle": "#434c5e",
      "border-subtle": "rgba(136,192,208,0.10)",
      "border-default": "rgba(136,192,208,0.16)",
      "border-strong": "rgba(136,192,208,0.30)",
      accent: "#88c0d0",
      "accent-hover": "#8fbcbb",
      "accent-muted": "rgba(136,192,208,0.12)",
      "accent-fg": "#2e3440",
      "text-primary": "#eceff4",
      "text-secondary": "#d8dee9",
      "text-muted": "#4c566a",
      success: "#a3be8c",
      warning: "#ebcb8b",
      danger: "#bf616a",
    },
  },
  {
    id: "gruvbox-dark",
    label: "Gruvbox Dark",
    mode: "dark",
    preview: { bg: "#1d2021", surface: "#282828", accent: "#458588", text: "#ebdbb2" },
    vars: {
      "bg-base": "#1d2021",
      "bg-surface": "#282828",
      "bg-elevated": "#3c3836",
      "bg-subtle": "#504945",
      "border-subtle": "rgba(69,133,136,0.10)",
      "border-default": "rgba(69,133,136,0.16)",
      "border-strong": "rgba(69,133,136,0.30)",
      accent: "#458588",
      "accent-hover": "#83a598",
      "accent-muted": "rgba(69,133,136,0.12)",
      "accent-fg": "#ebdbb2",
      "text-primary": "#ebdbb2",
      "text-secondary": "#d5c4a1",
      "text-muted": "#928374",
      success: "#b8bb26",
      warning: "#fabd2f",
      danger: "#fb4934",
    },
  },
  {
    id: "one-dark",
    label: "One Dark",
    mode: "dark",
    preview: { bg: "#1b1f27", surface: "#21252b", accent: "#61afef", text: "#abb2bf" },
    vars: {
      "bg-base": "#1b1f27",
      "bg-surface": "#21252b",
      "bg-elevated": "#282c34",
      "bg-subtle": "#2c313a",
      "border-subtle": "rgba(97,175,239,0.10)",
      "border-default": "rgba(97,175,239,0.16)",
      "border-strong": "rgba(97,175,239,0.30)",
      accent: "#61afef",
      "accent-hover": "#56b6c2",
      "accent-muted": "rgba(97,175,239,0.12)",
      "accent-fg": "#21252b",
      "text-primary": "#abb2bf",
      "text-secondary": "#9da5b4",
      "text-muted": "#5c6370",
      success: "#98c379",
      warning: "#e5c07b",
      danger: "#e06c75",
    },
  },
  {
    id: "rose-pine",
    label: "Rosé Pine",
    mode: "dark",
    preview: { bg: "#191724", surface: "#1f1d2e", accent: "#c4a7e7", text: "#e0def4" },
    vars: {
      "bg-base": "#191724",
      "bg-surface": "#1f1d2e",
      "bg-elevated": "#26233a",
      "bg-subtle": "#312e45",
      "border-subtle": "rgba(196,167,231,0.10)",
      "border-default": "rgba(196,167,231,0.16)",
      "border-strong": "rgba(196,167,231,0.30)",
      accent: "#c4a7e7",
      "accent-hover": "#d4b9f0",
      "accent-muted": "rgba(196,167,231,0.12)",
      "accent-fg": "#191724",
      "text-primary": "#e0def4",
      "text-secondary": "#c8c3d0",
      "text-muted": "#6e6a86",
      success: "#31748f",
      warning: "#f6c177",
      danger: "#eb6f92",
    },
  },
  {
    id: "kanagawa",
    label: "Kanagawa",
    mode: "dark",
    preview: { bg: "#1f1f28", surface: "#16161d", accent: "#7e9cd8", text: "#dcd7ba" },
    vars: {
      "bg-base": "#1f1f28",
      "bg-surface": "#16161d",
      "bg-elevated": "#2a2a37",
      "bg-subtle": "#363646",
      "border-subtle": "rgba(126,156,216,0.10)",
      "border-default": "rgba(126,156,216,0.16)",
      "border-strong": "rgba(126,156,216,0.30)",
      accent: "#7e9cd8",
      "accent-hover": "#98bb6c",
      "accent-muted": "rgba(126,156,216,0.12)",
      "accent-fg": "#1f1f28",
      "text-primary": "#dcd7ba",
      "text-secondary": "#c8c093",
      "text-muted": "#727169",
      success: "#76946a",
      warning: "#dca561",
      danger: "#c34043",
    },
  },
  {
    id: "tokyo-night",
    label: "Tokyo Night",
    mode: "dark",
    preview: { bg: "#1a1b26", surface: "#16161e", accent: "#7aa2f7", text: "#c0caf5" },
    vars: {
      "bg-base": "#1a1b26",
      "bg-surface": "#16161e",
      "bg-elevated": "#1f2335",
      "bg-subtle": "#292e42",
      "border-subtle": "rgba(122,162,247,0.10)",
      "border-default": "rgba(122,162,247,0.16)",
      "border-strong": "rgba(122,162,247,0.30)",
      accent: "#7aa2f7",
      "accent-hover": "#89b4fa",
      "accent-muted": "rgba(122,162,247,0.12)",
      "accent-fg": "#1a1b26",
      "text-primary": "#c0caf5",
      "text-secondary": "#a9b1d6",
      "text-muted": "#565f89",
      success: "#9ece6a",
      warning: "#e0af68",
      danger: "#f7768e",
    },
  },

  // ── Light themes ─────────────────────────────────────────────────────────
  {
    id: "gruvbox-light",
    label: "Gruvbox Light",
    mode: "light",
    preview: { bg: "#f9f5d7", surface: "#fbf1c7", accent: "#458588", text: "#3c3836" },
    vars: {
      "bg-base": "#f9f5d7",
      "bg-surface": "#fbf1c7",
      "bg-elevated": "#edded0",
      "bg-subtle": "#d5c4a1",
      "border-subtle": "rgba(102,92,84,0.10)",
      "border-default": "rgba(102,92,84,0.16)",
      "border-strong": "rgba(102,92,84,0.30)",
      accent: "#458588",
      "accent-hover": "#689d6a",
      "accent-muted": "rgba(69,133,136,0.12)",
      "accent-fg": "#fbf1c7",
      "text-primary": "#3c3836",
      "text-secondary": "#504945",
      "text-muted": "#928374",
      success: "#79740e",
      warning: "#b57614",
      danger: "#9d0006",
    },
  },
  {
    id: "catppuccin-latte",
    label: "Catppuccin Latte",
    mode: "light",
    preview: { bg: "#eff1f5", surface: "#e6e9ef", accent: "#7287fd", text: "#4c4f69" },
    vars: {
      "bg-base": "#eff1f5",
      "bg-surface": "#e6e9ef",
      "bg-elevated": "#ccd0da",
      "bg-subtle": "#bcc0cc",
      "border-subtle": "rgba(76,79,105,0.10)",
      "border-default": "rgba(76,79,105,0.16)",
      "border-strong": "rgba(76,79,105,0.30)",
      accent: "#7287fd",
      "accent-hover": "#5c70f0",
      "accent-muted": "rgba(114,135,253,0.12)",
      "accent-fg": "#ffffff",
      "text-primary": "#4c4f69",
      "text-secondary": "#5c5f77",
      "text-muted": "#8c8fa1",
      success: "#40a02b",
      warning: "#df8e1d",
      danger: "#d20f39",
    },
  },
  {
    id: "solarized-light",
    label: "Solarized Light",
    mode: "light",
    preview: { bg: "#fdf6e3", surface: "#eee8d5", accent: "#268bd2", text: "#657b83" },
    vars: {
      "bg-base": "#fdf6e3",
      "bg-surface": "#eee8d5",
      "bg-elevated": "#e3dcc9",
      "bg-subtle": "#d8cfb2",
      "border-subtle": "rgba(88,110,117,0.10)",
      "border-default": "rgba(88,110,117,0.16)",
      "border-strong": "rgba(88,110,117,0.30)",
      accent: "#268bd2",
      "accent-hover": "#2aa198",
      "accent-muted": "rgba(38,139,210,0.12)",
      "accent-fg": "#fdf6e3",
      "text-primary": "#657b83",
      "text-secondary": "#586e75",
      "text-muted": "#839496",
      success: "#859900",
      warning: "#b58900",
      danger: "#dc322f",
    },
  },
];

export const DEFAULT_DARK_PRESET = "pilot";
export const DEFAULT_LIGHT_PRESET = "catppuccin-latte";

// ── Storage keys ──────────────────────────────────────────────────────────────

const KEY_MODE = "pilot-theme-mode";
const KEY_DARK = "pilot-theme-dark";
const KEY_LIGHT = "pilot-theme-light";

// ── Apply ─────────────────────────────────────────────────────────────────────

function applyPreset(preset: ThemePreset): void {
  const root = document.documentElement;
  const v = preset.vars;
  root.style.setProperty("--color-bg-base", v["bg-base"]);
  root.style.setProperty("--color-bg-surface", v["bg-surface"]);
  root.style.setProperty("--color-bg-elevated", v["bg-elevated"]);
  root.style.setProperty("--color-bg-subtle", v["bg-subtle"]);
  root.style.setProperty("--color-border-subtle", v["border-subtle"]);
  root.style.setProperty("--color-border-default", v["border-default"]);
  root.style.setProperty("--color-border-strong", v["border-strong"]);
  root.style.setProperty("--color-accent", v["accent"]);
  root.style.setProperty("--color-accent-hover", v["accent-hover"]);
  root.style.setProperty("--color-accent-muted", v["accent-muted"]);
  root.style.setProperty("--color-accent-fg", v["accent-fg"]);
  root.style.setProperty("--color-text-primary", v["text-primary"]);
  root.style.setProperty("--color-text-secondary", v["text-secondary"]);
  root.style.setProperty("--color-text-muted", v["text-muted"]);
  root.style.setProperty("--color-success", v["success"]);
  root.style.setProperty("--color-warning", v["warning"]);
  root.style.setProperty("--color-danger", v["danger"]);
}

/** Resolve the effective mode (dark or light) given the stored mode. */
export function resolveMode(mode: ThemeMode): "dark" | "light" {
  if (mode === "system") {
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }
  return mode;
}

/** Read stored mode from localStorage. */
export function getStoredMode(): ThemeMode {
  const raw = localStorage.getItem(KEY_MODE);
  if (raw === "dark" || raw === "light" || raw === "system") return raw;
  return "dark";
}

/** Read stored preset ID for a given resolved mode. */
export function getStoredPreset(resolvedMode: "dark" | "light"): string {
  const key = resolvedMode === "dark" ? KEY_DARK : KEY_LIGHT;
  return localStorage.getItem(key) ?? (resolvedMode === "dark" ? DEFAULT_DARK_PRESET : DEFAULT_LIGHT_PRESET);
}

/** Find a preset by ID, falling back to the Pilot default. */
export function findPreset(id: string): ThemePreset {
  return THEME_PRESETS.find((p) => p.id === id) ?? THEME_PRESETS[0];
}

/**
 * Apply the persisted theme to the document. Call this once on app startup
 * and whenever the theme changes.
 */
export function applyTheme(): void {
  const mode = getStoredMode();
  const resolved = resolveMode(mode);
  const presetId = getStoredPreset(resolved);
  applyPreset(findPreset(presetId));
}

/** Set the mode and immediately re-apply. */
export function setThemeMode(mode: ThemeMode): void {
  localStorage.setItem(KEY_MODE, mode);
  applyTheme();
}

/** Set the preset for a given mode and immediately re-apply if it's the active mode. */
export function setThemePreset(resolvedMode: "dark" | "light", presetId: string): void {
  const key = resolvedMode === "dark" ? KEY_DARK : KEY_LIGHT;
  localStorage.setItem(key, presetId);
  const currentResolved = resolveMode(getStoredMode());
  if (currentResolved === resolvedMode) {
    applyPreset(findPreset(presetId));
  }
}

// ── UI Preferences ───────────────────────────────────────────────────────────

const KEY_TOOLTIPS = "pilot-ui-tooltips";

/** Read whether tooltips are enabled (default: true). */
export function getTooltipsEnabled(): boolean {
  return localStorage.getItem(KEY_TOOLTIPS) !== "false";
}

/** Persist the tooltips preference. */
export function setTooltipsEnabled(enabled: boolean): void {
  localStorage.setItem(KEY_TOOLTIPS, String(enabled));
}
