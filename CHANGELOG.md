# Changelog

All notable changes to this project are documented in this file.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed

- Channel icons use the full X32 BMP artwork as PNGs (`/icons/1.png`–`74.png`) — the partial SVG set had “NO ICON” stubs for bass, guitars, vocals, drum kit, etc.

### Planned

- Simple browser mixer (faders, mute) over OSC
- `/xremote` for live updates from the mixer to all clients

### Added

- After connect: read all 16 bus names via `/bus/XX/config/name`
- UI list of the 8 stereo buses (odd numbers 1–15)
- Bus selection for a shared submix; X32 device info hidden afterward
- Submix mixer for 32 channels (send level + on/mute) onto the selected bus
- API: `POST /api/select-bus`, `/api/clear-bus`, `/api/level`, `/api/mute`
- Live sync via `/xremote` (fader/pan/mute/name/icon from desk and other clients)
- Pan control per channel (`POST /api/pan`)
- Channel icons (X32 icon index as emoji)
- Fullscreen mixer without a connect dialog on top
- Flag `-mixer` / env `MIXER` for a fixed mixer address (skips discovery)

### Changed

- Mixer UI in Mixing Station / DAW style: narrow channel strips, rectangular fader caps with scale marks, pan/mute on top, channel color only in the scribble strip

### Added

- Fader design inspired by [CSS Audio Fader](https://codepen.io/verybradthings/pen/ZeNEpO): metal cap, LED readout, VCA meter with dB markers; mute **M** only (no solo)
- Channel color as background of the whole fader column (not only scribble strip)
- Live channel levels from the X32 (`/meters/0`) in the meter track beside each fader
- Bus names are re-read from the X32 when opening the bus picker
- App icons / favicon including Apple Touch Icon and web manifest (home screen)
- PWA fullscreen (`display: fullscreen`), service worker, Screen Wake Lock + keep-awake fallback for long-running use
- HTTPS on by default (`:8443`, self-signed in `.x32-personal-mix-tls/`); HTTP always redirects to HTTPS
- “Awake on/off” button in the mixer bar
- PWA install help per iOS / Android (hint + “?” dialog)
- App renamed to **X32 Personal Mix**
- Optional `-domain` / `DOMAIN` (e.g. `mixer.tcy`): DNS name on the self-signed certificate; regenerate when the name is missing
- PWA icon: large **X** plus fader strip with LED meter

### Changed

- Faders only move when grabbing the knob (no jump when tapping the track)
- Fader/pan OSC more often (~50 Hz + flush on release) for smoother desk motion
- Channel icon when “blank”: layout space kept, content transparent instead of a white square
- Pan: tap/click opens a full-width pan control; drag to set, release (or tap outside) closes
- Live dot: green only with an open WebSocket; red when the server is gone
- Mixer: side gutter on `.mixer-view` (JS, phone landscape min. 56px) — padding on scrollable `.mixer` is ignored by WebKit
- Dagger pipeline: cross-build Linux/Windows/macOS (amd64/arm64/386/armv7) + GitHub Actions CI
- Repository/module renamed: `X32-mix` → `X32-personal-mix`
- Documentation and UI copy translated to English
- Startup logs list LAN IP URLs in addition to localhost
- Fix Dagger module main type (`X32PersonalMix`) to match module name for CI
- Upgrade Dagger to v0.21.10; CI runs tests on push/PR, cross-build + GitHub Release only on `v*` tags
- README: example mixer screenshot (`docs/screenshot-mixer.jpg`)

### Removed

- Studio One layout and layout switcher (console layout only)

### Added

- Channel color (`/ch/XX/config/color`) for the scribble strip (`colorCss`)
- Fader height adapts to available screen height
- Independent browser sessions (`X-Session-ID` / `sid`): each client picks its own submix bus
- Last mix per session is stored and reopened on reconnect

### Fixed

- Live fader updates: OSC replies no longer block `/xremote`; drag tracking instead of `activeElement` (especially channel 1 / last touched fader)
- Stereo bus: also accept updates from the linked even bus

### Removed

- Debug mode (`-debug` / `DEBUG`) and manual IP entry in the UI
- `POST /api/connect`

## [0.1.0] - 2026-10-19

### Added

- Go HTTP server with embedded mobile web UI
- OSC UDP client including encode/decode (minimal OSC 1.0)
- Automatic X32/M32 discovery via `/xinfo` broadcast (port 10023)
- Picks the first found mixer when several reply
- Shows name, model, IP, and firmware version
- WebSocket hub for shared status to multiple clients
- REST: `GET /api/status`, `POST /api/discover`
- Configurable listen address via `ADDR`
