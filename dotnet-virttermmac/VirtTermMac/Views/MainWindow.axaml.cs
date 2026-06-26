// VirtTermMac — MainWindow.axaml.cs
//
// Hosts the TerminalControl (the live 80x25 ANSI pane, internal/virtterm)
// and the dynamic Menu (internal/userapi-driven, see
// Menu/DynamicMenuBuilder.cs). Wires keystrokes from both the terminal
// control and the menu into the same TerminalConnection.Send call, and
// polls nodelist versions for subscribed networks once per connection.
//
// Avalonia port of VirtTerm's WinForms MainForm.cs — same architecture,
// same event wiring, just Avalonia controls/dispatcher instead of
// WinForms ones.
using System;
using System.Threading.Tasks;
using Avalonia.Controls;
using Avalonia.Layout;
using Avalonia.Threading;
using VirtTermMac.Menus;
using VirtTermMac.Net;
using VirtTermMac.Nodelist;
using VirtTermMac.Settings;
using VirtTermMac.Terminal;

namespace VirtTermMac.Views;

public partial class MainWindow : Window
{
    private AppSettings _settings;
    private readonly AnsiScreen _screen = new();
    private readonly TerminalConnection _conn;
    private readonly TerminalControl _terminalControl;
    private readonly DynamicMenuBuilder _menuBuilder = new();
    private readonly TextBlock _statusLabel = new() { Margin = new Avalonia.Thickness(6, 2) };

    public MainWindow()
    {
        InitializeComponent();

        _settings = AppSettings.Load();

        _conn = new TerminalConnection(_screen);
        _conn.Disconnected += () => Dispatcher.UIThread.Post(() => SetStatus("Disconnected"));
        _conn.ConnectionError += ex => Dispatcher.UIThread.Post(() => SetStatus($"Error: {ex.Message}"));

        _terminalControl = new TerminalControl(_screen);
        _terminalControl.KeyInput += data => _conn.Send(data);

        // The "Command: " gate is checked on every screen update (cheap
        // substring check — see AnsiScreen's rolling byte tail) and
        // reflected into the menu's enabled state on the UI thread.
        _screen.Changed += () => Dispatcher.UIThread.Post(
            () => _menuBuilder.SetAtPrompt(_screen.IsAtCommandPrompt));

        _menuBuilder.Keystroke += b => _conn.Send(new[] { b });
        _menuBuilder.LogonRequested += () => _ = ConnectAsync();
        _menuBuilder.LogoffRequested += () => _conn.Disconnect();
        _menuBuilder.HelpRequested += ShowHelp;
        _menuBuilder.AboutRequested += () => new AboutWindow().ShowDialog(this);

        var menu = _menuBuilder.Build();
        _menuBuilder.SetSysopVisible(_settings.IsSysop);

        var statusBar = new Border
        {
            Background = Avalonia.Media.Brushes.Black,
            Child = _statusLabel,
        };
        DockPanel.SetDock(menu, Dock.Top);
        DockPanel.SetDock(statusBar, Dock.Bottom);

        var root = new DockPanel();
        root.Children.Add(menu);
        root.Children.Add(statusBar);
        root.Children.Add(_terminalControl);
        Content = root;

        SetStatus("Not connected");

        Opened += async (_, _) => await ConnectAsync();
    }

    private void SetStatus(string text) => _statusLabel.Text = text;

    private async Task ConnectAsync()
    {
        var dlg = new ConnectWindow(_settings);
        var result = await dlg.ShowDialog<AppSettings?>(this);
        if (result == null) return;

        _settings = result;
        _settings.Save();
        _menuBuilder.SetSysopVisible(_settings.IsSysop);

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
            await ShowMessage("Connection failed", ex.Message);
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
            {
                Dispatcher.UIThread.Post(() => SetStatus($"Nodelist updated: {string.Join(", ", changed)}"));
            }
        }
        catch
        {
            // userapi unreachable/misconfigured — nodelist sync is a
            // background convenience, never block the terminal session on it.
        }
    }

    private void ShowHelp()
    {
        _ = ShowMessage("VirtTermMac Help",
            "VirtTermMac is a graphical terminal for VirtBBS.\n\n" +
            "Type at the terminal pane exactly as you would over Telnet/SSH.\n" +
            "The BBS menu (top) sends the same single keystroke as typing it\n" +
            "yourself, and is only enabled while the BBS is showing its main\n" +
            "\"Command:\" prompt — mid-flow prompts (composing a message, etc.)\n" +
            "must be typed directly in the terminal pane.");
    }

    private async Task ShowMessage(string title, string message)
    {
        var dlg = new Window
        {
            Title = title,
            Width = 420,
            Height = 220,
            CanResize = false,
            WindowStartupLocation = WindowStartupLocation.CenterOwner,
        };
        var ok = new Button { Content = "OK", HorizontalAlignment = HorizontalAlignment.Center, IsDefault = true };
        ok.Click += (_, _) => dlg.Close();
        var panel = new DockPanel { Margin = new Avalonia.Thickness(16) };
        DockPanel.SetDock(ok, Dock.Bottom);
        panel.Children.Add(ok);
        panel.Children.Add(new TextBlock { Text = message, TextWrapping = Avalonia.Media.TextWrapping.Wrap });
        dlg.Content = panel;
        await dlg.ShowDialog(this);
    }

    protected override void OnClosed(EventArgs e)
    {
        _conn.Dispose();
        base.OnClosed(e);
    }
}
