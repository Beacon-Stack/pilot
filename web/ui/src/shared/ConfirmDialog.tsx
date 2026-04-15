import { createContext, useCallback, useContext, useRef, useState } from "react";
import type { ReactNode } from "react";
import Modal from "./Modal";

interface ConfirmOptions {
  title: string;
  message: string;
  confirmLabel?: string;
  /** Color the confirm button red. Defaults to true. */
  danger?: boolean;
}

type ConfirmFn = (opts: ConfirmOptions) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFn | null>(null);

/**
 * useConfirm returns a function that opens the shared confirm modal and
 * resolves to true/false when the user clicks a button or dismisses. Use
 * this instead of `window.confirm()` so every service gets the same
 * themed dialog instead of the browser's native one.
 *
 * Must be called inside a <ConfirmProvider> (usually wrapped at the App
 * root). Throws if no provider is mounted — this is intentional: a bare
 * call in a test or untouched service is a louder failure than a silent
 * fallback to window.confirm.
 *
 * Example:
 *
 *   const confirm = useConfirm();
 *   if (await confirm({
 *     title: "Delete torrent",
 *     message: "This cannot be undone.",
 *     confirmLabel: "Delete",
 *   })) {
 *     deleteTorrent.mutate(...);
 *   }
 */
export function useConfirm(): ConfirmFn {
  const fn = useContext(ConfirmContext);
  if (!fn) throw new Error("useConfirm must be used within ConfirmProvider");
  return fn;
}

/**
 * ConfirmProvider wraps the React tree and provides the useConfirm() hook.
 * Mount once at the App root, above any component that uses useConfirm.
 * Rendering the provider also renders the modal itself when open, so there
 * is no separate <ConfirmModal /> to place.
 */
export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [opts, setOpts] = useState<ConfirmOptions | null>(null);
  const resolveRef = useRef<((value: boolean) => void) | null>(null);

  const confirm = useCallback<ConfirmFn>(
    (options) =>
      new Promise<boolean>((resolve) => {
        resolveRef.current = resolve;
        setOpts(options);
      }),
    [],
  );

  function close(result: boolean) {
    resolveRef.current?.(result);
    resolveRef.current = null;
    setOpts(null);
  }

  const danger = opts?.danger ?? true;
  const confirmLabel =
    opts?.confirmLabel ?? (danger ? "Delete" : "Confirm");

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {opts && (
        <Modal onClose={() => close(false)} width={400}>
          <div style={{ padding: 24 }}>
            <h3
              style={{
                margin: 0,
                fontSize: 15,
                fontWeight: 600,
                color: "var(--color-text-primary)",
              }}
            >
              {opts.title}
            </h3>
            <p
              style={{
                margin: "10px 0 0",
                fontSize: 13,
                lineHeight: 1.5,
                color: "var(--color-text-secondary)",
              }}
            >
              {opts.message}
            </p>
          </div>

          <div
            style={{
              display: "flex",
              justifyContent: "flex-end",
              gap: 8,
              padding: "12px 24px",
              borderTop: "1px solid var(--color-border-subtle)",
            }}
          >
            <button
              onClick={() => close(false)}
              style={{
                background: "var(--color-bg-elevated)",
                border: "1px solid var(--color-border-default)",
                borderRadius: 6,
                padding: "6px 14px",
                fontSize: 13,
                color: "var(--color-text-secondary)",
                cursor: "pointer",
              }}
            >
              Cancel
            </button>
            <button
              autoFocus
              onClick={() => close(true)}
              style={{
                background: danger
                  ? "var(--color-danger)"
                  : "var(--color-accent)",
                border: "none",
                borderRadius: 6,
                padding: "6px 14px",
                fontSize: 13,
                color: "#fff",
                fontWeight: 600,
                cursor: "pointer",
              }}
            >
              {confirmLabel}
            </button>
          </div>
        </Modal>
      )}
    </ConfirmContext.Provider>
  );
}
