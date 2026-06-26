# VirtTerm

A graphical .NET 8 / WinForms terminal client for VirtBBS, connecting over
VirtBBS's own TLS protocol (`internal/virtterm` on the server, default port
**6323**) instead of Telnet or SSH — Phase 3 of the VirtAnd/VirtTerm plan.

## What it does

- Renders an 80x25 CP437/ANSI character grid (`Terminal/AnsiScreen.cs` +
  `Terminal/TerminalControl.cs`), fed raw bytes from the TLS connection —
  the exact same byte stream a Telnet client would see, since the server
  hands `internal/virtterm` connections straight to the unmodified
  `session.Run()`.
- A native Windows `MenuStrip`, built client-side from a small static table
  mirroring `mainMenu()`'s fixed items (`Menu/DynamicMenuBuilder.cs`).
  Clicking a top-level item sends that single keystroke into the terminal
  connection — nothing more. Multi-step flows (composing a message,
  transferring a file) are **not** modeled natively; they stay manual
  typing in the terminal pane. Every generated item is enabled only while
  the terminal is showing VirtBBS's literal `"Command: "` prompt, to avoid
  injecting a keystroke into the wrong field mid-flow.
- Per-device API token login (`Net/UserApiClient.cs`) against
  `internal/userapi` — generate a token on the BBS side via the profile
  menu's **[T]okens** option before connecting here.
- Nodelist "has it changed" polling (`Nodelist/NodelistSyncService.cs`)
  against `fido.nodelist.version`, once per connection, for whichever
  networks are listed in Settings.

## Building

Requires the [.NET 8 SDK](https://dotnet.microsoft.com/download/dotnet/8.0)
on Windows (WinForms is Windows-only).

```powershell
cd dotnet-virtterm\VirtTerm
dotnet build
dotnet run
```

> **Note:** this project was originally written without a .NET SDK
> available, so it couldn't be compiled at the time. The .NET 8 SDK is now
> available (see `../global.json`/`../CLAUDE.md`), and on macOS/Linux it
> compiles cleanly with `dotnet build -p:EnableWindowsTargeting=true`
> (zero warnings, zero errors) — that flag only unblocks compilation against
> the Windows reference assemblies, though; **it still cannot actually run**
> outside Windows, since WinForms has no real macOS/Linux runtime. A type
> check this way already caught and fixed one real bug (a blocking
> TLS handshake on the UI thread in `MainForm.ConnectAsync`). Still verify a
> real run on Windows before relying on it — type-checking proves the code
> compiles, not that the UI behaves correctly.

## Fonts

For pixel-accurate CP437 box-drawing/block art, install a real DOS-VGA
font such as **Px437 IBM VGA8** or **Perfect DOS VGA 437** — see
`Terminal/TerminalControl.PreferredFontFamilies`. Without one, it falls
back to Consolas, which renders box-drawing characters reasonably but not
identically to a real DOS font.

## Known limitations (by design, see the plan)

- **No Zmodem support — file uploads/downloads through the BBS's `[F]iles`
  menu do not work in this client.** VirtBBS's file transfers
  (`internal/transfer/zmodem.go`) are pure-Go Zmodem over the raw Telnet/SSH
  byte stream: the server sends a `ZRQINIT`/`ZRINIT` handshake and expects
  the *terminal client itself* to recognize it and switch into Zmodem
  receive/send mode. `TerminalConnection` just forwards raw bytes to the
  ANSI screen renderer — there's no handshake detection, so a Zmodem
  transfer would render as garbage characters instead of triggering a
  download/upload. Implementing this would mean porting (or binding to) a
  Zmodem implementation client-side, sniffing the incoming stream for the
  handshake, and wiring up a real file picker
  (`OpenFileDialog`/`SaveFileDialog`) for the upload source / download
  destination. Not yet done.
- Fixed 80x25 grid — no resize negotiation. VirtBBS's own session layer is
  hard-baked to this size, so there's nothing to negotiate.
- No native UI for composing messages, browsing files, or any other
  multi-step BBS flow — those are typed directly into the terminal pane.
- The server's TLS certificate is self-signed with no CA, so
  `TerminalConnection` accepts any certificate (same trust-on-first-connect
  model as SSH host keys) rather than validating against a certificate
  chain.
