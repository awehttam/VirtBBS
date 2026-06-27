// VirtBBS GUI — BbsModels.cs
// Plain C# records / classes matching the VirtBBS JSON API shapes.

using System;
using System.Collections.Generic;
using System.Text.Json.Serialization;

namespace VirtBBS.GUI.Models;

// ── Nodes ────────────────────────────────────────────────────────────────────

public record NodeStatus
(
    [property: JsonPropertyName("ID")]        int    NodeID,
    [property: JsonPropertyName("UserID")]    long   UserID,
    [property: JsonPropertyName("UserName")]  string UserName,
    [property: JsonPropertyName("City")]      string City,
    [property: JsonPropertyName("Status")]    string Status,
    [property: JsonPropertyName("Operation")] string Action,
    [property: JsonPropertyName("UpdatedAt")] DateTime ConnectedAt
);

// ── Users ────────────────────────────────────────────────────────────────────

public class BbsUser
{
    [JsonPropertyName("ID")]            public long   ID              { get; set; }
    [JsonPropertyName("Name")]          public string Name            { get; set; } = "";
    [JsonPropertyName("City")]          public string City            { get; set; } = "";
    [JsonPropertyName("SecurityLevel")] public int    SecurityLevel   { get; set; }
    [JsonPropertyName("TimesOnline")]   public int    TimesOnline     { get; set; }
    [JsonPropertyName("PageLength")]    public int    PageLength      { get; set; }
    [JsonPropertyName("ANSI")]          public bool   ANSI            { get; set; }
    [JsonPropertyName("EditorType")]    public string EditorType      { get; set; } = "simple";
    [JsonPropertyName("XferProtocol")] public string XferProtocol    { get; set; } = "Z";
    [JsonPropertyName("ExpertMode")]    public bool   ExpertMode      { get; set; }
    [JsonPropertyName("Deleted")]       public bool   Deleted         { get; set; }
    [JsonPropertyName("Sysop")]         public bool   Sysop           { get; set; }
    [JsonPropertyName("LastLoginDate")] public string LastLoginDate   { get; set; } = "";
    [JsonPropertyName("LastLoginTime")] public string LastLoginTime   { get; set; } = "";
    [JsonPropertyName("Comment1")]      public string Comment1        { get; set; } = "";
    [JsonPropertyName("PhoneBusiness")] public string PhoneBusiness   { get; set; } = "";
}

// ── Messages ─────────────────────────────────────────────────────────────────

public class BbsMessage
{
    [JsonPropertyName("ID")]           public long   ID           { get; set; }
    [JsonPropertyName("ConferenceID")] public int    ConferenceID { get; set; }
    [JsonPropertyName("MsgNumber")]    public int    MsgNumber    { get; set; }
    [JsonPropertyName("FromName")]     public string FromName     { get; set; } = "";
    [JsonPropertyName("ToName")]       public string ToName       { get; set; } = "";
    [JsonPropertyName("Subject")]      public string Subject      { get; set; } = "";
    [JsonPropertyName("DatePosted")]   public string DatePosted   { get; set; } = "";
    [JsonPropertyName("Body")]         public string Body         { get; set; } = "";
    [JsonPropertyName("Echo")]         public bool   Echo         { get; set; }
}

// ── Conferences ───────────────────────────────────────────────────────────────

public class BbsConference
{
    [JsonPropertyName("ID")]          public int    ID          { get; set; }
    [JsonPropertyName("Name")]        public string Name        { get; set; } = "";
    [JsonPropertyName("Description")] public string Description { get; set; } = "";
    [JsonPropertyName("Public")]      public bool   Public      { get; set; }
    [JsonPropertyName("ReadSec")]     public int    ReadSec     { get; set; }
    [JsonPropertyName("WriteSec")]    public int    WriteSec    { get; set; }
    [JsonPropertyName("SysopSec")]    public int    SysopSec    { get; set; }
    [JsonPropertyName("Echo")]        public bool   Echo        { get; set; }
    [JsonPropertyName("EchoTag")]     public string EchoTag     { get; set; } = "";
    [JsonPropertyName("UplinkAddr")]  public string UplinkAddr  { get; set; } = "";
    [JsonPropertyName("Network")]     public string Network     { get; set; } = "";
}

// ── File areas ────────────────────────────────────────────────────────────────

public class FileDir
{
    [JsonPropertyName("ID")]          public long  ID          { get; set; }
    [JsonPropertyName("Name")]        public string Name        { get; set; } = "";
    [JsonPropertyName("Description")] public string Description { get; set; } = "";
    [JsonPropertyName("Path")]        public string Path        { get; set; } = "";
    [JsonPropertyName("SortType")]    public int    SortType    { get; set; }
    [JsonPropertyName("ReadSec")]     public int    ReadSec     { get; set; }
    [JsonPropertyName("UploadSec")]   public int    UploadSec   { get; set; }
    [JsonPropertyName("Active")]      public bool   Active      { get; set; } = true;
}

