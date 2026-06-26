// VirtTermMac — AppSettings.cs
// Simple JSON settings file under the platform's app-data directory
// (~/Library/Application Support/VirtTermMac on macOS, %AppData%\VirtTermMac
// on Windows, ~/.config/VirtTermMac on Linux) — server address, the user's
// API token (see internal/userapi/server.go's AuthenticateToken — generated
// via the BBS profile menu's [T]okens option), and which FidoNet networks to
// keep a local nodelist cache for.
using System;
using System.Collections.Generic;
using System.IO;
using System.Text.Json;

namespace VirtTermMac.Settings;

public class AppSettings
{
    public string Host { get; set; } = "127.0.0.1";
    public int TerminalPort { get; set; } = 6323; // internal/virtterm default
    public int UserApiPort { get; set; } = 9998;  // internal/userapi default
    public string Token { get; set; } = "";
    public bool IsSysop { get; set; } = false; // see DynamicMenuBuilder.SetSysopVisible's doc comment
    public List<string> SubscribedNetworks { get; set; } = new() { "FidoNet" };

    private static string FilePath =>
        Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "VirtTermMac", "settings.json");

    public static string NodelistCacheDir =>
        Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "VirtTermMac", "nodelists");

    public static AppSettings Load()
    {
        try
        {
            if (File.Exists(FilePath))
            {
                var json = File.ReadAllText(FilePath);
                var s = JsonSerializer.Deserialize<AppSettings>(json);
                if (s != null) return s;
            }
        }
        catch { /* fall through to defaults */ }
        return new AppSettings();
    }

    public void Save()
    {
        var dir = Path.GetDirectoryName(FilePath)!;
        Directory.CreateDirectory(dir);
        var json = JsonSerializer.Serialize(this, new JsonSerializerOptions { WriteIndented = true });
        File.WriteAllText(FilePath, json);
    }
}
