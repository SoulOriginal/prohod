# Architecture

Prohod has one small core shared by every platform and a thin UI on top.

```
UI (desktop: Wails/TS, Android: Kotlin)
        │
        ▼
core
 ├─ engine    the ByeDPI binary, run on 127.0.0.1:<random>, integrity-checked before start
 ├─ profile   method table + pinned domain lists + fallback chain → engine arguments
 ├─ front     loopback SOCKS5 server; relays to the engine; counts traffic per service
 ├─ sysproxy  points the OS proxy at `front` and restores the exact originals
 ├─ guard     a child process that restores the proxy if the app dies
 └─ logs      ring buffer + a size-capped log file
```

## The path a request takes

1. An app opens a connection. The OS proxy (desktop) or the local VPN tunnel (Android) sends
   it to the loopback SOCKS5 server.
2. `front` hands the request to the engine over loopback.
3. The engine looks at the destination. If the domain is in a pinned list, it applies that
   list's method. Otherwise it sends the request untouched, and only retries with a method if
   it detects a block (no server hello within a short window, or a reset). A method that works
   is cached for that destination IP.
4. Bytes are relayed back and counted per service for the UI.

## Why this shape

- Most sites are not blocked, and reshaping traffic can break things like large CDN
  downloads, so the default is to touch nothing and step in only when a block shows up.
- Big CDNs rotate IPs, so a working method is bound to the domain name rather than an IP.
  The pinned lists come from a field test.
- The system proxy is restored on disconnect, on quit, by the guard after a crash, and by a
  stale-state check on the next start, so you are never left without a working connection.

## Methods

A "method" is one set of engine parameters (packet split / reorder / out-of-band byte / TLS
record fragmentation, and combinations). The UI never shows the parameters, only neutral
labels, but they are plain in the code. The chain order is a greedy set cover over the field
matrix: the method that opens the most still-closed domains goes first, then the next best,
and so on.

## Platform notes

- **Desktop** uses the OS system proxy (macOS `networksetup`, Linux KDE/GNOME settings). The
  engine is embedded in the binary and extracted, integrity-checked, to a cache dir.
- **Android** uses a `VpnService`. A TUN device feeds `hev-socks5-tunnel`, which turns packets
  into SOCKS5 and sends them to the engine. The engine ships as a native library so Android
  extracts it in a runnable form.

## Limits

Prohod reshapes a handshake that reaches the server. It cannot help when packets never reach
the server (IP-blocked services) or when the server refuses your region (geo-blocked sites).
Those need traffic to exit through a different IP, which is a job for a VPN or a proxy.
