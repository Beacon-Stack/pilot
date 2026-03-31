import { describe, it, expect } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useCommandPalette } from "./useCommandPalette";

describe("useCommandPalette", () => {
  it("starts closed", () => {
    const { result } = renderHook(() => useCommandPalette());
    expect(result.current.isOpen).toBe(false);
  });

  it("opens with open()", () => {
    const { result } = renderHook(() => useCommandPalette());
    act(() => result.current.open());
    expect(result.current.isOpen).toBe(true);
  });

  it("closes with close()", () => {
    const { result } = renderHook(() => useCommandPalette());
    act(() => result.current.open());
    act(() => result.current.close());
    expect(result.current.isOpen).toBe(false);
  });

  it("toggles on Ctrl+K", () => {
    const { result } = renderHook(() => useCommandPalette());

    act(() => {
      window.dispatchEvent(
        new KeyboardEvent("keydown", { key: "k", ctrlKey: true })
      );
    });
    expect(result.current.isOpen).toBe(true);

    act(() => {
      window.dispatchEvent(
        new KeyboardEvent("keydown", { key: "k", ctrlKey: true })
      );
    });
    expect(result.current.isOpen).toBe(false);
  });

  it("toggles on Meta+K (Cmd on Mac)", () => {
    const { result } = renderHook(() => useCommandPalette());

    act(() => {
      window.dispatchEvent(
        new KeyboardEvent("keydown", { key: "k", metaKey: true })
      );
    });
    expect(result.current.isOpen).toBe(true);
  });

  it("does not toggle on plain K key", () => {
    const { result } = renderHook(() => useCommandPalette());

    act(() => {
      window.dispatchEvent(
        new KeyboardEvent("keydown", { key: "k" })
      );
    });
    expect(result.current.isOpen).toBe(false);
  });
});
