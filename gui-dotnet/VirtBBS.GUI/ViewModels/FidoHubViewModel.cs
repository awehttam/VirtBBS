using System.Threading;
using System.Threading.Tasks;
using VirtBBS.GUI.Models;

namespace VirtBBS.GUI.ViewModels;

public class FidoHubViewModel(ApiClient client) : ViewModelBase
{
    public FidoNetworksViewModel Networks { get; } = new(client);
    public FidoRoutingViewModel Routing { get; } = new(client);
    public FidoJoinViewModel Join { get; } = new(client);
    public FidoToolsViewModel Tools { get; } = new(client);
    public FidoViewModel Operations { get; } = new(client);

    public async Task LoadAllAsync(CancellationToken ct = default)
    {
        await Networks.LoadAsync(ct);
        Routing.SelectedNetwork = Networks.SelectedNetwork;
        Join.SelectedNetwork = Networks.SelectedNetwork;
        Tools.SelectedNetwork = Networks.SelectedNetwork;
        Operations.SelectedNetwork = Networks.SelectedNetwork;
        await Task.WhenAll(
            Routing.LoadAsync(ct),
            Join.LoadAsync(ct),
            Tools.LoadNetworksAsync(ct),
            Operations.LoadNetworksAsync(ct)
        );
    }
}
