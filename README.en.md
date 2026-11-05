<p align="center">
  <img src="assets/logo.svg" alt="SacraLink" width="120"/>
</p>

<h1 align="center">SacraLink</h1>
<p align="center"><b>Wireless Church Broadcasting System</b><br/>
Turn up to 8 Android/iPhone smartphones into wireless cameras for OBS Studio —
no VPS, no cloud relay, no Docker.</p>

<p align="center">
  <a href="README.md">Português (Brasil)</a> ·
  <a href="docs/architecture/00-plano-tecnico.md">Technical plan</a> ·
  <a href="docs/mvp-checklist.md">MVP status</a>
</p>

> **Project status: under active development (pre-MVP).** See the
> [MVP checklist](docs/mvp-checklist.md) for what is implemented, what is tested
> automatically and what still requires manual validation on real devices.

## The problem

Parishes and churches that stream Masses and services rarely have budget for
professional wireless camera systems. Most volunteers already carry capable
smartphones, but existing "phone as webcam" tools are either single-camera,
cloud-dependent, or unaware of what is actually on air in OBS.

## The solution

SacraLink runs on the same Windows PC that runs OBS:

- Phones publish H.264 + Opus over **WebRTC (WHIP)** straight to the church PC.
- A local **MediaMTX** media server (managed by SacraLink) remuxes the streams to
  **RTSP** for OBS — no transcoding.
- SacraLink creates and maintains one OBS source + scene per camera through
  **obs-websocket 5.x**.
- It computes **tally** (PREVIEW / ON AIR) from the real OBS state — including
  nested scenes and groups — and sends it back to each phone in real time.
- The operator can control **zoom, lens and quality** of each phone remotely.
- Works on a LAN **without internet**. Mobile 4G/5G is supported when the church
  network has a real inbound route (public IPv4 + port forwarding or global IPv6).
  CG-NAT without IPv6 is detected and explained honestly.

## Architecture

```
Phones (Flutter) ──HTTPS/WSS :7443──▶ sacralink-server.exe ──▶ MediaMTX (loopback)
       └──────── WebRTC media UDP/TCP :8189 ─────────────────▶ MediaMTX
SacraLink.exe (Wails dashboard) ──127.0.0.1:7380──▶ sacralink-server.exe ──ws:4455──▶ OBS
OBS Media Source ──rtsp://127.0.0.1:8554──▶ MediaMTX
```

| Layer | Stack |
|---|---|
| Server | Go 1.27, SQLite (pure Go), MediaMTX supervisor, obs-websocket 5.x client |
| Dashboard | Wails v2, React 18, TypeScript, Tailwind CSS |
| Mobile | Flutter, flutter_webrtc |
| Installer | Inno Setup 6 (`SacraLink-Setup-x64.exe`) |

Details: [technical plan](docs/architecture/00-plano-tecnico.md) and
[ADRs](docs/architecture/adr/).

## Requirements (end user)

- Windows 10/11 64-bit (Windows 10 is past end of support — see warning in app)
- OBS Studio 28 or newer with WebSocket server enabled
- Microsoft Edge WebView2 Runtime (bundled installer handles it)
- Android 8+ or iOS 14+ phones on the same Wi-Fi (or reachable over the internet)

## Development

See [docs/development.md](docs/development.md).

## Security

See [SECURITY.md](SECURITY.md). Remote access is disabled by default; the admin
API is loopback-only; devices pair with single-use, short-lived tokens.

## Roadmap

See section 13 of the [technical plan](docs/architecture/00-plano-tecnico.md#13-roadmap-técnico-priorizado).

## Known limitations

- Remote (4G/5G) access is impossible behind CG-NAT without IPv6 — SacraLink does
  not use any relay by design.
- The camera stops when the app goes to background or the screen locks
  (OS restriction, mainly iOS). The app keeps the screen awake while streaming.
- Optical zoom and lens switching depend on each phone's camera APIs.
- 8-camera support is a design target; it is only declared validated for the
  reference hardware and profiles listed in [docs/performance.md](docs/performance.md)
  once load tests are executed.

## License

[MIT](LICENSE). Third-party components and their licenses:
[docs/licenses.md](docs/licenses.md).
