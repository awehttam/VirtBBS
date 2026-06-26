// VirtTermMac — ConnectWindow.axaml.cs
// Login dialog: server address + the two ports (terminal TLS / user API)
// + the per-device API token. The token itself is generated on the BBS
// side via the profile menu's [T]okens option — this dialog has no
// "create account" flow, it just collects what the user already
// generated there.
using Avalonia.Controls;
using Avalonia.Interactivity;
using VirtTermMac.Settings;

namespace VirtTermMac.Views;

public partial class ConnectWindow : Window
{
    public ConnectWindow()
    {
        InitializeComponent();
    }

    public ConnectWindow(AppSettings current) : this()
    {
        HostBox.Text = current.Host;
        TermPortBox.Value = current.TerminalPort;
        ApiPortBox.Value = current.UserApiPort;
        TokenBox.Text = current.Token;
        IsSysopBox.IsChecked = current.IsSysop;
        _subscribedNetworks = current.SubscribedNetworks;
    }

    private System.Collections.Generic.List<string> _subscribedNetworks = new() { "FidoNet" };

    private void OnConnect(object? sender, RoutedEventArgs e)
    {
        var result = new AppSettings
        {
            Host = HostBox.Text?.Trim() ?? "",
            TerminalPort = (int)(TermPortBox.Value ?? 6323),
            UserApiPort = (int)(ApiPortBox.Value ?? 9998),
            Token = TokenBox.Text?.Trim() ?? "",
            IsSysop = IsSysopBox.IsChecked ?? false,
            SubscribedNetworks = _subscribedNetworks,
        };
        Close(result);
    }

    private void OnCancel(object? sender, RoutedEventArgs e) => Close(null);
}
