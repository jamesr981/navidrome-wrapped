# Navidrome Wrapped

A year-in-music recap for every user on your Navidrome server, in the style of Spotify Wrapped. The stats come from Navidrome's own play history, so nothing leaves your server.

For each user the plugin produces:

- **A "Wrapped 2026 · Top Songs" playlist.** It's private and holds their 50 most played tracks. The playlist description gives a summary (minutes, top artist, top song, top genre, listening personality), so it shows up in the Navidrome web UI and in every Subsonic client.
- **An animated story page.** This is a tap-through, phone-sized story with these slides: minutes listened, first song of the year, top genres, top artist, top 5 artists, song of the year, top 5 songs, top album, the "on repeat" day, listening clock, biggest month, streaks, listening personality, and a summary card. Album and artist art is embedded, so each page is a single file that works offline.
- **A JSON file** with the raw stats, plus an `index.html` linking everyone's story.

## Requirements

- Navidrome **0.64 or newer**. The plugin uses the scrobble retriever API, which arrived in 0.64.
- Plugins enabled in `navidrome.toml`:

  ```toml
  [Plugins]
  Enabled = true
  ```

## Install

1. Copy `navidrome-wrapped.ndp` into your plugins folder. By default that's `<DataFolder>/plugins`.
2. In the web UI, go to **Settings → Plugins → Navidrome Wrapped**.
3. Under **Users**, choose "Allow all users" or pick specific people. The plugin only wraps the users you allow, and it can't be enabled until you choose.
4. Set your **Timezone** (e.g. `Australia/Melbourne`). It decides year boundaries, the listening clock and daily streaks.
5. Enable the plugin.

## Generating Wrapped

- **Automatically:** by default it runs at 06:00 on 1 December (cron `0 6 1 12 *`). Change or clear this under *Schedule*.
- **Right now:** turn on **Generate now** and save. Saving reloads the plugin, and generation starts about 5 seconds later. Turn it off again afterwards, or it will regenerate every time the server restarts.

Re-running is safe. The previous Wrapped playlist for that year is replaced rather than duplicated. When *Year* is `0` (the default), the plugin wraps the current year, or last year if it runs in January.

Progress and results appear in the Navidrome log (search for "Wrapped"). A per-user summary is written to `last-run.json`.

## Where the story pages go

Navidrome plugins can't serve web pages, so the stories are written to the plugin's storage folder:

```
<DataFolder>/plugins/navidrome-wrapped/storage/<year>/
    index.html        ← links to everyone
    <username>.html   ← the story
    <username>.json   ← raw stats
    last-run.json
```

Each `.html` is fully self-contained. You can send it to the person, or open it straight from disk. To give everyone a link, serve that folder with any static web server. For example, a Caddy block alongside a Docker Compose setup:

```
wrapped.example.com {
    root * /navidrome-data/plugins/navidrome-wrapped/storage
    file_server
    basicauth { ... }   # the pages contain listening history — keep them private
}
```

## Settings

| Setting | Default | What it does |
|---|---|---|
| Year | `0` (auto) | Year to wrap |
| Timezone | `UTC` | IANA timezone for day/hour calculations |
| Schedule | `0 6 1 12 *` | Cron for automatic runs; empty disables |
| Generate now | off | Run shortly after the plugin is loaded or its settings are saved |
| Minimum plays | `25` | Skip users who barely listened |
| Create Top Songs playlist | on | Create/replace the playlist |
| Playlist size | `50` | Tracks in the playlist |
| Embed artwork | on | Put cover and artist images inside the story pages |

## How the stats work

- **Plays** are Navidrome scrobbles for the year. Tracks deleted from the library since then are left out.
- **Minutes** add up the track length for every play. Navidrome records a play once a track has mostly been heard, so this is a close estimate.
- **Top artists** credit every artist on a track. A song with a featured artist counts for both.
- **Listening personality** is worked out from four signals:
  - *Loyal vs Explorer:* does your top artist account for at least 12% of plays?
  - *Trendsetter vs Timeless:* do at least 40% of plays come from this year's or last year's releases?
  - *Repeater vs Variety seeker:* are distinct songs no more than 35% of plays?
  - *Night owl vs Daylight listener:* is your peak hour between 9 PM and 5 AM?

  The combination picks one of eight archetypes.

## Building from source

You need [TinyGo](https://tinygo.org) 0.44+ and Go 1.27+.

```sh
make          # fetches the Navidrome PDK (pinned to v0.64.2), builds plugin.wasm, packages navidrome-wrapped.ndp
make test     # unit tests for the stats engine
```

To build against a different Navidrome release, set `NAVIDROME_REF`. For example, `make NAVIDROME_REF=v0.65.0`.

### Versioning

The plugin version shown in Navidrome comes from git tags. Building a commit tagged `v1.2.3` produces version `1.2.3`. Any other commit builds as `0.0.0-dev+<commit>`. To set it by hand, use `make VERSION=1.2.3`. To release, create a GitHub release with a new `v1.2.3` tag. Publishing it triggers CI, which builds the versioned `.ndp` and attaches it to the release.

### Layout

| File | Purpose |
|---|---|
| `main.go` | Config, lifecycle/scheduler hooks, per-user orchestration |
| `collect.go` | Scrobble paging, track lookup, cover art, playlist via the Subsonic API |
| `stats.go` | Pure stats aggregation and the personality logic (unit tested) |
| `render.go` | Injects stats JSON into the HTML templates |
| `story.html`, `index.html` | The story player and the index page (no external assets) |
| `manifest.json` | Plugin metadata, permissions, settings schema |
