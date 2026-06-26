// VirtTermMac — DynamicMenuBuilder.cs
//
// Avalonia port of VirtTerm's WinForms MenuStrip builder — same "remote
// control" design: a small client-side static table mirroring
// internal/session/session.go's mainMenu() exactly, rather than anything
// fetched from the server. Each top-level item is a SINGLE keystroke —
// clicking "Messages" sends 'M' into the terminal connection, exactly as
// if the user had typed it. No multi-step flow (composing a message,
// entering a conference, file transfer wizards) is modeled here — those
// stay as manual typing in the terminal pane, to avoid a real desync risk:
// clicking a menu macro while sitting at some other prompt mid-flow (e.g.
// "To:") would inject the keystroke into the wrong field. Every item here
// is therefore enabled only when AnsiScreen.IsAtCommandPrompt is true.
//
// Uses Avalonia's in-window Menu/MenuItem controls rather than the native
// macOS menu bar (NativeMenu) — simpler to keep cross-platform and to wire
// up dynamically; a native top-bar menu could be added later without
// touching this class's logic.
using System;
using Avalonia.Controls;

namespace VirtTermMac.Menus;

public class DynamicMenuBuilder
{
    /// <summary>Fired when a generated item is clicked — the single keystroke byte to send.</summary>
    public event Action<byte>? Keystroke;

    public event Action? LogonRequested;
    public event Action? LogoffRequested;
    public event Action? HelpRequested;
    public event Action? AboutRequested;

    private MenuItem? _bbsMenu;
    private MenuItem? _sysopItem;
    private MenuItem? _logonItem;
    private MenuItem? _logoffItem;

    // Mirrors mainMenu()'s exact set of single-keystroke commands.
    private static readonly (string Label, char Key)[] FixedBbsItems =
    {
        ("_Messages", 'M'),
        ("_Files", 'F'),
        ("_Conference", 'C'),
        ("_Users", 'U'),
        ("_Who's Online", 'W'),
        ("_Talk", 'T'),
        ("_Doors", 'D'),
        ("_PPE", 'P'),
        ("P_rofile", 'R'),
    };

    public Menu Build()
    {
        var menu = new Menu();

        // ── Connection menu (fixed: Logon/Logoff always present) ──────────
        var connMenu = new MenuItem { Header = "_Connection" };
        _logonItem = new MenuItem { Header = "_Logon..." };
        _logonItem.Click += (_, _) => LogonRequested?.Invoke();
        _logoffItem = new MenuItem { Header = "Log_off", IsEnabled = false };
        _logoffItem.Click += (_, _) => LogoffRequested?.Invoke();
        connMenu.Items.Add(_logonItem);
        connMenu.Items.Add(_logoffItem);
        connMenu.Items.Add(new Separator());
        var exitItem = new MenuItem { Header = "E_xit" };
        exitItem.Click += (_, _) =>
        {
            if (Avalonia.Application.Current?.ApplicationLifetime is
                Avalonia.Controls.ApplicationLifetimes.IClassicDesktopStyleApplicationLifetime lifetime)
            {
                lifetime.Shutdown();
            }
        };
        connMenu.Items.Add(exitItem);
        menu.Items.Add(connMenu);

        // ── BBS menu: generated single-keystroke items ────────────────────
        var bbsMenu = new MenuItem { Header = "_BBS" };
        foreach (var (label, key) in FixedBbsItems)
        {
            var item = new MenuItem { Header = label };
            item.Click += (_, _) => Keystroke?.Invoke((byte)key);
            bbsMenu.Items.Add(item);
        }
        bbsMenu.Items.Add(new Separator());
        _sysopItem = new MenuItem { Header = "_Sysop Menu", IsVisible = false };
        _sysopItem.Click += (_, _) => Keystroke?.Invoke((byte)'!'); // mainMenu()'s actual sysop key, not 'S' (that's now Stats)
        bbsMenu.Items.Add(_sysopItem);
        var goodbyeItem = new MenuItem { Header = "_Goodbye" };
        goodbyeItem.Click += (_, _) => Keystroke?.Invoke((byte)'G');
        bbsMenu.Items.Add(goodbyeItem);
        menu.Items.Add(bbsMenu);
        _bbsMenu = bbsMenu;

        // ── Help menu (fixed: Help/About always present) ──────────────────
        var helpMenu = new MenuItem { Header = "_Help" };
        var helpTopicsItem = new MenuItem { Header = "_Help Topics" };
        helpTopicsItem.Click += (_, _) => HelpRequested?.Invoke();
        helpMenu.Items.Add(helpTopicsItem);
        var aboutItem = new MenuItem { Header = "_About VirtTermMac..." };
        aboutItem.Click += (_, _) => AboutRequested?.Invoke();
        helpMenu.Items.Add(aboutItem);
        menu.Items.Add(helpMenu);

        SetAtPrompt(false);
        return menu;
    }

    /// <summary>
    /// Enables/disables every generated single-keystroke item based on
    /// whether the terminal is currently sitting at VirtBBS's main
    /// "Command: " prompt — see the file header for why this gate exists.
    /// </summary>
    public void SetAtPrompt(bool atPrompt)
    {
        if (_bbsMenu == null) return;
        foreach (var obj in _bbsMenu.Items)
        {
            if (obj is MenuItem mi && mi != _sysopItem)
                mi.IsEnabled = atPrompt;
        }
        if (_sysopItem != null) _sysopItem.IsEnabled = atPrompt && _sysopItem.IsVisible;
    }

    /// <summary>
    /// Shows/hides the Sysop Menu item. MainWindow sets this from the real
    /// session.whoami response once logged in; before that (or if
    /// session.whoami fails for some reason) it falls back to whatever the
    /// user checked in the Connect dialog. Either way the BBS itself still
    /// enforces the real security-level check if a non-sysop sends 'S'.
    /// </summary>
    public void SetSysopVisible(bool visible)
    {
        if (_sysopItem == null) return;
        _sysopItem.IsVisible = visible;
    }

    /// <summary>
    /// Greys out "Logon" and enables "Logoff" once logged in (or the reverse
    /// once logged out) — set by MainWindow on the first "Command: " prompt
    /// seen after connecting, and reversed when the connection drops.
    /// </summary>
    public void SetLoggedIn(bool loggedIn)
    {
        if (_logonItem != null) _logonItem.IsEnabled = !loggedIn;
        if (_logoffItem != null) _logoffItem.IsEnabled = loggedIn;
    }
}
