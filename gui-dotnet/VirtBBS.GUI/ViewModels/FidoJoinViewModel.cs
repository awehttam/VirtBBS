using System;
using System.Collections.ObjectModel;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using VirtBBS.GUI.Models;

namespace VirtBBS.GUI.ViewModels;

public partial class FidoJoinViewModel(ApiClient client) : ViewModelBase
{
    [ObservableProperty] private string _status = "";
    [ObservableProperty] private string _selectedNetwork = FidoNetworksViewModel.PrimaryNetwork;
    [ObservableProperty] private FidoJoinRequest? _selected;
    [ObservableProperty] private int _approveNet = 1;
    [ObservableProperty] private int _approveNode;
    [ObservableProperty] private bool _approveIsHost;
    [ObservableProperty] private string _approvedPassword = "";

    public ObservableCollection<string> NetworkNames { get; } = [];
    public ObservableCollection<FidoJoinRequest> Pending { get; } = [];

    partial void OnSelectedChanged(FidoJoinRequest? value)
    {
        if (value?.RequestedNet is int n)
            ApproveNet = n;
    }

    [RelayCommand]
    public async Task LoadAsync(CancellationToken ct = default)
    {
        try
        {
            var names = await client.CallAsync<string[]>("fido.networks.list", null, ct) ?? [FidoNetworksViewModel.PrimaryNetwork];
            NetworkNames.Clear();
            foreach (var n in names) NetworkNames.Add(n);

            var list = await client.CallAsync<FidoJoinRequest[]>("fido.join.list",
                new { Network = SelectedNetwork }, ct) ?? [];
            Pending.Clear();
            foreach (var r in list) Pending.Add(r);
            Status = $"{Pending.Count} pending join request(s).";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task ApproveAsync(CancellationToken ct = default)
    {
        if (Selected is null) return;
        try
        {
            var result = await client.CallAsync<JoinApproveResult>("fido.join.approve", new
            {
                Network = SelectedNetwork,
                ID = Selected.ID,
                Net = ApproveNet,
                Node = ApproveNode,
                IsHost = ApproveIsHost,
            }, ct);
            ApprovedPassword = result?.Password ?? "";
            Status = result?.Member is null
                ? "Approved."
                : $"Approved as {result.Member.Address}. Password: {ApprovedPassword}";
            await LoadAsync(ct);
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task DenyAsync(CancellationToken ct = default)
    {
        if (Selected is null) return;
        try
        {
            await client.CallAsync("fido.join.deny", new { ID = Selected.ID, DecidedBy = "Sysop" }, ct);
            Status = "Request denied.";
            await LoadAsync(ct);
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }
}
