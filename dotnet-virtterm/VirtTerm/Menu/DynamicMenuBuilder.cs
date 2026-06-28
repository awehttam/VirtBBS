// VirtTerm — DynamicMenuBuilder.cs
//
// Builds the native Windows MenuStrip using the "remote control" pattern
// from the VirtAnd/VirtTerm plan: VirtBBS has no structured menu-definition
// data (internal/session/session.go's mainMenu() just prints static text),
// so this is a small client-side static table mirroring that text exactly,
// rather than anything fetched from the server.
//
// Each top-level item is a SINGLE keystroke — clicking "Messages" sends
// 'M' into the terminal connection, exactly as if the user had typed it.
// Deliberately no multi-step flows are modeled here (composing a message,
// entering a conference, file transfer wizards) — those stay as manual
// typing in the terminal pane. Building a native equivalent for those would
// risk a real desync: if the user clicks a menu macro while sitting at some
// other prompt mid-flow (e.g. "To:"), the keystroke lands in the wrong
// field. To avoid that without any prompt-sniffing/state-tracking beyond a
// literal substring check, every item here is enabled only when
// AnsiScreen.IsAtCommandPrompt is true (i.e. the terminal is showing
// VirtBBS's own literal "\r\nCommand: " prompt from mainMenu()).
using System;
using System.Windows.Forms;

namespace VirtTerm.Menu;

public class DynamicMenuBuilder
{
    /// <summary>Fired when a generated item is clicked — the single keystroke byte to send.</summary>
    public event Action<byte>? Keystroke;

    /// <summary>Fired for the fixed Logon item.</summary>
    public event Action? LogonRequested;

    /// <summary>Fired for the fixed Logoff item.</summary>
    public event Action? LogoffRequested;

    /// <summary>Fired for the fixed Help item.</summary>
    public event Action? HelpRequested;

    /// <summary>Fired for the fixed About item.</summary>
    public event Action? AboutRequested;

    /// <summary>Fired for Mail → Offline Mail Reader (no live connection required).</summary>
    public event Action? OfflineMailRequested;

    private ToolStripMenuItem? _menuVirtBBS; // M/F/C/U/W/T/D/P/R/S/G submenu items, gated by prompt state
    private ToolStripMenuItem? _sysopItem;
    private ToolStripMenuItem? _logonItem;
    private ToolStripMenuItem? _logoffItem;

    // Mirrors mainMenu()'s exact set of single-keystroke commands.
    private static readonly (string Label, char Key)[] FixedBbsItems =
    {
        ("&Messages", 'M'),
        ("&Files", 'F'),
        ("&Conference", 'C'),
        ("&Users", 'U'),
        ("&Who's Online", 'W'),
        ("&Talk", 'T'),
        ("&Doors", 'D'),
        ("&PPE", 'P'),
        ("P&rofile", 'R'),
    };

    public MenuStrip Build()
    {
        var strip = new MenuStrip();

        // ── Connection menu (fixed: Logon/Logoff always present) ──────────
        var connMenu = new ToolStripMenuItem("&Connection");
        _logonItem = new ToolStripMenuItem("&Logon...", null, (_, _) => LogonRequested?.Invoke());
        _logoffItem = new ToolStripMenuItem("Log&off", null, (_, _) => LogoffRequested?.Invoke()) { Enabled = false };
        connMenu.DropDownItems.Add(_logonItem);
        connMenu.DropDownItems.Add(_logoffItem);
        connMenu.DropDownItems.Add(new ToolStripSeparator());
        var exitItem = new ToolStripMenuItem("E&xit", null, (_, _) => Application.Exit());
        connMenu.DropDownItems.Add(exitItem);
        strip.Items.Add(connMenu);

        // ── BBS menu: generated single-keystroke items ────────────────────
        var bbsMenu = new ToolStripMenuItem("&BBS");
        foreach (var (label, key) in FixedBbsItems)
        {
            var item = new ToolStripMenuItem(label, null, (_, _) => Keystroke?.Invoke((byte)key));
            bbsMenu.DropDownItems.Add(item);
        }
        bbsMenu.DropDownItems.Add(new ToolStripSeparator());
        _sysopItem = new ToolStripMenuItem("&Sysop Menu", null, (_, _) => Keystroke?.Invoke((byte)'!')) // mainMenu()'s actual sysop key, not 'S' (that's now Stats)
        {
            Visible = false, // only shown once the logged-in user is known to be a sysop
        };
        bbsMenu.DropDownItems.Add(_sysopItem);
        bbsMenu.DropDownItems.Add(new ToolStripMenuItem("&Goodbye", null, (_, _) => Keystroke?.Invoke((byte)'G')));
        strip.Items.Add(bbsMenu);
        _menuVirtBBS = bbsMenu;

        // ── Mail menu: offline QWK reader (no live connection required) ─
        var mailMenu = new ToolStripMenuItem("&Mail");
        mailMenu.DropDownItems.Add(new ToolStripMenuItem("&Offline Mail Reader...", null,
            (_, _) => OfflineMailRequested?.Invoke()));
        strip.Items.Add(mailMenu);

        // ── Help menu (fixed: Help/About always present) ──────────────────
        var helpMenu = new ToolStripMenuItem("&Help");
        helpMenu.DropDownItems.Add(new ToolStripMenuItem("&Help Topics", null, (_, _) => HelpRequested?.Invoke()));
        helpMenu.DropDownItems.Add(new ToolStripMenuItem("&About VirtTerm...", null, (_, _) => AboutRequested?.Invoke()));
        strip.Items.Add(helpMenu);

        SetAtPrompt(false);
        return strip;
    }

    /// <summary>
    /// Enables/disables every generated single-keystroke item based on
    /// whether the terminal is currently sitting at VirtBBS's main
    /// "Command: " prompt — see the file header for why this gate exists.
    /// </summary>
    public void SetAtPrompt(bool atPrompt)
    {
        if (_menuVirtBBS == null) return;
        foreach (ToolStripItem item in _menuVirtBBS.DropDownItems)
        {
            if (item is ToolStripMenuItem mi && mi != _sysopItem)
                mi.Enabled = atPrompt;
        }
        if (_sysopItem != null) _sysopItem.Enabled = atPrompt && _sysopItem.Visible;
    }

    /// <summary>
    /// Shows/hides the Sysop Menu item. MainForm sets this from the real
    /// session.whoami response once logged in; before that (or if
    /// session.whoami fails for some reason) it falls back to whatever the
    /// user checked in the Connect dialog. Either way the BBS itself still
    /// enforces the real security-level check if a non-sysop sends 'S'.
    /// </summary>
    public void SetSysopVisible(bool visible)
    {
        if (_sysopItem == null) return;
        _sysopItem.Visible = visible;
    }

    /// <summary>
    /// Greys out "Logon" and enables "Logoff" once logged in (or the reverse
    /// once logged out) — set by MainForm on the first "Command: " prompt
    /// seen after connecting, and reversed when the connection drops.
    /// </summary>
    public void SetLoggedIn(bool loggedIn)
    {
        if (_logonItem != null) _logonItem.Enabled = !loggedIn;
        if (_logoffItem != null) _logoffItem.Enabled = loggedIn;
    }
}
