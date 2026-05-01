import type { ReactNode, CSSProperties } from "react";

// TableScroll wraps a wide <table> (or any wide content) in a
// horizontal-scroll container so it doesn't overflow the page on
// narrow viewports. The vertical axis is unconstrained — page-level
// scrolling handles row height.
//
// Pattern: wrap any table whose minimum total column width exceeds the
// available content area at the smallest supported viewport. As of
// Phase 5 of the responsive pass, that means basically every table —
// the apps target 768px width minimum and most tables have ≥4 columns
// of headed cells, which exceeds 600px after sidebar.
//
// minWidth defaults to 0 (intrinsic). Pass an explicit value when the
// inner table has columns whose content can collapse below their
// header width — set minWidth large enough that the headers stay
// readable.
//
// CSS-only: no event listeners, no ResizeObserver. The container
// scrolls horizontally on its own when content overflows.

interface TableScrollProps {
  children: ReactNode;
  // Optional minimum width for the inner content. Defaults to no
  // minimum (the table sets its own intrinsic width).
  minWidth?: number | string;
  // Optional override for the wrapper style.
  style?: CSSProperties;
}

export default function TableScroll({
  children,
  minWidth,
  style,
}: TableScrollProps) {
  return (
    <div
      style={{
        overflowX: "auto",
        // Hide the vertical scrollbar — the table doesn't have its
        // own height; page scroll handles it.
        overflowY: "visible",
        // Subtle scrollbar styling so it doesn't dominate.
        scrollbarWidth: "thin",
        // WebKit-only fallback for narrow scrollbar.
        WebkitOverflowScrolling: "touch",
        ...style,
      }}
    >
      {minWidth !== undefined ? (
        <div style={{ minWidth }}>{children}</div>
      ) : (
        children
      )}
    </div>
  );
}
