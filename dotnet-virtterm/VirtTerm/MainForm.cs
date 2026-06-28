// VirtTerm — MainForm.cs
// Hosts the TerminalControl (the live 80x25 ANSI pane, internal/virtterm)
// and the dynamic MenuStrip (internal/userapi-driven, see
// Menu/DynamicMenuBuilder.cs). Wires keystrokes from both the terminal
// control and the menu into the same TerminalConnection.Send call, and
// polls nodelist versions for subscribed networks once per connection.
using System;
using System.Drawing;
using System.Threading.Tasks;
using System.Windows.Forms;
using VirtTerm.Forms;
using VirtTerm.Menu;
using VirtTerm.Net;
using VirtTerm.Nodelist;
using VirtTerm.Settings;
using VirtTerm.Terminal;

namespace VirtTerm;

public class MainForm : Form
{
    private const string DefaultTitle = "VirtTerm";

    private AppSettings _settings;
    private readonly AnsiScreen _screen = new();
    private readonly TerminalConnection _conn;
    private readonly TerminalControl _terminalControl;
    private readonly DynamicMenuBuilder _menuBuilder = new();
    private readonly StatusStrip _status = new();
    private readonly ToolStripStatusLabel _statusLabel = new("Not connected");

    // Set once per connection, the first time the terminal reaches the main
    // "Command: " prompt — that's the closest thing to a "login succeeded"
    // signal available from a dumb-terminal byte stream. Reset on disconnect
    // so a fresh connection fetches whoami again.
    private bool _loggedIn;
    private OfflineMailForm? _offlineMailForm;

    public MainForm()
    {
        _settings = AppSettings.Load();

        Text = DefaultTitle;
        StartPosition = FormStartPosition.CenterScreen;

        _conn = new TerminalConnection(_screen);
        _conn.Disconnected += () => BeginInvoke(new MethodInvoker(HandleLoggedOut));
        _conn.ConnectionError += ex => BeginInvoke(new MethodInvoker(() => SetStatus($"Error: {ex.Message}")));

        // Zmodem handoff callbacks fire synchronously on TerminalConnection's
        // background read thread (it blocks for the whole transfer), so the
        // dialog calls below use the blocking Invoke (not BeginInvoke) to
        // hop to the UI thread and wait for the result — safe here
        // specifically because we're never calling this *from* the UI
        // thread, so there's no deadlock risk in blocking on it.
        _conn.ZmodemResolveDownloadPath = info => (string?)Invoke(new Func<string?>(() =>
        {
            using var dlg = new FolderBrowserDialog
            {
                Description = $"Save {info.Filename} ({info.Size:N0} bytes) to...",
                UseDescriptionForTitle = true,
            };
            return dlg.ShowDialog(this) == DialogResult.OK
                ? System.IO.Path.Combine(dlg.SelectedPath, info.Filename)
                : null;
        }));

        _conn.ZmodemResolveUploadPath = () => (string?)Invoke(new Func<string?>(() =>
        {
            using var dlg = new OpenFileDialog { Title = "Select file to upload", Multiselect = false };
            return dlg.ShowDialog(this) == DialogResult.OK ? dlg.FileName : null;
        }));

        _conn.ZmodemProgress = bytes => BeginInvoke(new MethodInvoker(() => SetStatus($"Zmodem: {bytes:N0} bytes transferred...")));
        _conn.ZmodemCompleted += path => BeginInvoke(new MethodInvoker(() => SetStatus($"Zmodem transfer complete: {path}")));
        _conn.ZmodemFailed += msg => BeginInvoke(new MethodInvoker(() => SetStatus($"Zmodem transfer failed: {msg}")));

        _terminalControl = new TerminalControl(_screen);
        _terminalControl.KeyInput += data => _conn.Send(data);

        // The "Command: " gate is checked on every screen update (cheap
        // substring check — see AnsiScreen.UpdateTail) and reflected into
        // the menu's enabled state on the UI thread. The very first time it
        // goes true after a connect, treat that as "login succeeded" and
        // fetch the user/BBS identity for the title bar.
        _screen.Changed += () => BeginInvoke(new MethodInvoker(() =>
        {
            var atPrompt = _screen.IsAtCommandPrompt;
            _menuBuilder.SetAtPrompt(atPrompt);
            if (atPrompt && !_loggedIn)
            {
                _loggedIn = true;
                _menuBuilder.SetLoggedIn(true);
                _ = FetchWhoAmIAndUpdateTitleAsync();
            }
        }));

        _menuBuilder.Keystroke += b => _conn.Send(new[] { b });
        _menuBuilder.LogonRequested += () => _ = ConnectAsync();
        _menuBuilder.LogoffRequested += () => _conn.Disconnect();
        _menuBuilder.HelpRequested += ShowHelp;
        _menuBuilder.AboutRequested += () => new AboutForm().ShowDialog(this);
        _menuBuilder.OfflineMailRequested += ShowOfflineMail;

        var menuStrip = _menuBuilder.Build();
        _menuBuilder.SetSysopVisible(_settings.IsSysop);

        _status.Items.Add(_statusLabel);
        _status.Dock = DockStyle.Bottom;
        menuStrip.Dock = DockStyle.Top;
        _terminalControl.Dock = DockStyle.Fill;

        // Fill-docked control must be added last so Top/Bottom controls
        // reserve their space first and the terminal pane takes the rest.
        MainMenuStrip = menuStrip;
        Controls.Add(menuStrip);
        Controls.Add(_status);
        Controls.Add(_terminalControl);

        ClientSize = new Size(_terminalControl.Width, _terminalControl.Height + menuStrip.Height + _status.Height);

        Shown += async (_, _) => await ConnectAsync();
    }

