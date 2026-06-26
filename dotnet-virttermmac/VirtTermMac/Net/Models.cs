// VirtTermMac — Models.cs
// DTOs for internal/userapi responses. Field names match the Go JSON tags
// exactly (PascalCase for conferences.Conference/files.Dir/File, which use
// Go's default un-tagged marshaling; snake_case for fido.NodelistVersion,
// which has explicit json tags) — see internal/userapi/server.go.
using System.Text.Json.Serialization;

namespace VirtTermMac.Net;

public class Conference
{
    public int ID { get; set; }
    public string Name { get; set; } = "";
    public string Description { get; set; } = "";
    public bool Public { get; set; }
    public int ReadSec { get; set; }
    public int WriteSec { get; set; }
    public int SysopSec { get; set; }
    public bool Echo { get; set; }
}

public class FileDir
{
    public long ID { get; set; }
    public string Name { get; set; } = "";
    public string Description { get; set; } = "";
    public int ReadSec { get; set; }
    public int UploadSec { get; set; }
    public bool Active { get; set; }
}

public class FileEntry
{
    public long ID { get; set; }
    public long DirID { get; set; }
    public string Filename { get; set; } = "";
    public long Size { get; set; }
    public string Description { get; set; } = "";
    public string Uploader { get; set; } = "";
    public string UploadDate { get; set; } = "";
    public int Downloads { get; set; }
}

public class NodelistVersion
{
    [JsonPropertyName("network")] public string Network { get; set; } = "";
    [JsonPropertyName("imported_at")] public string ImportedAt { get; set; } = "";
    [JsonPropertyName("node_count")] public int NodeCount { get; set; }
}
