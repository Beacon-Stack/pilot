import { useRef, useState } from "react";

// ── Scale ──────────────────────────────────────────────────────────────────────
// Logarithmic mapping between slider position [0, 1] and MB/min value [0, 800].
// Position 0 always means "0" (no minimum / no-limit sentinel for min).
// Max thumb at position ≥ 0.99 maps to stored value 0 (= "no limit").

const SLIDER_MAX = 800;

function valueToPosition(value: number): number {
  if (value <= 0) return 0;
  if (value >= SLIDER_MAX) return 0.99;
  return Math.log(value + 1) / Math.log(SLIDER_MAX + 1);
}

function positionToValue(position: number, isMax: boolean): number {
  if (isMax && position >= 0.99) return 0; // no-limit sentinel
  if (position <= 0) return 0;
  const raw = Math.exp(position * Math.log(SLIDER_MAX + 1)) - 1;
  if (raw < 10) return Math.round(raw * 10) / 10;
  if (raw < 100) return Math.round(raw);
  return Math.round(raw / 5) * 5;
}

function formatValue(value: number, isMax: boolean): string {
  if (isMax && value === 0) return "∞";
  if (value === 0) return "0";
  return value % 1 === 0 ? String(value) : value.toFixed(1);
}

// ── Component ─────────────────────────────────────────────────────────────────

export interface RangeSliderProps {
  /** Stored min_size value (MB/min). 0 = no minimum. */
  minValue: number;
  /** Stored max_size value (MB/min). 0 = no limit. */
  maxValue: number;
  /** Stored preferred_size value (MB/min). 0 = same as max. */
  preferredValue: number;
  onChange: (min: number, max: number, preferred: number) => void;
}

const THUMB_SIZE = 14; // px — min/max circle diameter
const PREF_SIZE = 10;  // px — preferred diamond size (before rotation)
const TRACK_HEIGHT = 4; // px

