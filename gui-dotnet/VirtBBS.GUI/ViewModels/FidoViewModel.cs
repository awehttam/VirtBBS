using System;
using System.Collections.ObjectModel;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using VirtBBS.GUI.Models;

namespace VirtBBS.GUI.ViewModels;

public partial class FidoViewModel(ApiClient client) : ViewModelBase
{
    [ObservableProperty] private string _status = "";
    [ObservableProperty] private string _selectedNetwork = FidoNetworksViewModel.DefaultPrimaryNetwork;

    // Nodelist search.
    [ObservableProperty] private string _nodeQuery = "";
    [ObservableProperty] private int _nodePage = 1;
    [ObservableProperty] private int _nodeTotalPages = 1;
    [ObservableProperty] private FidoNode? _selectedNode;

    public ObservableCollection<string> NetworkNames { get; } = [];
    public ObservableCollection<FidoNode> NodeResults { get; } = [];

    // Netmail compose.
    [ObservableProperty] private string _toAddr = "";
    [ObservableProperty] private string _toName = "";
    [ObservableProperty] private string _nmSubject = "";
    [ObservableProperty] private string _nmBody = "";
    [ObservableProperty] private bool _crash;

    // Nodelist import.
    [ObservableProperty] private string _importPath = "";
    [ObservableProperty] private string _versionText = "";

    [RelayCommand]
    public async Task LoadNetworksAsync(CancellationToken ct = default)
    {
        try
        {
            var names = await client.CallAsync<string[]>("fido.networks.list", null, ct)
                ?? [FidoNetworksViewModel.DefaultPrimaryNetwork];
            NetworkNames.Clear();
            foreach (var n in names) NetworkNames.Add(n);
            if (!NetworkNames.Contains(SelectedNetwork))
                SelectedNetwork = NetworkNames.FirstOrDefault() ?? FidoNetworksViewModel.DefaultPrimaryNetwork;
        }
        catch { /* ignore */ }
    }

    [RelayCommand]
    private async Task SearchNodesAsync(CancellationToken ct = default)
    {
        try
        {
            NodePage = 1;
            await PageSearchAsync(ct);
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task NextPageAsync(CancellationToken ct = default)
    {
        if (NodePage >= NodeTotalPages) return;
        NodePage++;
        await PageSearchAsync(ct);
    }

    [RelayCommand]
    private async Task PrevPageAsync(CancellationToken ct = default)
    {
        if (NodePage <= 1) return;
        NodePage--;
        await PageSearchAsync(ct);
    }

    private async Task PageSearchAsync(CancellationToken ct)
    {
        var r = await client.CallAsync<NodelistSearchResult>("fido.nodes.search",
            new { network = SelectedNetwork, query = NodeQuery, page = NodePage, size = 25 }, ct);
        if (r is null) return;
        NodeResults.Clear();
        foreach (var n in r.Nodes) NodeResults.Add(n);
        NodeTotalPages = r.Pages;
        Status = $"{r.Total} node(s) found.";
    }

    [RelayCommand]
    private async Task SendNetmailAsync(CancellationToken ct = default)
    {
        if (string.IsNullOrWhiteSpace(ToAddr)) { Status = "Destination address required."; return; }
        try
        {
            var cfg = await client.CallAsync<BbsConfig>("config.get", null, ct);
            var fromAddr = cfg?.Fido.Address ?? "";
            await client.CallAsync("fido.netmail.send", new
            {
                FromName = "Sysop",
                FromAddr = fromAddr,
                ToAddr = ToAddr,
                ToName = ToName,
                Subject = NmSubject,
                Body = NmBody,
                Crash = Crash,
                Network = SelectedNetwork == FidoNetworksViewModel.DefaultPrimaryNetwork ? "" : SelectedNetwork,
            }, ct);
            Status = "Netmail queued.";
            ToAddr = ToName = NmSubject = NmBody = "";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task ImportNodelistAsync(CancellationToken ct = default)
    {
        if (string.IsNullOrWhiteSpace(ImportPath)) { Status = "Path required."; return; }
        try
        {
            await client.CallAsync("fido.import.nodelist",
                new { path = ImportPath, network = SelectedNetwork }, ct);
            Status = "Nodelist imported.";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task FetchNodelistAsync(CancellationToken ct = default)
    {
        try
        {
            await client.CallAsync("fido.nodelist.fetch", new { network = SelectedNetwork }, ct);
            Status = "Nodelist fetched and imported.";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task TossAsync(CancellationToken ct = default)
    {
        try
        {
            await client.CallAsync("fido.toss", null, ct);
            Status = "Toss complete.";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task ScanAsync(CancellationToken ct = default)
    {
        try
        {
            await client.CallAsync("fido.scan", null, ct);
            Status = "Scan complete.";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task PollAsync(CancellationToken ct = default)
    {
        try
        {
            await client.CallAsync("fido.poll", new { network = SelectedNetwork }, ct);
            Status = "Poll complete.";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task CheckVersionAsync(CancellationToken ct = default)
    {
        try
        {
            var v = await client.CallAsync<NodelistVersion>("fido.nodelist.version",
                new { network = SelectedNetwork }, ct);
            VersionText = v is null
                ? $"No nodelist imported yet for '{SelectedNetwork}'."
                : $"{v.Network}: last imported {v.ImportedAt}, {v.NodeCount} node(s).";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }
}
