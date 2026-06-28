using System;
using System.Collections.ObjectModel;
using System.Threading;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using VirtBBS.GUI.Models;

namespace VirtBBS.GUI.ViewModels;

public partial class FidoToolsViewModel(ApiClient client) : ViewModelBase
{
    [ObservableProperty] private string _status = "";
    [ObservableProperty] private string _selectedNetwork = FidoNetworksViewModel.DefaultPrimaryNetwork;
    [ObservableProperty] private string _pingAddr = "";
    [ObservableProperty] private string _pingToName = "";
    [ObservableProperty] private string _traceAddr = "";
    [ObservableProperty] private string _traceToName = "";
    [ObservableProperty] private string _areaFixAdds = "";
    [ObservableProperty] private string _areaFixRemoves = "";
    [ObservableProperty] private string _fileFixAdds = "";
    [ObservableProperty] private string _fileFixRemoves = "";

    public ObservableCollection<string> NetworkNames { get; } = [];

    [RelayCommand]
    public async Task LoadNetworksAsync(CancellationToken ct = default)
    {
        try
        {
            var names = await client.CallAsync<string[]>("fido.networks.list", null, ct)
                ?? [FidoNetworksViewModel.DefaultPrimaryNetwork];
            NetworkNames.Clear();
            foreach (var n in names) NetworkNames.Add(n);
        }
        catch { /* ignore */ }
    }

    [RelayCommand]
    private async Task PingAsync(CancellationToken ct = default)
    {
        if (string.IsNullOrWhiteSpace(PingAddr)) { Status = "Address required."; return; }
        try
        {
            var r = await client.CallAsync<PktResult>("fido.ping.send", new
            {
                Network = SelectedNetwork,
                ToAddr = PingAddr,
                ToName = PingToName,
            }, ct);
            Status = $"Ping sent → {r?.Pkt}";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task TraceAsync(CancellationToken ct = default)
    {
        if (string.IsNullOrWhiteSpace(TraceAddr)) { Status = "Address required."; return; }
        try
        {
            var r = await client.CallAsync<PktResult>("fido.trace.send", new
            {
                Network = SelectedNetwork,
                ToAddr = TraceAddr,
                ToName = TraceToName,
            }, ct);
            Status = $"Trace sent → {r?.Pkt}";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task RequestAreaFixAsync(CancellationToken ct = default)
    {
        try
        {
            var r = await client.CallAsync<PktResult>("fido.areafix.request", new
            {
                Network = SelectedNetwork,
                Adds = SplitTags(AreaFixAdds),
                Removes = SplitTags(AreaFixRemoves),
            }, ct);
            Status = $"AreaFix request sent → {r?.Pkt}";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task RequestFileFixAsync(CancellationToken ct = default)
    {
        try
        {
            var r = await client.CallAsync<PktResult>("fido.filefix.request", new
            {
                Network = SelectedNetwork,
                Adds = SplitTags(FileFixAdds),
                Removes = SplitTags(FileFixRemoves),
            }, ct);
            Status = $"FileFix request sent → {r?.Pkt}";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    private static string[] SplitTags(string s) =>
        s.Split(new[] { ' ', ',', '\n', '\r' }, StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries);
}
