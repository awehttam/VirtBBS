namespace VirtTerm.Qwk;

public sealed record QwkMessage(
    int ConferenceId,
    int MsgNumber,
    string Date,
    string Time,
    string ToName,
    string FromName,
    string Subject,
    string Body,
    bool IsRead = false)
{
    public override string ToString()
    {
        var mark = IsRead ? "" : "* ";
        return $"{mark}#{MsgNumber} {FromName}: {Subject}";
    }
}

public sealed record QwkReply(
    int ConferenceId,
    int RefNum,
    string ToName,
    string FromName,
    string Subject,
    string Body);

public sealed record QwkConference(int Id, string Name, int MessageCount)
{
    public override string ToString() => $"{Name} ({MessageCount})";
}

public sealed class QwkControlInfo
{
    public string BbsName { get; set; } = "";
    public string CityState { get; set; } = "";
    public string BbsPhone { get; set; } = "";
    public string SysopName { get; set; } = "";
    public string CallerName { get; set; } = "";
    public int TotalMessages { get; set; }
    public Dictionary<int, string> ConferenceNames { get; } = new();
}