    private void SetStatus(string text) => _statusLabel.Text = text;

    /// <summary>Reverses everything HandleLoggedOut undoes once the user reaches the main menu.</summary>
    private async Task FetchWhoAmIAndUpdateTitleAsync()
    {
        var api = new UserApiClient { Host = _settings.Host, Port = _settings.UserApiPort, Token = _settings.Token };
        try
        {
            var who = await api.CallAsync<WhoAmI>("session.whoami");
            if (who == null) return;
            BeginInvoke(new MethodInvoker(() =>
            {
                Text = $"{who.Name} — {who.BbsName} — {DefaultTitle}";
                _menuBuilder.SetSysopVisible(who.Sysop);
            }));
        }
        catch
        {
            // Couldn't fetch identity (userapi unreachable/misconfigured) —
            // leave the title generic rather than blocking the session on it.
        }
    }

    /// <summary>Resets title/menu state to "not logged in" — fired on disconnect.</summary>
    private void HandleLoggedOut()
    {
        SetStatus("Disconnected");
        _loggedIn = false;
        Text = DefaultTitle;
        _menuBuilder.SetLoggedIn(false);
    }

    private async Task ConnectAsync()
    {
        using var dlg = new ConnectForm(_settings);
        if (dlg.ShowDialog(this) != DialogResult.OK) return;

        _settings = dlg.Result;
        _settings.Save();
        _menuBuilder.SetSysopVisible(_settings.IsSysop);

        // A fresh connect attempt always starts in the "not logged in" state,
        // even if the previous session never cleanly fired Disconnected.
        _loggedIn = false;
        _menuBuilder.SetLoggedIn(false);
        Text = DefaultTitle;

        SetStatus($"Connecting to {_settings.Host}:{_settings.TerminalPort}...");
        try
        {
            // Connect() blocks on the TCP+TLS handshake — run it off the UI
            // thread so the window doesn't freeze while it's in progress.
            await Task.Run(() => _conn.Connect(_settings.Host, _settings.TerminalPort));
            SetStatus($"Connected to {_settings.Host}:{_settings.TerminalPort}");
        }
        catch (Exception ex)
        {
            SetStatus($"Connect failed: {ex.Message}");
            MessageBox.Show(this, ex.Message, "Connection failed", MessageBoxButtons.OK, MessageBoxIcon.Error);
            return;
        }

        _ = SyncNodelistsAsync();
    }

    private async Task SyncNodelistsAsync()
    {
        var api = new UserApiClient { Host = _settings.Host, Port = _settings.UserApiPort, Token = _settings.Token };
        var sync = new NodelistSyncService(api);
        try
        {
            var changed = await sync.CheckAllAsync(_settings.SubscribedNetworks);
            if (changed.Length > 0)
                BeginInvoke(new MethodInvoker(() =>
                    SetStatus($"Nodelist updated: {string.Join(", ", changed)}")));
        }
        catch
        {
            // userapi unreachable/misconfigured — nodelist sync is a
            // background convenience, never block the terminal session on it.
        }
    }

    private void ShowHelp()
    {
        MessageBox.Show(this,
            "VirtTerm is a graphical terminal for VirtBBS.\r\n\r\n" +
            "Type at the terminal pane exactly as you would over Telnet/SSH.\r\n" +
            "The BBS menu (top) sends the same single keystroke as typing it\r\n" +
            "yourself, and is only enabled while the BBS is showing its main\r\n" +
            "\"Command:\" prompt — mid-flow prompts (composing a message, etc.)\r\n" +
            "must be typed directly in the terminal pane.\r\n\r\n" +
            "Mail → Offline Mail Reader opens a graphical QWK offline\r\n" +
            "mail client. It works without a live BBS connection — open a\r\n" +
            ".QWK packet received via Zmodem or download from the BBS when\r\n" +
            "connected, compose replies, and save/upload a REP packet.",
            "VirtTerm Help", MessageBoxButtons.OK, MessageBoxIcon.Information);
    }

    private void ShowOfflineMail()
    {
        if (_offlineMailForm == null || _offlineMailForm.IsDisposed)
        {
            _offlineMailForm = new OfflineMailForm(_settings);
            _offlineMailForm.FormClosed += (_, _) => _offlineMailForm = null;
            _offlineMailForm.Show(this);
        }
        else
        {
            _offlineMailForm.BringToFront();
            _offlineMailForm.Focus();
        }
    }

    protected override void OnFormClosing(FormClosingEventArgs e)
    {
        _conn.Dispose();
        base.OnFormClosing(e);
    }
}
