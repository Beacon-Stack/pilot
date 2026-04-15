# web-shared

Shared React/TypeScript components used by every Beacon service's frontend (Pilot, Prism, Haul, Pulse).

## Why this exists

Before this directory, each service either had a duplicated local copy of components like `Modal.tsx` and `ConfirmDialog.tsx` (Pilot/Prism kept drifting toward incidental divergence), or simply didn't have them at all (Haul and Pulse used raw `window.confirm()`, leaking the browser's ugly native dialog into the UX). The "dead torrent delete shows a browser prompt" bug was the symptom that surfaced this class.

Now there's one canonical source. If you fix a bug or tweak the styling in `Modal.tsx`, every service picks it up on its next frontend build. No manual sync, no drift.

## How it works

Each service's `vite.config.ts` adds an alias:

```ts
resolve: {
  alias: {
    "@": path.resolve(__dirname, "./src"),
    "@beacon-shared": path.resolve(__dirname, "../../../web-shared"),
  },
},
```

And each service's `tsconfig.app.json` mirrors it for type resolution:

```json
"paths": {
  "@": ["./src/*"],
  "@beacon-shared/*": ["../../../web-shared/*"]
}
```

Then any component in the service imports like:

```tsx
import Modal from "@beacon-shared/Modal";
import { useConfirm, ConfirmProvider } from "@beacon-shared/ConfirmDialog";
```

## Contents

- `Modal.tsx` — Generic modal shell (backdrop, Escape, click-outside, width/height controls).
- `ConfirmDialog.tsx` — `ConfirmProvider` + `useConfirm()` hook. Replaces `window.confirm()` everywhere.

## Rules for adding components

1. **Only add things that ≥3 services need** (or will need within the next few commits). This isn't a dumping ground for "it might be useful someday".
2. **No service-specific deps.** Pure React + TypeScript + CSS custom properties. No imports from `@/` paths.
3. **Props must be stable.** Changes here break every service simultaneously — treat it like a public API.
4. **Test before committing.** Run `npm run build` in at least one service that imports the component.
5. **Don't create parallel versions.** If you need a variant, add a prop or make two components, but never fork.

## Gotchas

- The alias targets a relative path (`../../../web-shared`). That assumes a service lives at `<service>/web/ui/`. If the directory layout moves, update the aliases.
- Because this isn't an npm package, there's no versioning. Breaking changes ship to every service at once. Make them rarely and intentionally.
- Vitest (used by Pilot's test suite) picks up the alias from `vite.config.ts` automatically. No extra test-config needed.
