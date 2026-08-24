# hex-music-bot — Command Reference

Auto-generated from `internal/commands/defs.go`. Do not edit manually.

| Command | Description |
|---------|-------------|
| `/play` | Play a song, playlist or search query |
| `/pause` | Pause the current song |
| `/resume` | Resume the paused song |
| `/skip` | Skip ahead in the queue |
| `/previous` | Play the previously finished song |
| `/stop` | Stop playback and clear the queue |
| `/leave` | Disconnect the bot from voice |
| `/join` | Join your current voice channel |
| `/queue` | Show the queued songs |
| `/now-playing` | Show the currently playing song |
| `/shuffle` | Shuffle the queue |
| `/loop` | Set the loop mode (off/track/queue) |
| `/clear` | Remove all queued songs |
| `/volume` | Set the player volume (0–1000) |
| `/seek` | Seek to a position in the current song |
| `/remove` | Remove one queued song by its position |
| `/search` | Search for a song and pick the right one (live autocomplete) |
| `/lyrics` | Show synced lyrics for the current song (LRCLIB); `mode: live` posts a scrolling window synced to playback |
| `/history` | Show recently played tracks |
| `/taste` | Manage your taste profile for autoplay |
| `/playlist` | Manage playlists |
| `/autoplay` | Toggle autoplay — keeps music going after queue drains |
| `/247` | Toggle 24/7 mode — survives restarts |
| `/request-channel` | Set this channel as the song-request channel |

## Card Buttons

| Button | Action |
|--------|--------|
| ⏮ | Previous track |
| ⏯ | Pause/resume toggle |
| ⏭ | Skip to next |
| 🔁 | Cycle loop mode (off → track → queue) |
| 🔀 | Shuffle queue |
| 🔉 / 🔊 | Volume down/up by 25 |
| ⏹ | Stop and lock card |

---

_Last generated: 2026-08-22_