// ── Callers ───────────────────────────────────────────────────────────────────

public record CallerEntry
(
    [property: JsonPropertyName("timestamp")]     string Timestamp,
    [property: JsonPropertyName("user_name")]     string UserName,
    [property: JsonPropertyName("city")]          string City,
    [property: JsonPropertyName("remote_addr")]   string RemoteAddr,
    [property: JsonPropertyName("security_level")] int   SecurityLevel,
    [property: JsonPropertyName("node")]          int    Node,
    [property: JsonPropertyName("action")]        string Action,
    [property: JsonPropertyName("duration_secs")] int    DurationSecs
);

// ── Config ────────────────────────────────────────────────────────────────────

public class BbsConfig
{
    [JsonPropertyName("bbs")]     public BbsSection     Bbs     { get; set; } = new();
    [JsonPropertyName("network")] public NetworkSection Network { get; set; } = new();
    [JsonPropertyName("paths")]   public PathsSection   Paths   { get; set; } = new();
    [JsonPropertyName("session")] public SessionSection Session { get; set; } = new();
    [JsonPropertyName("sysop")]   public SysopSection   Sysop   { get; set; } = new();
    [JsonPropertyName("fido")]    public FidoSection    Fido    { get; set; } = new();
}

public class BbsSection
{
    [JsonPropertyName("name")]      public string Name     { get; set; } = "";
    [JsonPropertyName("max_nodes")] public int    MaxNodes { get; set; }
}

public class NetworkSection
{
    [JsonPropertyName("telnet_port")]   public int    TelnetPort   { get; set; }
    [JsonPropertyName("ssh_port")]      public int    SshPort      { get; set; }
    [JsonPropertyName("api_port")]      public int    ApiPort      { get; set; }
    [JsonPropertyName("api_bind")]      public string ApiBind      { get; set; } = "";
    [JsonPropertyName("userapi_port")]  public int    UserApiPort  { get; set; }
    [JsonPropertyName("userapi_bind")]  public string UserApiBind  { get; set; } = "";
    [JsonPropertyName("virtterm_port")] public int    VirtTermPort { get; set; }
    [JsonPropertyName("virtterm_bind")] public string VirtTermBind { get; set; } = "";
}

public class PathsSection
{
    [JsonPropertyName("db")]         public string Db        { get; set; } = "";
    [JsonPropertyName("files")]      public string Files     { get; set; } = "";
    [JsonPropertyName("logs")]       public string Logs      { get; set; } = "";
    [JsonPropertyName("caller_log")] public string CallerLog { get; set; } = "";
}

public class SessionSection
{
    [JsonPropertyName("time_per_call_mins")]  public int TimePerCallMins  { get; set; }
    [JsonPropertyName("idle_timeout_mins")]   public int IdleTimeoutMins  { get; set; }
    [JsonPropertyName("max_failed_logins")]   public int MaxFailedLogins  { get; set; }
    [JsonPropertyName("new_user_security")]   public int NewUserSecurity  { get; set; }
}

public class SysopSection
{
    [JsonPropertyName("name")]  public string Name  { get; set; } = "";
}

public class FidoSection
{
    [JsonPropertyName("enabled")]      public bool   Enabled     { get; set; }
    [JsonPropertyName("address")]      public string Address     { get; set; } = "";
    [JsonPropertyName("uplink")]       public string Uplink      { get; set; } = "";
    [JsonPropertyName("password")]     public string Password    { get; set; } = "";
    [JsonPropertyName("inbound_dir")]  public string InboundDir  { get; set; } = "";
    [JsonPropertyName("outbound_dir")] public string OutboundDir { get; set; } = "";
    [JsonPropertyName("nodelist_dir")] public string NodelistDir { get; set; } = "";
    [JsonPropertyName("binkp_port")]   public int    BinkpPort   { get; set; }
    [JsonPropertyName("areas")]        public Dictionary<string, int> Areas { get; set; } = new();
    [JsonPropertyName("akas")]         public List<string> AKAs { get; set; } = new();
    [JsonPropertyName("taglines_file")] public string TaglinesFile { get; set; } = "";
    [JsonPropertyName("downlinks")]    public List<FidoDownlink> Downlinks { get; set; } = new();
    [JsonPropertyName("areafix_password")] public string AreaFixPassword { get; set; } = "";
    [JsonPropertyName("poll_interval_mins")] public int PollIntervalMins { get; set; }
    [JsonPropertyName("file_areas")]   public Dictionary<string, int> FileAreas { get; set; } = new();
    [JsonPropertyName("filefix_password")] public string FileFixPassword { get; set; } = "";
    [JsonPropertyName("nodelist_url")]  public string NodelistURL { get; set; } = "";
    [JsonPropertyName("nodelist_update_interval_hours")] public int NodelistUpdateIntervalHours { get; set; }
    [JsonPropertyName("networks")]     public List<FidoNetworkDef> Networks { get; set; } = new();
}

