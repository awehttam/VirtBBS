# VirtBBS — AI assistant notes

Guidance for Claude, Cursor, and other coding agents working in this repository.

## Repository location

The repo may live on an external volume (e.g. `/Volumes/JohnDovey/Projects/BBS/VirtBBS`). Toolchains are installed on the **host system**, not on that volume.

## Go server

The BBS server is Go (no cgo). See `BUILDING.md` for full instructions.

```bash
go build ./cmd/virtbbs
./virtbbs -config VirtBBS.DAT
```

## Web interface

Browser-based BBS UI and sysop admin served by `internal/web`. Templates and static assets live under `paths.www` (default `www/`). See `www/README.md` for routes and feature checklist.

Default URL: **http://localhost:8081/**

Sysop administration: log in as sysop and use **Admin** in the nav bar (`/admin/*`).

## Android app (VirtAnd) — build like ClonesApp

### Reference project (working template)

Copy Gradle/project structure from:
`/Volumes/JohnDovey/Projects/ClonesApp`

Use the same tooling — do not invent a new Android stack:

- Kotlin 2.0 + Jetpack Compose + Room + KSP
- Gradle **9.5.1** wrapper (`./gradlew`, not system `gradle`)
- AGP **8.13.2**, `compileSdk`/`targetSdk` **35**, `minSdk` **26**, JVM **17**
- Version catalog in `android/VirtAnd/gradle/libs.versions.toml`

### SDK and JDK (macOS, JohnDovey drive)

| Item | Path |
|------|------|
| Android SDK | `/Volumes/JohnDovey/Android/Sdk` |
| JDK 17 | `/usr/local/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home` |
| Reference app | `/Volumes/JohnDovey/Projects/ClonesApp` |
| VirtAnd project | `android/VirtAnd/` |

Create `android/VirtAnd/local.properties` (gitignored; copy from `local.properties.example`):

```
sdk.dir=/Volumes/JohnDovey/Android/Sdk
```

If Gradle can't find Java or Kotlin fails to compile:

```bash
export JAVA_HOME="/usr/local/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home"
```

### VirtAnd layout

- `android/VirtAnd/core/` — pure Kotlin/JVM (`UserApiClient`, QWK parsing). No Android SDK required.
- `android/VirtAnd/app/` — Android app (Compose UI, Room, WorkManager). Requires SDK.

Server API: `internal/userapi` (per-device token auth). Users create tokens on the BBS via profile **[T]okens**.

### Build commands

```bash
# Verify Android toolchain (reference project):
cd /Volumes/JohnDovey/Projects/ClonesApp && ./gradlew assembleDebug

# VirtAnd JVM module only (no Android SDK):
cd android/VirtAnd && ./gradlew :core:test

# VirtAnd Android APK:
cd android/VirtAnd && ./gradlew :app:assembleDebug
# or:
cd android/VirtAnd && ./android-build.sh
```

### Scaffolding / extending VirtAnd

When adding Android dependencies or UI, mirror ClonesApp patterns:

- `gradle/libs.versions.toml` for dependency versions
- Compose `MainActivity` + navigation graph
- Room + KSP for local cache
- `android-build.sh` for CLI builds on the JohnDovey drive

## Common mistakes to avoid

- Assuming toolchains are on the same drive as the repo — Go, JDK, and Android SDK are on the system install paths above.
- Building `:app` without `local.properties` pointing at the SDK.
- Using JDK 21+ or JDK 8 for VirtAnd/ClonesApp — use JDK 17.
