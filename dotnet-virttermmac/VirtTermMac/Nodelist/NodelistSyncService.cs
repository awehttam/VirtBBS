// VirtTermMac — NodelistSyncService.cs
//
// Per the plan's "refresh only when changed" requirement: for each
// subscribed FidoNet network, ask internal/userapi's fido.nodelist.version
// endpoint for the server's current (imported_at, node_count), compare
// against what's cached locally, and only re-fetch the actual node search
// data if it changed. VirtTermMac has no need to mirror the *entire* nodelist
// client-side (unlike VirtAnd's offline-first design) — it just needs to
// know when to invalidate any cached fido.nodes.search results, so this
// only persists the small version-stamp file, not full node data.
using System;
using System.IO;
using System.Text.Json;
using System.Threading.Tasks;
using VirtTermMac.Net;
using VirtTermMac.Settings;

namespace VirtTermMac.Nodelist;

public class NodelistSyncService
{
    private readonly UserApiClient _api;

    public NodelistSyncService(UserApiClient api)
    {
        _api = api;
    }

    /// <summary>
    /// Checks one network's version against the local cache stamp. Returns
    /// true if the server's nodelist has changed since our last check (and
    /// updates the local stamp to match) — false if nothing changed.
    /// </summary>
    public async Task<bool> CheckAndUpdateAsync(string network)
    {
        var server = await _api.CallAsync<NodelistVersion>("fido.nodelist.version", new { network });
        if (server == null) return false; // network has no imported nodelist yet

        var stampPath = StampPath(network);
        if (File.Exists(stampPath))
        {
            var cached = JsonSerializer.Deserialize<NodelistVersion>(File.ReadAllText(stampPath));
            if (cached != null && cached.ImportedAt == server.ImportedAt && cached.NodeCount == server.NodeCount)
                return false; // unchanged — nothing to do
        }

        Directory.CreateDirectory(AppSettings.NodelistCacheDir);
        File.WriteAllText(stampPath, JsonSerializer.Serialize(server));
        return true;
    }

    /// <summary>Checks every subscribed network, returning the ones that changed.</summary>
    public async Task<string[]> CheckAllAsync(System.Collections.Generic.IEnumerable<string> networks)
    {
        var changed = new System.Collections.Generic.List<string>();
        foreach (var net in networks)
        {
            try
            {
                if (await CheckAndUpdateAsync(net)) changed.Add(net);
            }
            catch
            {
                // Network unreachable / endpoint error for this one network —
                // skip it this round rather than aborting the whole sync.
            }
        }
        return changed.ToArray();
    }

    private static string StampPath(string network) =>
        Path.Combine(AppSettings.NodelistCacheDir, SanitizeFileName(network) + ".version.json");

    private static string SanitizeFileName(string s)
    {
        foreach (var c in Path.GetInvalidFileNameChars())
            s = s.Replace(c, '_');
        return s;
    }
}
