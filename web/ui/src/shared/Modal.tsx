import { useEffect } from "react";
import type { ReactNode, CSSProperties } from "react";

interface ModalProps {
  onClose: () => void;
  children: ReactNode;
  /** Width of the inner container. Default: 520. */
  width?: number | string;
  maxWidth?: string;
  maxHeight?: string;
  /** Extra styles merged onto the inner container. */
  innerStyle?: CSSProperties;
}

/**
 * Generic modal shell — backdrop overlay, centered content, Escape-to-close,
 * click-outside-to-close. All modals across every Beacon service frontend
 * should use this so behaviour stays consistent and escape/click-away logic
 * isn't duplicated per service.
 *
 * Colors come from CSS custom properties (`--color-bg-surface`,
 * `--color-border-subtle`, `--shadow-modal`). Each service defines its own
 * theme; this component doesn't know or care which.
 */
export default function Modal({
  onClose,
  children,
  width = 520,
  maxWidth = "calc(100vw - 48px)",
  maxHeight = "calc(100vh - 80px)",
  innerStyle,
}: ModalProps) {
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  // Detect mobile viewport (matches the Shell.tsx breakpoint exactly so
  // mobile-mode modal styling kicks in at the same tier as the
  // sidebar-becomes-drawer transition). At ≤640px the modal goes full
  // bleed: edge-to-edge width, no border-radius, no surrounding margin.
  // The 640 threshold is tighter than Shell's 768 because tablets in
  // landscape (768-1023) still benefit from the centered modal look;
  // it's only phones where the centered modal wastes space + makes the
  // close affordance hard to thumb-reach.
  const isPhone =
    typeof window !== "undefined" && window.innerWidth <= 640;

  const phoneStyles: CSSProperties = isPhone
    ? {
        width: "100vw",
        maxWidth: "100vw",
        maxHeight: "100vh",
        height: "100vh",
        borderRadius: 0,
        border: "none",
      }
    : {};

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.6)",
        backdropFilter: "blur(2px)",
        display: "flex",
        alignItems: isPhone ? "stretch" : "center",
        justifyContent: "center",
        zIndex: 200,
      }}
      onClick={onClose}
    >
      <div
        style={{
          background: "var(--color-bg-surface)",
          border: "1px solid var(--color-border-subtle)",
          borderRadius: 12,
          width,
          maxWidth,
          maxHeight,
          boxShadow: "var(--shadow-modal)",
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
          ...innerStyle,
          ...phoneStyles,
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
}
