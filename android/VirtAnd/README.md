# VirtAnd

A Kotlin/Android "point" client for VirtBBS, modeled on classic FidoNet
point software for VirtBBS. Runs mostly
offline: syncs new mail via real QWK/REP packets, lets the user browse a
previously-synced file catalog and queue downloads/uploads, and only talks
to the network during an explicit "synchronize" (manual, or a best-effort
WorkManager background pass).

## Module layout

```
android/VirtAnd/
├── core/   — pure Kotlin/JVM: UserApiClient, QWK/REP packet parsing.
│             No Android dependency at all — actually compiled and
│             test-verified in the development environment (see below).
└── app/    — the real Android app: Room (local cache), WorkManager
              (background sync), Compose UI. Requires the Android SDK to
              even configure — NOT verified in the development environment
              (no Android SDK was available there). See "Verification
              status" below.
```

This split exists specifically so the substantial, non-trivial parts of
VirtAnd's logic — the wire protocol and the binary QWK packet format,
both ported from the Go server's `internal/userapi` and `internal/qwk` —
could be genuinely compiled and unit-tested here, rather than written
blind the way the whole project would otherwise have to be.

## What it does

- **`core/UserApiClient.kt`** — JSON-over-TCP client for
  `internal/userapi`: one fresh socket per call, newline-delimited
  JSON request/response, token auth.
- **`core/QwkPacket.kt`** — `parseQwkPacket()` decodes a downloaded QWK
  packet's `MESSAGES.DAT` (128-byte header records, soft-CR-wrapped body
  blocks) into a flat list of messages; `buildRepPacket()` encodes queued
  replies into the REP text format the server's `internal/qwk.ParseRep`
  expects. Byte-for-byte the same layouts as `internal/qwk/qwk.go`.
- **`app/sync/SyncEngine.kt`** — the single "synchronize" entry point:
  conference list refresh → QWK download/import → file catalog refresh →
  execute queued downloads → execute queued uploads → nodelist
  version-check per subscribed network → build+upload a REP packet for
  queued replies (queue is only cleared on confirmed success).
- **`app/sync/SyncWorker.kt`** — periodic background sync via WorkManager.
  Per the plan, this is accepted-as-is best-effort: Doze/battery
  restrictions mean it won't run promptly (15-minute minimum interval,
  plus further OS deferral) — the primary flow is the user tapping
  Synchronize manually.
- **`app/ui/`** — Compose UI with four tabs (Messages, Files, Queue,
  Settings), message detail view, offline compose/reply, file search,
  upload description prompt, queue management, connection test, and
  FidoNet node lookup. The app bar shows `session.whoami` after sync.

## Building

Gradle wrapper and version catalog match
[ClonesApp](/Volumes/JohnDovey/Projects/ClonesApp) (Gradle 9.5.1, AGP
8.13.2, Kotlin 2.0, JDK 17). See `../../CLAUDE.md` for SDK paths and
AI-assistant notes.

Copy `local.properties.example` to `local.properties` if you don't already
have one:

```bash
sdk.dir=/Volumes/JohnDovey/Android/Sdk
```

```bash
cd android/VirtAnd
export JAVA_HOME="/usr/local/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home"

./gradlew :core:test           # pure JVM — no Android SDK needed
./gradlew :app:assembleDebug   # needs Android SDK + local.properties
./android-build.sh               # same as :app:assembleDebug
```

Open in Android Studio: **File → Open →** `android/VirtAnd`

> **JDK note:** use JDK 17 explicitly. Newer JDKs can break older Kotlin
> toolchains or trigger `Internal compiler error` from `compileKotlin`.

## Verification status

**`:core` was actually compiled and tested** in the development
environment (Gradle 9.5.1 + JDK 17, no Android SDK present) —
`gradle :core:test` passes both `QwkPacketTest` cases. This caught and fixed
a real bug before it ever shipped: `decodeBody` was decoding raw bytes as
US-ASCII, which silently replaces any byte > 0x7F (including the 0xE3
soft-CR marker itself) with U+FFFD, corrupting every multi-line message
body. Fixed by switching to ISO-8859-1, a lossless 1:1 byte↔char mapping.

**`:app` now builds** with the Android SDK on the JohnDovey drive
(`./gradlew :app:assembleDebug`, Gradle 9.5.1 + AGP 8.13.2 + Kotlin 2.0,
matching ClonesApp). It was originally written without an SDK available;
manual review before the first real compile caught two bugs:
- `executeQueuedDownloads` originally called `files.download` and threw
  the response away without ever saving the file — fixed to decode the
  base64 payload and write it to app-specific external storage.
- The upload file picker's `OpenDocument` URI grant is temporary by
  default; since uploads are only executed on the *next* synchronize
  (possibly after the app/device has restarted), the read permission
  needed to be persisted at pick-time via
  `takePersistableUriPermission` — fixed.

The first real `:app:assembleDebug` also needed two small compile fixes
(missing `kotlinx.serialization.json.int`/`long` imports in `SyncEngine.kt`,
`@OptIn(ExperimentalMaterial3Api::class)` for `TopAppBar` in `MainActivity.kt`).
Runtime behaviour on a device/emulator still needs manual verification.

## Known limitations

- WorkManager background sync is best-effort only (15-minute minimum
  interval, plus OS deferral) — manual **Synchronize** is the primary flow.
- No push notifications for new mail.
- No rich text / ANSI rendering in the message reader.
- File and node search require network access (not cached offline).
