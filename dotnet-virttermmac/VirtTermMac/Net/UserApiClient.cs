// VirtTermMac — UserApiClient.cs
//
// JSON-over-TCP client for internal/userapi (VirtBBS's token-authenticated
// end-user API — a separate port and trust boundary from the sysop-only
// management API that gui-dotnet/VirtBBS.GUI/Models/ApiClient.cs talks to).
// Structurally identical to that file: one fresh TCP connection per call,
// one newline-delimited JSON request, one newline-delimited JSON response.
// The only real difference is the auth payload — a per-device token
// generated via the BBS profile menu's [T]okens option, never a password.
using System;
using System.Net.Sockets;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Threading;
using System.Threading.Tasks;

namespace VirtTermMac.Net;

public class UserApiClient
{
    public string Host { get; set; } = "127.0.0.1";
    public int Port { get; set; } = 9998;
    public string Token { get; set; } = "";

    private static readonly JsonSerializerOptions Opts = new()
    {
        PropertyNamingPolicy = null,
        PropertyNameCaseInsensitive = true,
        WriteIndented = false,
        DefaultIgnoreCondition = System.Text.Json.Serialization.JsonIgnoreCondition.WhenWritingNull,
    };

    public async Task<JsonNode?> CallAsync(string method, object? @params = null, CancellationToken ct = default)
    {
        var req = new
        {
            method,
            @params,
            auth = new { token = Token },
        };

        string reqJson = JsonSerializer.Serialize(req, Opts) + "\n";
        byte[] reqBytes = Encoding.UTF8.GetBytes(reqJson);

        using var tcp = new TcpClient { NoDelay = true };
        await tcp.ConnectAsync(Host, Port, ct);
        var stream = tcp.GetStream();
        await stream.WriteAsync(reqBytes, ct);

        var sb = new StringBuilder();
        var buf = new byte[8192];
        while (true)
        {
            int n = await stream.ReadAsync(buf, ct);
            if (n == 0) break;
            sb.Append(Encoding.UTF8.GetString(buf, 0, n));
            if (sb.ToString().Contains('\n')) break;
        }

        var respJson = sb.ToString().Trim();
        if (string.IsNullOrEmpty(respJson))
            throw new UserApiException("Empty response from server.");

        var node = JsonNode.Parse(respJson) ?? throw new UserApiException("Invalid JSON from server.");

        var errNode = node["error"];
        if (errNode is not null && errNode.GetValueKind() != JsonValueKind.Null)
        {
            var errMsg = errNode.ToString();
            if (!string.IsNullOrEmpty(errMsg))
                throw new UserApiException(errMsg);
        }

        return node["result"];
    }

    public async Task<T?> CallAsync<T>(string method, object? @params = null, CancellationToken ct = default)
    {
        var node = await CallAsync(method, @params, ct);
        if (node is null) return default;
        return node.Deserialize<T>(Opts);
    }

    public async Task<bool> TestConnectionAsync(CancellationToken ct = default)
    {
        try
        {
            await CallAsync("conferences.list", null, ct);
            return true;
        }
        catch { return false; }
    }
}

public class UserApiException(string message) : Exception(message);