export function RangeSlider({ minValue, maxValue, preferredValue, onChange }: RangeSliderProps) {
  const trackRef = useRef<HTMLDivElement>(null);
  const [dragging, setDragging] = useState<"min" | "max" | "pref" | null>(null);
  const [hoveredThumb, setHoveredThumb] = useState<"min" | "max" | "pref" | null>(null);

  // Effective max cap for calculations
  const maxCap = maxValue === 0 ? SLIDER_MAX : maxValue;

  const minPos  = valueToPosition(minValue);
  const maxPos  = maxValue === 0 ? 1.0 : valueToPosition(maxValue);

  // Preferred: if 0, treat same as max (sits at maxPos)
  const prefEffective = preferredValue === 0 ? maxCap : Math.min(preferredValue, maxCap);
  const prefPosRaw    = valueToPosition(prefEffective);
  const prefPos       = Math.max(minPos, Math.min(prefPosRaw, maxPos));

  function posFromEvent(e: React.PointerEvent): number {
    const track = trackRef.current;
    if (!track) return 0;
    const rect = track.getBoundingClientRect();
    return Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
  }

  function handleTrackClick(e: React.MouseEvent) {
    if (dragging) return; // ignore click after drag
    const track = trackRef.current;
    if (!track) return;
    const rect = track.getBoundingClientRect();
    const pos = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
    // Move the nearest of three thumbs
    const dMin  = Math.abs(pos - minPos);
    const dMax  = Math.abs(pos - maxPos);
    const dPref = Math.abs(pos - prefPos);
    const nearest = Math.min(dMin, dMax, dPref);
    if (nearest === dMin) {
      onChange(positionToValue(Math.min(pos, maxPos), false), maxValue, preferredValue);
    } else if (nearest === dMax) {
      const newMax = positionToValue(Math.max(pos, minPos), true);
      // Snap preferred if it falls outside new [min, max]
      const newPref = preferredValue > 0 && preferredValue > (newMax || SLIDER_MAX)
        ? newMax
        : preferredValue;
      onChange(minValue, newMax, newPref);
    } else {
      const clampedPos = Math.max(minPos, Math.min(pos, maxPos));
      onChange(minValue, maxValue, positionToValue(clampedPos, false));
    }
  }

  // ── Min thumb ──
  function handleMinPointerDown(e: React.PointerEvent) {
    e.preventDefault();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    setDragging("min");
  }
  function handleMinPointerMove(e: React.PointerEvent) {
    if (dragging !== "min") return;
    const pos = posFromEvent(e);
    const clampedPos = Math.min(pos, prefPos - 0.01, maxPos - 0.02);
    onChange(positionToValue(clampedPos, false), maxValue, preferredValue);
  }

  // ── Max thumb ──
  function handleMaxPointerDown(e: React.PointerEvent) {
    e.preventDefault();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    setDragging("max");
  }
  function handleMaxPointerMove(e: React.PointerEvent) {
    if (dragging !== "max") return;
    const pos = posFromEvent(e);
    const clampedPos = Math.max(pos, prefPos + 0.01, minPos + 0.02);
    const newMax = positionToValue(clampedPos, true);
    // Snap preferred if it now exceeds new max
    const newPrefEffective = preferredValue === 0 ? maxCap : preferredValue;
    const newMaxCap = newMax === 0 ? SLIDER_MAX : newMax;
    const newPref = newPrefEffective > newMaxCap ? newMax : preferredValue;
    onChange(minValue, newMax, newPref);
  }

  // ── Preferred thumb ──
  function handlePrefPointerDown(e: React.PointerEvent) {
    e.preventDefault();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    setDragging("pref");
  }
  function handlePrefPointerMove(e: React.PointerEvent) {
    if (dragging !== "pref") return;
    const pos = posFromEvent(e);
    const clampedPos = Math.max(minPos + 0.01, Math.min(pos, maxPos - 0.01));
    onChange(minValue, maxValue, positionToValue(clampedPos, false));
  }

  function handlePointerUp(e: React.PointerEvent) {
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
    setDragging(null);
  }

  // ── Labels ──

  const minLabel  = formatValue(minValue, false);
  const maxLabel  = formatValue(maxValue, true);
  const prefLabel = preferredValue === 0 ? formatValue(maxValue, true) : formatValue(preferredValue, false);

  // Suppress preferred label if it's too close to min or max label (< 8% gap)
  const showPrefLabel = Math.abs(prefPos - minPos) > 0.08 && Math.abs(prefPos - maxPos) > 0.08;
  // Suppress max label if too close to min (< 8% gap)
  const showMaxLabel = maxPos - minPos > 0.08;

  function labelLeft(pos: number): string {
    const pct = pos * 100;
    if (pct < 4) return "0%";
    if (pct > 96) return "auto";
    return `${pct}%`;
  }
  function labelRight(pos: number): string {
    return pos * 100 > 96 ? "0%" : "auto";
  }
  function labelTransform(pos: number): string {
    const pct = pos * 100;
    if (pct < 4 || pct > 96) return "none";
    return "translateX(-50%)";
  }

  // ── Thumb style factories ──

  function circleThumbStyle(which: "min" | "max", pos: number): React.CSSProperties {
    const isHovered = hoveredThumb === which;
    const isDragging = dragging === which;
    return {
      position: "absolute",
      left: `calc(${pos * 100}% - ${THUMB_SIZE / 2}px)`,
      top: `calc(50% - ${THUMB_SIZE / 2}px)`,
      width: THUMB_SIZE,
      height: THUMB_SIZE,
      borderRadius: "50%",
      background: "var(--color-accent)",
      border: "2px solid var(--color-bg-surface)",
      cursor: isDragging ? "grabbing" : "grab",
      transform: isHovered || isDragging ? "scale(1.3)" : "scale(1)",
      boxShadow: isHovered || isDragging
        ? "0 0 0 3px color-mix(in srgb, var(--color-accent) 25%, transparent)"
        : "none",
      transition: isDragging ? "none" : "transform 0.1s, box-shadow 0.1s",
      touchAction: "none",
      zIndex: isDragging ? 4 : 2,
    };
  }

  function prefThumbStyle(): React.CSSProperties {
    const isHovered = hoveredThumb === "pref";
    const isDragging = dragging === "pref";
    return {
      position: "absolute",
      left: `calc(${prefPos * 100}% - ${PREF_SIZE / 2}px)`,
      top: `calc(50% - ${PREF_SIZE / 2}px)`,
      width: PREF_SIZE,
      height: PREF_SIZE,
      borderRadius: 2,
      background: "var(--color-info)",
      border: "2px solid var(--color-bg-surface)",
      cursor: isDragging ? "grabbing" : "grab",
      transform: isHovered || isDragging ? "rotate(45deg) scale(1.3)" : "rotate(45deg) scale(1)",
      boxShadow: isHovered || isDragging
        ? "0 0 0 3px color-mix(in srgb, var(--color-info) 25%, transparent)"
        : "none",
      transition: isDragging ? "none" : "transform 0.1s, box-shadow 0.1s",
      touchAction: "none",
      zIndex: isDragging ? 4 : 3,
    };
  }

  return (
    <div style={{ paddingBottom: 20, userSelect: "none" }}>
      {/* Track container */}
      <div
        ref={trackRef}
        onClick={handleTrackClick}
        style={{
          position: "relative",
          height: TRACK_HEIGHT,
          background: "var(--color-border-default)",
          borderRadius: TRACK_HEIGHT / 2,
          marginTop: THUMB_SIZE / 2 + 2,
          cursor: "pointer",
        }}
      >
        {/* Filled range between min and max thumbs */}
        <div
          style={{
            position: "absolute",
            left: `${minPos * 100}%`,
            right: `${(1 - maxPos) * 100}%`,
            top: 0,
            bottom: 0,
            background: "color-mix(in srgb, var(--color-accent) 35%, transparent)",
            borderRadius: TRACK_HEIGHT / 2,
            pointerEvents: "none",
          }}
        />

        {/* Preferred fill: min → preferred (slightly brighter) */}
        <div
          style={{
            position: "absolute",
            left: `${minPos * 100}%`,
            right: `${(1 - prefPos) * 100}%`,
            top: 0,
            bottom: 0,
            background: "var(--color-accent)",
            borderRadius: TRACK_HEIGHT / 2,
            pointerEvents: "none",
          }}
        />

        {/* Min thumb */}
        <div
          style={circleThumbStyle("min", minPos)}
          onPointerDown={handleMinPointerDown}
          onPointerMove={handleMinPointerMove}
          onPointerUp={handlePointerUp}
          onPointerEnter={() => setHoveredThumb("min")}
          onPointerLeave={() => { if (dragging !== "min") setHoveredThumb(null); }}
        />

        {/* Preferred thumb (diamond) */}
        <div
          style={prefThumbStyle()}
          onPointerDown={handlePrefPointerDown}
          onPointerMove={handlePrefPointerMove}
          onPointerUp={handlePointerUp}
          onPointerEnter={() => setHoveredThumb("pref")}
          onPointerLeave={() => { if (dragging !== "pref") setHoveredThumb(null); }}
        />

        {/* Max thumb */}
        <div
          style={circleThumbStyle("max", maxPos)}
          onPointerDown={handleMaxPointerDown}
          onPointerMove={handleMaxPointerMove}
          onPointerUp={handlePointerUp}
          onPointerEnter={() => setHoveredThumb("max")}
          onPointerLeave={() => { if (dragging !== "max") setHoveredThumb(null); }}
        />
      </div>

      {/* Value labels */}
      <div style={{ position: "relative", height: 18, marginTop: 4 }}>
        {/* Min label */}
        <span
          style={{
            position: "absolute",
            left: labelLeft(minPos),
            right: labelRight(minPos),
            transform: labelTransform(minPos),
            fontSize: 11,
            fontFamily: "var(--font-family-mono)",
            color: "var(--color-text-secondary)",
            whiteSpace: "nowrap",
          }}
        >
          {minLabel}
        </span>

        {/* Preferred label */}
        {showPrefLabel && (
          <span
            style={{
              position: "absolute",
              left: labelLeft(prefPos),
              right: labelRight(prefPos),
              transform: labelTransform(prefPos),
              fontSize: 11,
              fontFamily: "var(--font-family-mono)",
              color: "var(--color-info)",
              whiteSpace: "nowrap",
            }}
          >
            {prefLabel}
          </span>
        )}

        {/* Max label */}
        {showMaxLabel && (
          <span
            style={{
              position: "absolute",
              left: labelLeft(maxPos),
              right: labelRight(maxPos),
              transform: labelTransform(maxPos),
              fontSize: 11,
              fontFamily: "var(--font-family-mono)",
              color: "var(--color-text-secondary)",
              whiteSpace: "nowrap",
            }}
          >
            {maxLabel}
          </span>
        )}

        {/* Unit label */}
        <span
          style={{
            position: "absolute",
            right: 0,
            fontSize: 10,
            color: "var(--color-text-muted)",
            letterSpacing: "0.02em",
            lineHeight: "18px",
          }}
        >
          MB/min
        </span>
      </div>
    </div>
  );
}
