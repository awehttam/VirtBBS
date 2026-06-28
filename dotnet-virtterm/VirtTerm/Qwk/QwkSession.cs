namespace VirtTerm.Qwk;

/// <summary>
/// In-memory offline mail session — load a QWK packet, browse, queue replies, export REP.
/// </summary>
public sealed class QwkSession
{
    public QwkControlInfo Control { get; private set; } = new();
    public List<QwkMessage> Messages { get; } = [];
    public List<QwkReply> PendingReplies { get; } = [];
    public string? SourcePath { get; private set; }

    public void LoadFromFile(string path)
    {
        var bytes = File.ReadAllBytes(path);
        LoadFromBytes(bytes, path);
    }

    public void LoadFromBytes(byte[] zipBytes, string? sourcePath = null)
    {
        SourcePath = sourcePath;
        Messages.Clear();
        PendingReplies.Clear();

        var controlDat = QwkPacket.ReadZipEntry(zipBytes, "CONTROL.DAT");
        Control = QwkPacket.ParseControlDat(controlDat);
        Messages.AddRange(QwkPacket.ParseQwkPacket(zipBytes));
    }

    public IEnumerable<QwkConference> Conferences()
    {
        var ids = Messages.Select(m => m.ConferenceId).Distinct().OrderBy(id => id);
        foreach (var id in ids)
        {
            var name = Control.ConferenceNames.GetValueOrDefault(id, $"Conference {id}");
            var count = Messages.Count(m => m.ConferenceId == id);
            yield return new QwkConference(id, name, count);
        }
    }

    public IEnumerable<QwkMessage> MessagesInConference(int conferenceId) =>
        Messages.Where(m => m.ConferenceId == conferenceId).OrderByDescending(m => m.MsgNumber);

    public void MarkRead(QwkMessage message)
    {
        var idx = Messages.FindIndex(m =>
            m.ConferenceId == message.ConferenceId && m.MsgNumber == message.MsgNumber);
        if (idx >= 0)
            Messages[idx] = message with { IsRead = true };
    }

    public void QueueReply(QwkReply reply) => PendingReplies.Add(reply);

    public void RemoveReply(QwkReply reply) => PendingReplies.Remove(reply);

    public string DefaultFromName =>
        string.IsNullOrWhiteSpace(Control.CallerName) ? "User" : Control.CallerName;

    public byte[] BuildRepBytes() => QwkPacket.BuildRepPacket(PendingReplies);

    public void ClearRepliesAfterExport() => PendingReplies.Clear();
}
