using System;
using System.Collections.ObjectModel;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using VirtBBS.GUI.Models;

namespace VirtBBS.GUI.ViewModels;

public partial class FidoRoutingViewModel(ApiClient client) : ViewModelBase
{
    [ObservableProperty] private string _status = "";
    [ObservableProperty] private string _selectedNetwork = FidoNetworksViewModel.PrimaryNetwork;
    [ObservableProperty] private string _newPattern = "";
    [ObservableProperty] private string _newRouteTo = "";
    [ObservableProperty] private FidoRoute? _selectedRoute;
    [ObservableProperty] private FidoMember? _selectedMember;
    [ObservableProperty] private string _routesExportText = "";
    [ObservableProperty] private string _routesImportText = "";
    [ObservableProperty] private string _routingExportText = "";
    [ObservableProperty] private string _routingImportText = "";

    [ObservableProperty] private string _editBbsName = "";
    [ObservableProperty] private string _editSysopName = "";
    [ObservableProperty] private string _editLocation = "";
    [ObservableProperty] private string _editContact = "";
    [ObservableProperty] private string _editBinkpHost = "";
    [ObservableProperty] private string _editPassword = "";

    public ObservableCollection<string> NetworkNames { get; } = [];
    public ObservableCollection<FidoRoute> Routes { get; } = [];
    public ObservableCollection<FidoMember> Members { get; } = [];

    partial void OnSelectedNetworkChanged(string value) => _ = LoadAsync();

    partial void OnSelectedMemberChanged(FidoMember? value)
    {
        if (value is null) return;
        EditBbsName = value.BBSName;
        EditSysopName = value.SysopName;
        EditLocation = value.Location;
        EditContact = value.Contact;
        EditBinkpHost = value.BinkpHost;
        EditPassword = value.Password;
    }

    [RelayCommand]
    public async Task LoadAsync(CancellationToken ct = default)
    {
        try
        {
            var names = await client.CallAsync<string[]>("fido.networks.list", null, ct) ?? [FidoNetworksViewModel.PrimaryNetwork];
            NetworkNames.Clear();
            foreach (var n in names) NetworkNames.Add(n);
            if (!NetworkNames.Contains(SelectedNetwork))
                SelectedNetwork = NetworkNames.FirstOrDefault() ?? FidoNetworksViewModel.PrimaryNetwork;

            var routes = await client.CallAsync<FidoRoute[]>("fido.routes.list",
                new { Network = SelectedNetwork }, ct) ?? [];
            Routes.Clear();
            foreach (var r in routes) Routes.Add(r);

            var members = await client.CallAsync<FidoMember[]>("fido.members.list",
                new { Network = SelectedNetwork }, ct) ?? [];
            Members.Clear();
            foreach (var m in members) Members.Add(m);

            Status = $"{Routes.Count} route(s), {Members.Count} member(s).";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task AddRouteAsync(CancellationToken ct = default)
    {
        if (string.IsNullOrWhiteSpace(NewPattern) || string.IsNullOrWhiteSpace(NewRouteTo))
        {
            Status = "Pattern and route-to required.";
            return;
        }
        try
        {
            await client.CallAsync("fido.routes.add",
                new { Network = SelectedNetwork, Pattern = NewPattern.Trim(), RouteTo = NewRouteTo.Trim() }, ct);
            NewPattern = NewRouteTo = "";
            await LoadAsync(ct);
            Status = "Route added.";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task RemoveRouteAsync(CancellationToken ct = default)
    {
        if (SelectedRoute is null) return;
        try
        {
            await client.CallAsync("fido.routes.remove",
                new { Network = SelectedNetwork, Pattern = SelectedRoute.Pattern }, ct);
            await LoadAsync(ct);
            Status = "Route removed.";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task ExportRoutesAsync(CancellationToken ct = default)
    {
        try
        {
            var r = await client.CallAsync<TextExport>("fido.routes.export",
                new { Network = SelectedNetwork }, ct);
            RoutesExportText = r?.Text ?? "";
            Status = "ROUTES.BBS exported.";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task ImportRoutesAsync(CancellationToken ct = default)
    {
        try
        {
            var r = await client.CallAsync<RoutesImportResult>("fido.routes.import", new
            {
                Network = SelectedNetwork,
                Text = RoutesImportText,
            }, ct);
            await LoadAsync(ct);
            Status = r is null ? "Imported." : $"Imported {r.Added} route(s), {r.Errors.Count} error(s).";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task ExportRoutingTableAsync(CancellationToken ct = default)
    {
        try
        {
            var r = await client.CallAsync<TextExport>("fido.routing.export",
                new { Network = SelectedNetwork }, ct);
            RoutingExportText = r?.Text ?? "";
            Status = "Member routing table exported.";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task ImportRoutingTableAsync(CancellationToken ct = default)
    {
        try
        {
            var r = await client.CallAsync<RoutingImportResult>("fido.routing.import", new
            {
                Network = SelectedNetwork,
                Text = RoutingImportText,
            }, ct);
            await LoadAsync(ct);
            Status = r is null ? "Imported." : $"Updated {r.Updated} member(s), {r.Unknown.Count} unknown.";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task SaveMemberAsync(CancellationToken ct = default)
    {
        if (SelectedMember is null) return;
        var m = new FidoMember
        {
            ID = SelectedMember.ID,
            Network = SelectedMember.Network,
            Zone = SelectedMember.Zone,
            Net = SelectedMember.Net,
            NodeNum = SelectedMember.NodeNum,
            Point = SelectedMember.Point,
            BBSName = EditBbsName,
            SysopName = EditSysopName,
            Location = EditLocation,
            Contact = EditContact,
            BinkpHost = EditBinkpHost,
            Password = EditPassword,
            IsHost = SelectedMember.IsHost,
            IsActive = SelectedMember.IsActive,
            IsDelegated = SelectedMember.IsDelegated,
        };
        try
        {
            await client.CallAsync("fido.members.update", m, ct);
            Status = $"Member {SelectedMember.Address} updated.";
            await LoadAsync(ct);
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }
}
