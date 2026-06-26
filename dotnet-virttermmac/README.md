# VirtTermMac

A macOS-native (and cross-platform: Linux/Windows too) graphical terminal
client for VirtBBS, connecting over VirtBBS's own TLS protocol
(`internal/virtterm`, default port **6323**) instead of Telnet or SSH —
an **Avalonia UI port of [`dotnet-virtterm/VirtTerm`](../dotnet-virtterm)**
(WinForms, Windows-only), built specifically so the same client experience
works on macOS without a Windows machine.

## Why a separate project instead of changing VirtTerm

`VirtTerm` stays as-is for Windows users (native WinForms, no Avalonia
dependency). `VirtTermMac` is a sibling project targeting the same
protocol and functionality through Avalonia, which actually runs on
macOS/Linux. The two share no project reference, but most of the
non-UI logic was carried over **unmodified** (only namespaces changed):

| File | Status |
|---|---|
| `Terminal/AnsiScreen.cs` | Unmodified — pure C# state machine, no UI framework dependency |
| `Terminal/Cp437.cs` | Unmodified — CP437→Unicode table |
| `Net/TerminalConnection.cs` | Unmodified — `TcpClient`/`SslStream`, no UI dependency |
| `Net/UserApiClient.cs`, `Net/Models.cs` | Unmodified — JSON/TCP client for `internal/userapi` |
| `Settings/AppSettings.cs` | Unmodified logic; settings folder renamed `VirtTermMac` so it doesn't collide with VirtTerm's `%AppData%\VirtTerm` if both are ever installed on the same Windows machine |
| `Nodelist/NodelistSyncService.cs` | Unmodified |
| `Terminal/TerminalControl.cs` | **Rewritten** — Avalonia `Control` + `DrawingContext.DrawText`/`FillRectangle` instead of WinForms `Control`/GDI+. Input via `OnKeyDown` (arrows + Enter/Backspace/Tab/Escape) and `OnTextInput` (printable characters) instead of WinForms' `OnKeyDown`/`OnKeyPress` |
| `Menus/DynamicMenuBuilder.cs` | **Rewritten** — Avalonia `Menu`/`MenuItem` instead of WinForms `MenuStrip`/`ToolStripMenuItem`. (Namespace is `Menus`, not `Menu`, to avoid colliding with the `Avalonia.Controls.Menu` type name.) Same "remote control" single-keystroke design, same `Command:`-prompt enable/disable gate |
| `Views/ConnectWindow.*`, `Views/AboutWindow.*` | **Rewritten** as Avalonia `Window`s (XAML + code-behind) replacing the WinForms `Form`s |
| `Views/MainWindow.*` | **Rewritten** — same architecture as the WinForms `MainForm.cs` (built in code-behind, since the menu and terminal-grid size are both determined at runtime), just Avalonia controls and `Dispatcher.UIThread` instead of WinForms ones |

## Building

Requires the [.NET 8 SDK](https://dotnet.microsoft.com/download/dotnet/8.0).
Unlike `dotnet-virtterm` (WinForms, Windows-only), this one is genuinely
cross-platform:

```bash
cd dotnet-virttermmac/VirtTermMac
dotnet build
dotnet run
```

## Verification status

**This one was actually built and run** on macOS (unlike `VirtTerm`,
which could only be type-checked there). `dotnet build` succeeds with
zero warnings and zero errors. Launching it (`dotnet run`) produces a
stable process with no startup exceptions and a realistic memory
footprint for an initialized Avalonia renderer (confirmed via process
monitoring in the development environment, which had no way to capture
a screenshot of the actual window). Two real `FormattedText` API
mismatches were caught and fixed during this process — the constructor
signature in Avalonia 12 is
`FormattedText(text, CultureInfo, FlowDirection, Typeface, double, IBrush?)`,
not the WPF-style overload used in early drafts.

**Still needs a manual interactive check** on a real desktop session:
does the window actually appear and render correctly, does the dynamic
menu enable/disable at the right moments, does typing and clicking menu
items send the right bytes to a real `internal/virtterm` connection.
The architecture is identical to `VirtTerm`'s already-described
behavior in `../dotnet-virtterm/README.md` — what's untested here is
specifically the Avalonia rendering/input layer, not the underlying
protocol logic (which is the unmodified, shared code listed above).

## Known limitations (same as VirtTerm, by design — see the plan)

- **No Zmodem support — file uploads/downloads through the BBS's `[F]iles`
  menu do not work in this client.** VirtBBS's file transfers
  (`internal/transfer/zmodem.go`) are pure-Go Zmodem over the raw Telnet/SSH
  byte stream: the server sends a `ZRQINIT`/`ZRINIT` handshake and expects
  the *terminal client itself* to recognize it and switch into Zmodem
  receive/send mode. Neither `VirtTerm` nor `VirtTermMac` does any such
  detection — `TerminalConnection` just forwards raw bytes to the ANSI
  screen renderer, so a Zmodem handshake would be displayed as garbage
  characters instead of triggering a download. Implementing this would mean
  porting (or binding to) a Zmodem implementation client-side, sniffing the
  incoming byte stream for the handshake sequence, and wiring up a real file
  picker (`SaveFileDialog`/`OpenFileDialog` — Avalonia's equivalent is
  `IStorageProvider`) for where downloaded files land and which file gets
  uploaded. Not yet done in either client.
- Fixed 80x25 grid — no resize negotiation, since VirtBBS's own session
  layer is hard-baked to this size.
- No native UI for any multi-step BBS flow (composing a message,
  transferring a file) — those are typed directly into the terminal pane.
- The server's TLS certificate is self-signed with no CA, so
  `TerminalConnection` accepts any certificate (same trust-on-first-connect
  model as SSH host keys).
- The in-window `Menu` is used instead of macOS's native top menu bar
  (`Avalonia.Native.NativeMenu`) — simpler to keep cross-platform and to
  wire up dynamically. A native top-bar menu could be added later without
  touching `DynamicMenuBuilder`'s logic.
