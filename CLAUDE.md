# VirtBBS — AI assistant notes

Guidance for Claude, Cursor, and other coding agents working in this repository.

## Repository location

The repo may live on an external volume (e.g. `/Volumes/JohnDovey/Projects/BBS/VirtBBS`). Toolchains are installed on the **host system**, not on that volume.

## .NET SDK (macOS)

| Item | Path |
|------|------|
| `dotnet` CLI | `/usr/local/share/dotnet/dotnet` |
| SDKs | `/usr/local/share/dotnet/sdk/` |
| Runtimes | `/usr/local/share/dotnet/shared/` |
| User cache | `~/.dotnet/` |

This repo pins **.NET 8** via `global.json` (SDK 8.0.203). Projects target `net8.0`.

If `dotnet` is not found, ensure PATH includes:

```bash
export PATH="/usr/local/share/dotnet:$HOME/.dotnet/tools:$PATH"
```

Or invoke the full path: `/usr/local/share/dotnet/dotnet build …`

## .NET projects

| Project | Directory | Target | macOS build? |
|---------|-----------|--------|--------------|
| Sysop GUI (Avalonia) | `gui-dotnet/VirtBBS.GUI/` | `net8.0` | Yes |
| Terminal client (WinForms) | `dotnet-virtterm/VirtTerm/` | `net8.0-windows` | No — Windows only |

### Sysop GUI (primary .NET app on macOS)

```bash
cd gui-dotnet/VirtBBS.GUI
dotnet build
dotnet run
```

### Terminal client

`dotnet-virtterm` uses WinForms (`net8.0-windows`) — it cannot **run** on macOS/Linux (no WinForms runtime). For a real run, build on Windows instead.

It *can* be type-checked on macOS/Linux, which is useful for catching compile errors without a Windows machine:

```bash
cd dotnet-virtterm/VirtTerm
dotnet build -p:EnableWindowsTargeting=true
```

This only unblocks compilation against the Windows reference assemblies — it does not make the app runnable here. Don't rely on it as a substitute for a real run.

## Go server

The BBS server is Go (no cgo). See `BUILDING.md` for full instructions.

```bash
go build ./cmd/virtbbs
./virtbbs -config VirtBBS.DAT
```

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

- Assuming the .NET SDK is on the same drive as the repo — it is on the system install path above.
- Assuming the Android SDK is on the system drive — it is on `/Volumes/JohnDovey/Android/Sdk`.
- Trying to *run* `dotnet-virtterm` on macOS — it requires Windows. (Type-checking with `-p:EnableWindowsTargeting=true` works fine; running does not.)
- Using MAUI or .NET for VirtAnd — it is Kotlin/Android like ClonesApp.
- Building `:app` without `local.properties` pointing at the SDK.
- Using JDK 21+ or JDK 8 for VirtAnd/ClonesApp — use JDK 17.
- Using a .NET SDK older than 8 for GUI work — use 8.0.203 (or newer 8.x with `rollForward`).