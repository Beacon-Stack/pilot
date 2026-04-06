import { useEffect } from "react";
import type { ReactNode, CSSProperties } from "react";

interface DrawerProps {
  onClose: () => void;
  children: ReactNode;
  width?: number | string;
  innerStyle?: CSSProperties;
}

/**
 * Slide-in drawer from the right edge. Escape-to-close, click-backdrop-to-close.
 */
export default function Drawer({
  onClose,
  children,
  width = 440,
  innerStyle,
}: DrawerProps) {
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <>
      {/* Backdrop */}
      <div
        onClick={onClose}
        style={{
          position: "fixed",
          inset: 0,
          background: "rgba(0,0,0,0.4)",
          zIndex: 100,
        }}
      />
      {/* Panel */}
      <div
        style={{
          position: "fixed",
          top: 0,
          right: 0,
          width,
          maxWidth: "100vw",
          height: "100vh",
          background: "var(--color-bg-surface)",
          borderLeft: "1px solid var(--color-border-subtle)",
          zIndex: 101,
          display: "flex",
          flexDirection: "column",
          boxShadow: "-8px 0 32px rgba(0,0,0,0.3)",
          overflowY: "auto",
          ...innerStyle,
        }}
      >
        {children}
      </div>
    </>
  );
}
