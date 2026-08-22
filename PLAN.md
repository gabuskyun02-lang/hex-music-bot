# hex-music-bot — Build Plan

Go + disgo + disgolink + Lavalink v4 music bot. Goal: match the combined feature set of the 8 reference bots in `../research/`, then beat them on the axes where they're weak.

Reference tags: `[B]` Beatra · `[LX]` Lunox · `[VC]` Vocard · `[MC]` MusicCat · `[BB]` ByteBlaze · `[PM]` PrimeMusic · `[LK]` Lucky · `[DP]` discord-player-bot

## Non-negotiables

1. **License hygiene.** Port freely from MIT/Apache/ISC (`beatra`, `lunox`, `vocard`, `primemusic`, `lucky`, `discord-player-bot` is GPL — ideas only). `musiccat` has **no license**: concepts yes, code never. `byteblaze` GPL-3.0: ideas only. hex-music-bot ships **MIT**.
2. **Single action path.** Slash commands AND buttons resolve to the same `player.Actions` methods. No duplicated handler logic (every reference bot duplicates; it's their #1 bug farm).
3. **SQLite, zero external services.** Embedded store (modernc.org/sqlite, pure Go). No Mongo/Redis/Postgres requirement for a self-host single binary. Optional Postgres later via store interface.
4. **Everything survives restart.** Player state snapshots persist; 24/7 mode rejoins and resumes queues on boot. No reference bot does this.

## Layout (Lunox decomposition, Go packaging)

```
cmd/hex-music-bot/main.go
internal/
  bot/        client wiring, event router, graceful shutdown
  config/     typed env+yaml config, validates ALL problems before exit
  commands/   one file per command (flat, kebab-case)
  events/     gateway + disgolink listeners, one file per event
  player/     guild manager, state machine, queue, actions (single dispatcher)
  ui/         player card, embeds, pagination, components builders
  store/      SQLite repos: settings, playlists, history, snapshots
  lyrics/     LRCLIB client (+ provider interface)
  autoplay/   related-track engine + content filtering
lavalink/application.yml
scripts/start-lavalink.ps1
migrations/*.sql (embedded)
```

---

## Phase 0 — Foundations `[size: S]`

- Module skeleton, `config` loader (`.env` + optional yaml), `slog` logging, embedded SQL migrations, CI (build/vet/test).
- `scripts/start-lavalink.ps1`: download latest jar if missing, run with bundled `application.yml` (sources on; plugins section pre-documented for LavaSrc/DuncteBot).
- ⬆ **Improvement:** config validation reports every problem at once with fix hints; references fail one-at-a-time or panic.

**Accept:** `go build ./...` green; bot starts, connects gateway + node, logs versions, exits cleanly on Ctrl-C.

## Phase 1 — Core playback parity `[size: M]`

Commands: `/play` `/pause` `/resume` `/skip [n]` `/previous` `/stop` `/queue` `/now-playing` `/shuffle` `/loop off|track|queue` `/volume 0–1000` `/seek <pos> [unit]` `/remove <idx>` (autocomplete) `/join` `/leave` `/clear`.

- `player.Manager`: map[guildID]*PlayerState, RWMutex; state machine `idle→joining→playing⇄paused→draining`.
- `/play` flow: URL vs query detection → search-type prefixing (multi-engine choices incl. ytsearch/ytmsearch/scsearch/dzsearch/spsearch) → playlist expansion (all tracks queued, first plays) → join-if-needed → update player.
- `/previous`: ring buffer of finished tracks `[LX]`.
- `/skip n` skips ahead n queued tracks atomically.
- Multi-source `/play --source` choice list `[MC][BB matrix]`.

**Accept:** in a test server: play URL/query/playlist, pause/resume, skip chain, loop modes, volume, seek, remove-by-autocomplete, previous replays. Queue survives concurrent /play (race-checked).

## Phase 2 — UX layer (the visible jump) `[size: L]`

- **Live player card** `[B][MC][PM]`: Components V2 container — cover art, title/author links, progress bar refreshed ~10s, source badge, queue-next line. Button deck: ⏮ ▶/⏸ ⏭ 🔁 🔀 🔊 ⏹. Card locks ("session ended") when idle.
- **Edit coalescing** ⬆: card updates batched through a rate-limit-aware editor (max ~2 edits/10s); progress ticks dropped when throttled. References hammer the API and get 429s.
- Buttons + commands → same `player.Actions` (non-negotiable #2).
- **Queue pagination** with buttons `[LX]`; **/search** autocomplete resolving live track previews, cached 5min `[MC]` ⬆ multi-engine, cache TTL'd.
- **Lyrics**: LRCLIB-first `[B improved]` — free API, *time-synced* lines. `/lyrics` paginates; ⬆ when card is active, highlight current line in an expandable section (none of the 8 do synced-in-card).

**Accept:** card appears on first play, buttons all work identically to commands, progress advances without 429s, lyrics sync visibly.

## Phase 3 — Resilience `[size: M]`

- Voice watchdog: stale WS detection → forced rejoin `[B]`.
- Node reconnect + **session resume** via persisted disgolink SessionID `[MC gap]` ⬆.
- Track failure ladder ⬆ (novel): TrackException/Stuck → mark track failed → try same query on alternate source once → skip + notice. Spotify/Deezer links resolve to a streamable source automatically.
- Auto-pause on empty VC, auto-resume on rejoin `[BB]`; idle auto-disconnect after configurable timeout `[LX]` (default 5m; disabled in 24/7 mode).
- Graceful shutdown: snapshot players → close gateway → close node.

**Accept:** kill Lavalink mid-play → bot recovers and resumes; empty VC pauses within seconds; broken track auto-falls-over instead of silencing.

## Phase 4 — Persistence & differentiators `[size: L]`

Schema: `guild_settings` (locale, default volume, DJ roles, request-channel id, 247 flag, autoplay flag+aggressiveness, leave-timeout) · `playlists` (owner-scoped, tracks JSON, share code) · `play_history` (guild, track, requester, ts) · `user_taste` (user id, preferred artists + weights) · `player_snapshots`.

- **Playlists** `[VC][MC]`: create/add/remove/list/load/play; share via short code; ⬆ import from YouTube/Spotify playlist URLs directly.
- **Autoplay engine** `[B crown jewel, improved]`: queue drains → pick seeds from last N history (artist rotation, not just last track) → resolve related via YouTube radio/Mix + optional Last.fm similar → content filter: duration window (default 90s–10min) + keyword blocklist (live, mix, podcast, tutorial, full album…) + already-played dedupe window. Per-guild toggle + aggressiveness (off/light/normal/aggressive).
- **Taste profiles** `[LK stolen, bot-native]`: `/taste add|remove|list` maintains per-user preferred artists; autoplay blends the taste of everyone currently in voice, weighted by listen share ("Choose artists to guide autoplay recommendations" — Lucky's dashboard concept, reimplemented as one query + one merge function).
- **Track history** `[LK]`: `/history [user]` paginated recent plays; doubles as autoplay seed source and "what was that song" lookup.
- **24/7 mode** `[BB, improved]`: persists across restarts — on boot, restore snapshots, rejoin channels, resume queues. Nobody in the reference set does this.
- **Song-request channel** `[BB, merged]` ⬆: dedicated channel where any posted link/query queues instantly (+✅ reaction); the live player card is permanently pinned there — one surface is both jukebox input and control panel.
- **DJ mode** `[B]`: optional role gate on skip/stop/volume/leave; bypass for requester's own queued track.
- **Filters** `[VC][BB]`: `/filter bassboost|nightcore|vaporwave|…` preset stacks (composable), per-guild default preset.
- Lightweight command cooldowns `[BB]`; `/stats` (tracks played, uptime, node health).

**Accept:** restart bot mid-queue with 24/7 on → same song region resumes; SR channel accepts a raw YouTube link with zero slash commands; autoplay fills naturally after playlist end without podcasts sneaking in.

## Phase 5 — Polish & scale-out `[size: M]`

- **i18n**: embedded JSON string packs (start: en + tr; structure copied conceptually from `vocard/langs`). All UI strings through the pack — enforced by lint check that no literal user-facing strings leak.
- **Metrics**: `/metrics` Prometheus endpoint (plays, errors, skips, node latency, card edits) `[LK spirit, 100x lighter]` + `/healthz`.
- Dockerfile multi-stage + `docker-compose.yml` (bot + lavalink) `[DP][MC pattern]`.
- **Generated command reference**: `docs/COMMANDS.md` rendered from command definitions at build time `[DP pattern]` ⬆ single source of truth.
- Sharding: design-compatible (disgo shard manager behind config flag); implement only when >2k guilds. YAGNI now.
- Dashboard: **out of scope v1, deliberately staged** `[LK]`. Every dashboard feature ships bot-native first: Media Player → live card + buttons (Phase 2); Track History → `/history`; Lyrics → `/lyrics` (Phase 2); Musical Taste → `/taste` + blended autoplay (Phase 4). Later, an embedded HTTP server inside the same binary can serve a local web UI over the same SQLite file — no Lucky-style 4-package split (`frontend` + `backend` + `shared` exist mostly to host these four screens, and their own source comments record the cost: a recurring "Musical-Taste Discover hang" bug class). Only remote-control playback with OAuth + realtime truly needs web infrastructure — revisit when wanted, not now.

**Accept:** `docker compose up` runs the whole stack; metrics scrape; docs regenerate in CI.

## Phase 6 — Web dashboard `[size: L, after v1]`

- **Single binary stays single**: `go:embed` static assets served by `internal/httpapi` on a configurable port. No Node build chain, no second deployable — the anti-Lucky constraint holds even here.
- **Auth**: Discord OAuth2 login; guild list scoped to servers where the user has Manage Server. Session cookies, CSRF-checked state mutations.
- **Screens**, each backed by an existing internal service (that's why the phase is last — its API surface already exists):
  - Remote player control — WebSocket mirror of `player.Actions` (play/pause/skip/volume/seek) with live card state
  - Queue editor — drag-to-reorder, remove, jump
  - Playlists manager — richer than `/playlist` commands (bulk edit, reorder)
  - History + stats — charts over `play_history` and `/metrics` counters
  - Taste manager — visual preferred-artists editor feeding the same `user_taste` table as `/taste`
  - Guild settings — DJ roles, 24/7, autoplay aggressiveness, filters, request channel
- **Stack**: server-rendered Go templates + htmx first; SPA only if a screen truly needs it. Boring, debuggable, zero toolchain.

**Accept:** log in via Discord, see only your manageable servers, skip a track from your phone while the bot plays in VC, edit taste visually, all state identical to what commands see (same store, same Actions).

---

## Explicit "make it better" ledger

| Axis | Best reference | Our edge |
|---|---|---|
| Restart survival | none | persisted snapshots, boot-resume |
| Failure handling | silence/skip | alternate-source failover ladder |
| Lyrics | Genius scraping `[B]` | LRCLIB synced, shown live on card |
| Autoplay | genre heuristic `[B]` | history-seeded rotation + tunable aggressiveness |
| Personalization | dashboard-only `[LK]` | bot-native `/taste`, blended across VC listeners |
| Control surfaces | separate card/SR-channel | merged pinned card = jukebox |
| API etiquette | frequent 429s observed | coalesced card editor |
| Deployment deps | Mongo/Redis/Prisma | one binary + one jar |
| Config UX | partial docs | validated, all-errors-at-once |
| Code paths | command/button duplication | single Actions dispatcher |

## Sequencing rule
Phases are dependency-ordered; inside a phase, order = riskiest integration first (voice/node glue before polish). Each phase ends with a tagged build that runs against a real server — no phase merges on unit tests alone.
