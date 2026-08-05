# TZ: Diagnostic & Fix — SoundCloud, Design Parity, Flutter Rebuild

## CRITICAL CONTEXT
The user has deployed multiple rounds of "fixes" but NOTHING visually changed.
Previous commits changed CSS variable names (rounded-xl → rounded-control) which produce IDENTICAL output.
The user needs REAL visual changes that are obvious at a glance.

---

## BUG 1: SoundCloud Search Broken (PRIORITY: HIGH)

### Symptom
SoundCloud search returns 404 from yt-dlp:
```
WARN parallel provider search failed provider=soundcloud_stream 
error="yt-dlp failed: exit status 1: ERROR: [soundcloud] Unable to download JSON metadata: HTTP Error 404: Not Found"
```

### Root cause
yt-dlp version `2026.03.17` (installed via apt) is 5+ months old.
SoundCloud API changed — old yt-dlp can't scrape it.

### Fix
1. Update yt-dlp to latest:
   ```bash
   sudo pip3 install -U yt-dlp
   # or if pip not available:
   sudo wget https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -O /usr/local/bin/yt-dlp
   sudo chmod a+rx /usr/local/bin/yt-dlp
   ```
2. Verify: `yt-dlp --dump-json 'scsearch1:Drake' 2>/dev/null | head -5`
3. Restart backend: `systemctl --user restart music-orchestrator-backend-test`
4. Test: `curl -s 'http://localhost:18080/v1/search?q=Drake&provider_ids=soundcloud_stream&limit=3'`

---

## BUG 2: React Desktop Does NOT Match Flutter (PRIORITY: HIGH)

### Problem
Previous Claude Code commits only renamed CSS classes (rounded-xl → rounded-control, added CSS variables).
The ACTUAL visual design is completely different from Flutter. Renaming classes doesn't change appearance.

### What "match Flutter" means — CONCRETE visual changes:

#### 2a. Page headers
Flutter pattern (every screen):
```dart
Text('КОЛЛЕКЦИЯ', style: labelSmall.copyWith(color: accent))  // mono, 11px, letter-spacing 1.4, uppercase, accent color
SizedBox(height: 6)
Text('Медиатека', style: headlineLarge.copyWith(fontSize: 40))  // Unbounded, w800, 40px, letter-spacing -0.5
SizedBox(height: 8)  
Text('Всё, что ты лайкнул или скачал, — в одном месте.')  // 15px, muted
```
React currently: has eyebrow+title but wrong sizes, wrong spacing, border-bottom rule that Flutter doesn't have.
**FIX**: Remove border-bottom from SectionHeader. Make title exactly 40px. Eyebrow exactly 11px mono uppercase with 1.4 letter-spacing.

#### 2b. Track rows
Flutter: 52×52 art, radius 10, gap 12px, current track = accent@8% bg, NO separator lines between rows.
React currently: has separator lines, wrong art size, different spacing.
**FIX**: Remove all `border-b border-white/5` between track rows. Set art to exactly 52×52. Current track highlight: `bg-[color:var(--accent)]/[0.08]`.