public class FidoNetworkDef
{
    [JsonPropertyName("name")]        public string Name        { get; set; } = "";
    [JsonPropertyName("enabled")]       public bool   Enabled     { get; set; }
    [JsonPropertyName("address")]       public string Address     { get; set; } = "";
    [JsonPropertyName("uplink")]        public string Uplink      { get; set; } = "";
    [JsonPropertyName("password")]      public string Password    { get; set; } = "";
    [JsonPropertyName("inbound_dir")]   public string InboundDir  { get; set; } = "";
    [JsonPropertyName("outbound_dir")]  public string OutboundDir { get; set; } = "";
    [JsonPropertyName("nodelist_dir")]  public string NodelistDir { get; set; } = "";
    [JsonPropertyName("binkp_port")]    public int    BinkpPort   { get; set; }
    [JsonPropertyName("areas")]         public Dictionary<string, int> Areas { get; set; } = new();
    [JsonPropertyName("akas")]          public List<string> AKAs { get; set; } = new();
    [JsonPropertyName("taglines_file")] public string TaglinesFile { get; set; } = "";
    [JsonPropertyName("downlinks")]     public List<FidoDownlink> Downlinks { get; set; } = new();
    [JsonPropertyName("areafix_password")] public string AreaFixPassword { get; set; } = "";
    [JsonPropertyName("poll_interval_mins")] public int PollIntervalMins { get; set; }
    [JsonPropertyName("file_areas")]    public Dictionary<string, int> FileAreas { get; set; } = new();
    [JsonPropertyName("filefix_password")] public string FileFixPassword { get; set; } = "";
    [JsonPropertyName("nodelist_url")]  public string NodelistURL { get; set; } = "";
    [JsonPropertyName("nodelist_update_interval_hours")] public int NodelistUpdateIntervalHours { get; set; }
    [JsonPropertyName("nodelist_echo_tag")] public string NodelistEchoTag { get; set; } = "";
}

public class FidoDownlink
{
    [JsonPropertyName("name")]     public string Name     { get; set; } = "";
    [JsonPropertyName("address")]  public string Address  { get; set; } = "";
    [JsonPropertyName("password")] public string Password { get; set; } = "";
    [JsonPropertyName("akas")]     public List<string> AKAs { get; set; } = new();
}

public class FidoRoute
{
    [JsonPropertyName("ID")]        public long  ID        { get; set; }
    [JsonPropertyName("Network")]   public string Network   { get; set; } = "";
    [JsonPropertyName("Pattern")]   public string Pattern   { get; set; } = "";
    [JsonPropertyName("RouteTo")]   public string RouteTo   { get; set; } = "";
    [JsonPropertyName("IsDefault")] public bool  IsDefault { get; set; }
    [JsonPropertyName("CreatedAt")] public string CreatedAt { get; set; } = "";
}

public class FidoMember
{
    [JsonPropertyName("ID")]          public long  ID          { get; set; }
    [JsonPropertyName("Network")]     public string Network     { get; set; } = "";
    [JsonPropertyName("Zone")]        public int   Zone        { get; set; }
    [JsonPropertyName("Net")]         public int   Net         { get; set; }
    [JsonPropertyName("NodeNum")]     public int   NodeNum     { get; set; }
    [JsonPropertyName("Point")]       public int   Point       { get; set; }
    [JsonPropertyName("BBSName")]     public string BBSName     { get; set; } = "";
    [JsonPropertyName("SysopName")]   public string SysopName   { get; set; } = "";
    [JsonPropertyName("Location")]    public string Location    { get; set; } = "";
    [JsonPropertyName("Contact")]     public string Contact     { get; set; } = "";
    [JsonPropertyName("BinkpHost")]   public string BinkpHost   { get; set; } = "";
    [JsonPropertyName("Password")]    public string Password    { get; set; } = "";
    [JsonPropertyName("IsHost")]      public bool  IsHost      { get; set; }
    [JsonPropertyName("IsActive")]    public bool  IsActive    { get; set; }
    [JsonPropertyName("IsDelegated")] public bool  IsDelegated { get; set; }
    [JsonPropertyName("JoinedAt")]    public string JoinedAt    { get; set; } = "";

