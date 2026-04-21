# Mobile app architecture — REST vs gRPC

**Short answer: use REST for the student mobile app. Add WebSocket for real-time
surfaces. Add gRPC only if and when you have a measured bandwidth/latency problem
on low-end devices — probably never for a coaching app.**

## Why REST wins here

### 1. The backend is already 100% REST (~500 endpoints)
Switching to gRPC means either rewriting every handler or running two surfaces.
Both waste engineer-months that buy zero end-user improvement.

### 2. Your bottleneck is not payload size
For PW-style traffic, 90%+ of the bytes are video streams + PDFs from MinIO.
Those are served over HTTP(S) directly from object storage, untouched by gRPC
or REST. Compressing 400-byte JSON with protobuf doesn't move the needle
when the user is downloading 200 MB of lecture video.

### 3. WebSocket already covers real-time
- Live chat: `/api/v1/chat/ws/:stream_id` — working
- HLS stream: pulled directly from Nginx-RTMP over HTTP
- Doubts, announcements: in-app notifications polled from `/notifications`

If you want push notifications on the phone, use **FCM (Android) / APNs (iOS)** —
that's a separate surface from REST or gRPC.

### 4. Tooling for a REST mobile app is vastly better
- OpenAPI → auto-generated Swift/Kotlin/Dart clients (`openapi-generator`)
- Browser-based debugging via Swagger UI (now at `/swagger/index.html`)
- Easy caching via HTTP headers and reverse proxies
- Works out of the box with CDNs, WAFs, and retry libraries
- Every mobile dev knows it

### 5. gRPC's real wins don't apply to a student app
| gRPC strength | Applies to student app? |
|---|---|
| Low-latency service-to-service RPC | No — client ↔ server |
| Tight schema coupling across polyglot services | No — one backend, one mobile codebase |
| Bi-directional streaming | Marginal — WebSocket handles this |
| Small binary frames | Negligible savings vs. video bytes |
| HTTP/2 multiplexing | REST over HTTP/2 gets most of this already |

## Where gRPC *might* pay off later

Keep as a future option, not for v1:

- **Internal service-to-service calls**: if you split off AI doubt solving, video
  transcoding workers, or an analytics pipeline into separate services, gRPC
  between those backends is a fine choice.
- **Teacher/admin desktop app (Electron or web)**: grpc-web is usable but
  noticeably more work than calling your existing REST endpoints.

## Recommended stack for the student mobile app

1. **Flutter** or **React Native** for the app (cross-platform, big hiring pool).
2. **OpenAPI client generation** — run `openapi-generator` against
   `http://localhost:3000/swagger/doc.json` to produce typed clients.
   ```bash
   openapi-generator-cli generate \
     -i http://localhost:3000/swagger/doc.json \
     -g dart \
     -o mobile/generated/api
   ```
3. **Video playback**: hls.js / ExoPlayer / AVPlayer pointing at the HLS URL
   returned by `GET /api/v1/streams/:id` (`hls_url` field).
4. **Live chat**: WebSocket client to `/api/v1/chat/ws/:stream_id?token=...`.
5. **Offline lectures**:
   - Get a download token: `POST /api/v1/downloads/token`
   - Fetch: `GET /api/v1/downloads/fetch?token=...` → presigned MinIO URL
   - Store video + metadata locally; use `watched_seconds` sync via
     `POST /api/v1/lectures/watch` when back online.
6. **Push notifications**: register an FCM/APNs token on login, backend fans
   out via `POST /admin/notifications/send` or a server-side job when events
   happen (attendance low, doubt answered, fee due, etc.).
7. **Auth**: access token (15 m) + refresh token (7 d). Store refresh in
   encrypted storage (iOS Keychain, Android Keystore). Rotate on use — the
   backend now blocklists reused refresh tokens via Redis.

## Performance checklist for low-end phones / 2G/3G

These are the levers that actually move battery + bandwidth:

- Enable HTTP/2 on the edge (Nginx / ALB / Cloudflare) — already free with gRPC
  or HTTPS REST.
- Serve `Content-Encoding: gzip` or `br` for JSON (Fiber supports both).
- Pre-resolve DNS via HTTP/3 + Alt-Svc headers.
- Paginate aggressively — `/lectures`, `/assignments`, etc. all accept
  `?limit=&offset=`; keep `limit` ≤ 20 by default on mobile.
- Lazy-fetch thumbnails, not full lists.
- Use HLS adaptive bitrate (you already register 240p–1080p variants via
  `POST /downloads/variants`). The player picks the right bitrate for the link.
- Sync progress/doubts/bookmarks in background batches, not per-tap.

## When you would *actually* reach for gRPC

You will know it's time when all of these are true:

1. The JSON payload savings from protobuf would reclaim > 20% of your mobile
   data usage (not possible with video-heavy traffic).
2. Your teams are already writing proto files for internal service comms.
3. You have dedicated client infra engineers who can own client regeneration.

For PW-style learning platforms, teams never hit that threshold. Spend that
engineering budget on video transcoding, search relevance, and content quality —
those compound.

## TL;DR

> **Build the mobile app against the REST API you already have. Use
> WebSocket for live chat. Use FCM/APNs for push. Skip gRPC until you
> have a reason you can point to on a graph.**