#### 2c. Mini player
Flutter: surface2 bg (#151923), radius 16, shadow `0 8px 24px rgba(0,0,0,.45)`, 44×44 art, margin 12px sides 8px bottom.
React currently: different bg, different radius, different shadow.
**FIX**: Match exactly.

#### 2d. Full player (Now Playing)
Flutter order: drag handle → 1:1 art (radius 16) → title/artist → progress bar → time labels → action buttons (circle) → transport (64px accent play button) → queue.
**FIX**: Reorder React full player to match this exact layout.

#### 2e. Settings
Flutter: sections separated by Divider (40px height), accent swatches are 44px circles with 2px ring on selected.
**FIX**: Match section dividers and swatch sizes.

#### 2f. Search input  
Flutter: fill surface, border borderStrong, radius 10, hint "Название, исполнитель или альбом"
**FIX**: Match input style exactly.

#### 2g. Cover strip ("Слушай снова")
Flutter: 140×140 art cards, radius 16, 14px gap, horizontal scroll.
**FIX**: Match card size and spacing.

### IMPORTANT RULE
Do NOT just rename CSS classes. Make ACTUAL visual changes that are visible at a glance.
If you change `rounded-xl` to `rounded-control` and both are 12px, that's NOT a fix.
The changes must be VISIBLE when comparing screenshots of React vs Flutter.

---

## BUG 3: Flutter Web Container Not Rebuilt (PRIORITY: MEDIUM)

### Problem
Flutter web container `music-flutter-web` was built on Aug 4. Latest Flutter commits are from Jul 23.
The container IS up to date with Flutter code, BUT the user says "на флатере не видно версии" (no version on Flutter).

### Fix
1. Rebuild Flutter web container to pick up any code changes:
   ```bash
   cd /home/kisskin/music-orchestrator
   docker build -f mobile/Dockerfile.flutter -t music-flutter-web:latest mobile/ 
   # OR if no Dockerfile exists, rebuild using the existing build method
   docker stop music-flutter-web && docker rm music-flutter-web
   docker run -d --name music-flutter-web --restart unless-stopped \
     -p 172.22.0.1:5174:80 music-flutter-web:latest
   ```

2. Add version display to Flutter settings screen:
   - In `mobile/lib/screens/settings_screen.dart`, add at the bottom:
   ```dart
   Center(
     child: Padding(
       padding: const EdgeInsets.symmetric(vertical: 24),
       child: Text(
         'Music Orchestrator v0.2.0',
         style: TextStyle(color: AppColors.subtle, fontSize: 12),
       ),
     ),
   )
   ```

---

## BUG 4: Verify Previous Bugfixes Actually Work (PRIORITY: HIGH)

The 8 bugfixes from commit `4e6af83` may not be working. Verify EACH one:

1. **Search race condition**: Search "Drake", then immediately search "Beatles" — should show Beatles results, not Drake
2. **YouTube duration**: Play a YouTube track — duration should be correct (not 2x inflated)
3. **iOS artwork**: On iOS Safari — artwork should appear in lock screen/widget
4. **Playlist add track**: Open a playlist → "Добавить трек" button should be visible
5. **Nav bar sticky**: Desktop sidebar should scroll with the page (sticky, not fixed)
6. **SoundCloud**: Search should return SoundCloud results (after yt-dlp update)
7. **Parallel search**: Search should be fast (both providers search simultaneously)
8. **Infinite scroll**: Scroll down in search results → should auto-load more

For each one that doesn't work, diagnose and fix.

---

## CONSTRAINTS

1. Do NOT touch: auth.go, store.go, .env, deployment service files
2. You MAY modify: frontend/frontend/src/*, frontend/frontend/public/*, mobile/lib/*
3. You MAY run: sudo pip3/sudo wget for yt-dlp update
4. You MAY rebuild Docker container for Flutter
5. All tests must pass after changes
6. Commit with descriptive messages
7. Bump version to 0.3.0 after all fixes

---

## VERIFICATION CHECKLIST

After ALL fixes:
- [ ] `yt-dlp --dump-json 'scsearch1:Drake'` returns valid JSON
- [ ] `curl -s 'http://localhost:18080/v1/search?q=Drake&provider_ids=soundcloud_stream&limit=3'` returns SoundCloud results
- [ ] React page headers: eyebrow 11px mono uppercase accent, title 40px display, no border-bottom
- [ ] React track rows: 52×52 art, no separator lines, accent-8% current highlight
- [ ] React mini player: surface2, radius 16, shadow, 44×44 art
- [ ] React full player: handle → art → title → progress → actions → transport
- [ ] React settings: version v0.3.0 visible at bottom
- [ ] Flutter settings: version visible
- [ ] `go test ./...` passes
- [ ] `npm test -- --run` passes
- [ ] `npm run build` passes
- [ ] Git committed and pushed