    public string Address => Point != 0
        ? $"{Zone}:{Net}/{NodeNum}.{Point}"
        : $"{Zone}:{Net}/{NodeNum}";
}

public class AreaFixSubscription
{
    [JsonPropertyName("Downlink")] public string Downlink { get; set; } = "";
    [JsonPropertyName("Name")]     public string Name     { get; set; } = "";
    [JsonPropertyName("Areas")]    public List<string> Areas { get; set; } = new();

    public string AreasText => string.Join(", ", Areas);
}

public class FidoJoinRequest
{
    [JsonPropertyName("ID")]                public long  ID                { get; set; }
    [JsonPropertyName("Network")]           public string Network           { get; set; } = "";
    [JsonPropertyName("BBSName")]           public string BBSName           { get; set; } = "";
    [JsonPropertyName("SysopName")]         public string SysopName         { get; set; } = "";
    [JsonPropertyName("Location")]          public string Location          { get; set; } = "";
    [JsonPropertyName("Contact")]           public string Contact           { get; set; } = "";
    [JsonPropertyName("RequestedNet")]      public int?   RequestedNet      { get; set; }
    [JsonPropertyName("BinkpHost")]         public string BinkpHost         { get; set; } = "";
    [JsonPropertyName("Status")]            public string Status            { get; set; } = "";
    [JsonPropertyName("CreatedAt")]         public string CreatedAt         { get; set; } = "";
}

public class TextExport
{
    [JsonPropertyName("text")] public string Text { get; set; } = "";
}

public class JoinApproveResult
{
    [JsonPropertyName("member")]   public FidoMember Member   { get; set; } = new();
    [JsonPropertyName("password")] public string Password { get; set; } = "";
}

public class RoutingImportResult
{
    [JsonPropertyName("Updated")] public int Updated { get; set; }
    [JsonPropertyName("Unknown")] public List<string> Unknown { get; set; } = new();
    [JsonPropertyName("Errors")]  public List<string> Errors  { get; set; } = new();
}

public class RoutesImportResult
{
    [JsonPropertyName("Added")]  public int Added  { get; set; }
    [JsonPropertyName("Errors")] public List<string> Errors { get; set; } = new();
}

public class PktResult
{
    [JsonPropertyName("pkt")] public string Pkt { get; set; } = "";
}

// ── FidoNet ───────────────────────────────────────────────────────────────────

public record FidoNode
(
    [property: JsonPropertyName("network")]   string Network,
    [property: JsonPropertyName("zone")]      int    Zone,
    [property: JsonPropertyName("net")]       int    Net,
    [property: JsonPropertyName("node")]      int    Node,
    [property: JsonPropertyName("point")]     int    Point,
    [property: JsonPropertyName("name")]      string Name,
    [property: JsonPropertyName("location")]  string Location,
    [property: JsonPropertyName("sysop")]     string Sysop,
    [property: JsonPropertyName("phone")]     string Phone,
    [property: JsonPropertyName("baud")]      int    Baud,
    [property: JsonPropertyName("flags")]     string Flags,
    [property: JsonPropertyName("type")]      string Type,
    [property: JsonPropertyName("active")]    bool   Active
)
{
    public string Address => Point != 0
        ? $"{Zone}:{Net}/{Node}.{Point}"
        : $"{Zone}:{Net}/{Node}";
}

public record NodelistSearchResult
(
    [property: JsonPropertyName("nodes")]  List<FidoNode> Nodes,
    [property: JsonPropertyName("total")]  int Total,
    [property: JsonPropertyName("page")]   int Page,
    [property: JsonPropertyName("pages")]  int Pages
);

public record NodelistVersion
(
    [property: JsonPropertyName("network")]     string Network,
    [property: JsonPropertyName("imported_at")] string ImportedAt,
    [property: JsonPropertyName("node_count")]  int    NodeCount
);

// ── API tokens (VirtAnd/VirtTerm device tokens, sysop administration) ────────

public class ApiToken
{
    [JsonPropertyName("ID")]          public long   ID          { get; set; }
    [JsonPropertyName("UserID")]      public long   UserID      { get; set; }
    [JsonPropertyName("UserName")]    public string UserName    { get; set; } = "";
    [JsonPropertyName("DeviceLabel")] public string DeviceLabel { get; set; } = "";
    [JsonPropertyName("CreatedAt")]   public string CreatedAt   { get; set; } = "";
    [JsonPropertyName("RevokedAt")]   public string RevokedAt   { get; set; } = "";

    public bool IsActive => string.IsNullOrEmpty(RevokedAt);
}
