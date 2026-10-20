# Changelog

All notable changes to this project are documented in this file.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.3.0] - 2026-10-20

### Added

- Sticky **bus master** strip (toggle **Master on/off**): fader, pan, mute, and meter for the selected mix bus; channels scroll beside it
- Mixer bar **VU on/off**: optional slow meter ballistics (persisted in `localStorage`)
- Per-channel peak-hold tick on meters (~1.2 s hold, then falls to the live level)
- API: `POST /api/bus-level`, `/api/bus-pan`, `/api/bus-mute`; live `/bus/XX/mix/*` via `/xremote`

### Changed

- Fader scale matches Mixing Station / X32 (+10 … −60, pseudo-log positions); LED readout shows dB
- Channel meters use a dBFS bar (−60…0) with a colour gradient fixed to the track (yellow near 0, deep red at +10)
- Meters update ~50 ms with rAF smoothing (fast attack)
- Channel icons are white PNGs (no CSS invert — sharper on iOS)
- iPhone portrait: sharper mixer-bar text; button bar sits lower under the notch

## [0.2.0] - 2026-10-19

### Added

- Submix mixer for 32 channels (send level, mute, pan) onto a selected stereo bus
- Bus list from the desk (`/bus/XX/config/name`); 8 stereo buses (odd 1–15); re-read on every pick
- Live sync via `/xremote` (fader/pan/mute/name/icon/color) and channel meters (`/meters/0`)
- Independent browser sessions (`X-Session-ID` / `sid`); each client picks its own submix bus
- Last mix per session restored from `localStorage` on reopen
- Channel colors (`/ch/XX/config/color`) as strip background; fader height adapts to the screen
- Channel icons: full X32 set as PNGs (`/icons/1.png`–`74.png`, from behringer-icons BMPs)
- Flag `-mixer` / `MIXER` for a fixed mixer address (skips discovery)
- Optional `-domain` / `DOMAIN` on the self-signed certificate; `-https` / `-http` / `-tls-dir`
- HTTPS by default (`:8443`); HTTP redirects to HTTPS
- PWA: fullscreen, service worker, install icons (X + fader), iOS/Android install help
- Screen Wake Lock + keep-awake fallback; “Awake on/off” in the mixer bar
- Dagger cross-build (Linux/Windows/macOS, amd64/arm64/386/armv7) and GitHub Actions CI
- CI: tests on push/PR; cross-build + GitHub Release on `v*` tags (Dagger v0.21.10)
- README mixer screenshot (`docs/screenshot-mixer.jpg`); startup logs list LAN IP URLs
- App / module name **X32 Personal Mix** (`X32-personal-mix`)

### Changed

- Fullscreen console UI (CodePen-style metal faders, LED, meters, mute **M**); no connect dialog overlay
- Faders move only when grabbing the knob; OSC for fader/pan ~50 Hz with flush on release
- Pan: tap opens a full-width control; drag to set, release or tap outside closes
- Live dot: green with open WebSocket, red when the server is gone
- Phone landscape: side gutters on `.mixer-view` (WebKit ignores padding on the scrollable mixer)
- Documentation and UI copy in English

### Fixed

- Live fader updates: OSC replies no longer block `/xremote`; drag tracking instead of `activeElement`
- Stereo bus: accept updates from the linked even bus
- Blank channel icon keeps layout space without a white square
- Dagger module main type (`X32PersonalMix`) matches the module name for CI

### Removed

- Studio One layout and layout switcher
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

[Unreleased]: https://github.com/bbuehrig/X32-personal-mix/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/bbuehrig/X32-personal-mix/releases/tag/v0.3.0
[0.2.0]: https://github.com/bbuehrig/X32-personal-mix/releases/tag/v0.2.0
[0.1.0]: https://github.com/bbuehrig/X32-personal-mix/tree/6975fed
