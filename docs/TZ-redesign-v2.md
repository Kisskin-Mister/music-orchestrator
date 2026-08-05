# TZ v2: REAL Visual Redesign — React Must Look Like Flutter

## CRITICAL CONTEXT
Previous rounds of "fixes" only renamed CSS classes (rounded-xl → rounded-control) which produce IDENTICAL visual output.
This TZ demands ACTUAL visual changes that are immediately visible when comparing screenshots.
Every task specifies EXACT CSS values — if the current code already has these values, it's NOT a change.

SoundCloud works — DO NOT touch backend code. yt-dlp is updated (2026.07.04).

---

## TASK 1: Page Headers — Remove border-bottom, match Flutter exactly

### Current React (SearchPage.tsx SectionHeader):
```tsx
<header className="mb-8 border-b border-white/8 pb-6">
```
This has a **border-bottom** and **pb-6** padding. Flutter has NO border and NO bottom padding.

### Required change:
```tsx
<header className="mb-8">
```
Remove: `border-b border-white/8 pb-6`

Also ensure title is exactly `text-[40px]` (not `text-[2.6rem]` which is 41.6px).
Eyebrow must be `font-mono text-[11px] uppercase tracking-[1.4px]` with accent color.

---

## TASK 2: Track Rows — Remove separator lines, fix spacing

### Current React:
Track rows have `border-b border-white/5` dividers between them. Flutter has NO dividers.

### Required changes:
1. Find ALL `border-b border-white/5` or similar separator lines between track rows and REMOVE them
2. Track art must be exactly 52×52px (verify, not 48 or 44)
3. Gap between art and text must be exactly 12px (not 10px or 8px)
4. Current track highlight: accent color at 8% opacity background, rounded-2xl (NOT a left border or underline)
5. Remove any `<hr>` or `border-t` between tracks

### How to verify:
Search for tracks → there should be NO visible lines between rows. Current track should have a subtle colored background pill, not a border accent.

---

## TASK 3: Mini Player — Match Flutter surface2, shadow, art size

### Flutter values (EXACT):
- Background: `#151923` (surface-2)
- Border-radius: 16px
- Shadow: `0 8px 24px rgba(0,0,0,.45)`
- Art size: 44×44
- Padding: 8px
- Margin: 12px left/right, 8px bottom
- Title: 13px, font-weight 600
- Artist: 12px, muted color

### Required changes:
Verify the mini player matches these EXACT values. If any value differs, fix it.

---

## TASK 4: Full Player — Reorder to match Flutter NowPlayingSheet

### Flutter layout order (top to bottom):
1. Drag handle (44×5 rounded bar, white/24, centered)
2. Square album art (1:1 aspect, radius 16, full width - padding)
3. Title (headlineMedium: Unbounded w700) + Artist (bodyMedium: muted)
4. Progress bar (thin, accent-colored track, rounded)
5. Time labels (current / total, mono font, muted, 12px)
6. Action buttons row (like, download, shuffle, repeat — 48px circular circles)
7. Transport row (prev, play/pause, next — play/pause is 64px accent circle)
8. Queue section

### Required changes:
Reorder the full player components to match this exact sequence. The play/pause button MUST be a 64px accent-colored circle.

---

## TASK 5: Version Display

### In Settings page, at the very bottom:
```tsx
<div className="mt-8 flex items-center justify-center gap-2 text-xs text-[#626875]">
  <span className="inline-block h-4 w-4 rounded bg-lime-300/20 text-center text-[10px] font-bold leading-4 text-lime-300">O</span>
  <span>Music Orchestrator v{__APP_VERSION__}</span>
</div>
```
This was already added. Verify it's there.

---

## TASK 6: Bump version to 0.3.0

Update `frontend/frontend/package.json` version to `"0.3.0"`.

---

## CONSTRAINTS
1. Do NOT touch backend Go code — SoundCloud already works
2. Do NOT touch auth.go, store.go, .env, deployment files
3. Only modify: frontend/frontend/src/ and frontend/frontend/public/
4. Tests must pass: `cd frontend/frontend && npm test -- --run`
5. Build must pass: `cd frontend/frontend && npm run build`
6. Go tests must pass: `go test ./...`
7. Commit with message: "feat(frontend): real visual parity with Flutter v0.3.0"

## VERIFICATION
After changes, verify EACH of these visually:
- [ ] No border-bottom on page headers
- [ ] No separator lines between track rows
- [ ] Mini player has surface2 bg, 16px radius, shadow
- [ ] Full player: handle → art → title → progress → actions → transport
- [ ] Version 0.3.0 visible in settings
- [ ] `npm test -- --run` passes
- [ ] `npm run build` passes
