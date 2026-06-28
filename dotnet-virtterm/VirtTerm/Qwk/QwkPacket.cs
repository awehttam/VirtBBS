using System.IO.Compression;
using System.Text;

namespace VirtTerm.Qwk;

/// <summary>
/// Parses QWK download packets and builds REP upload packets, matching
/// VirtBBS internal/qwk and VirtAnd core/QwkPacket.kt byte-for-byte.
/// </summary>
public static class QwkPacket
{
    private const int BlockSize = 128;
    private const byte SoftCr = 0xE3;
    private static readonly Encoding RawBytes = Encoding.Latin1;

    public static QwkControlInfo ParseControlDat(byte[]? controlDat)
    {
        var info = new QwkControlInfo();
        if (controlDat == null || controlDat.Length == 0) return info;

        var lines = RawBytes.GetString(controlDat)
            .Replace("\r\n", "\n")
            .Split('\n', StringSplitOptions.None);

        if (lines.Length > 0) info.BbsName = lines[0].Trim();
        if (lines.Length > 1) info.CityState = lines[1].Trim();
        if (lines.Length > 2) info.BbsPhone = lines[2].Trim();
        if (lines.Length > 3) info.SysopName = lines[3].Trim();
        if (lines.Length > 6) info.CallerName = lines[6].Trim();
        if (lines.Length > 8 && int.TryParse(lines[8].Trim(), out var total))
            info.TotalMessages = total;

        for (var i = 10; i + 1 < lines.Length; i++)
        {
            var idLine = lines[i].Trim();
            if (idLine == "0" || string.IsNullOrEmpty(idLine)) break;
            if (!int.TryParse(idLine, out var confId)) continue;
            info.ConferenceNames[confId] = lines[i + 1].Trim();
            i++;
        }

        return info;
    }

    public static List<QwkMessage> ParseQwkPacket(byte[] zipBytes)
    {
        var messagesDat = ReadZipEntry(zipBytes, "MESSAGES.DAT")
            ?? throw new InvalidDataException("QWK packet has no MESSAGES.DAT");

        var outList = new List<QwkMessage>();
        var offset = BlockSize;
        while (offset + BlockSize <= messagesDat.Length)
        {
            var header = DecodeHeader(messagesDat, offset);
            var bodyStart = offset + BlockSize;
            var bodyBlocks = Math.Max(header.NumBlocks - 1, 0);
            var bodyEnd = Math.Min(bodyStart + bodyBlocks * BlockSize, messagesDat.Length);

            outList.Add(new QwkMessage(
                header.ConfNum,
                header.MsgNum,
                header.Date,
                header.Time,
                header.To,
                header.From,
                header.Subject,
                DecodeBody(messagesDat, bodyStart, bodyEnd)));

            var totalBlocks = Math.Max(header.NumBlocks, 1);
            offset += totalBlocks * BlockSize;
        }

        return outList;
    }

    public static byte[] BuildRepPacket(IReadOnlyList<QwkReply> replies)
    {
        using var ms = new MemoryStream();
        using (var zip = new ZipArchive(ms, ZipArchiveMode.Create, leaveOpen: true))
        {
            for (var i = 0; i < replies.Count; i++)
            {
                var r = replies[i];
                var entry = zip.CreateEntry($"{i + 1}.MSG");
                using var writer = new StreamWriter(entry.Open(), Encoding.ASCII);
                writer.Write($"{r.ConferenceId}\r\n");
                writer.Write($"{r.RefNum}\r\n");
                writer.Write($"{r.ToName}\r\n");
                writer.Write($"{r.FromName}\r\n");
                writer.Write($"{r.Subject}\r\n");
                writer.Write("\r\n");
                writer.Write(r.Body);
            }
        }

        return ms.ToArray();
    }

    public static byte[]? ReadZipEntry(byte[] zipBytes, string name)
    {
        using var ms = new MemoryStream(zipBytes);
        using var zip = new ZipArchive(ms, ZipArchiveMode.Read);
        var entry = zip.Entries.FirstOrDefault(e =>
            e.Name.Equals(name, StringComparison.OrdinalIgnoreCase));
        if (entry == null) return null;
        using var stream = entry.Open();
        using var outMs = new MemoryStream();
        stream.CopyTo(outMs);
        return outMs.ToArray();
    }

    private sealed class MessageHeader
    {
        public int MsgNum { get; init; }
        public string Date { get; init; } = "";
        public string Time { get; init; } = "";
        public string To { get; init; } = "";
        public string From { get; init; } = "";
        public string Subject { get; init; } = "";
        public int NumBlocks { get; init; }
        public int ConfNum { get; init; }
    }

    private static MessageHeader DecodeHeader(byte[] data, int offset)
    {
        string Field(int start, int len) =>
            RawBytes.GetString(data, offset + start, len).Trim();

        return new MessageHeader
        {
            MsgNum = int.TryParse(Field(1, 7), out var n) ? n : 0,
            Date = Field(8, 8),
            Time = Field(16, 5),
            To = Field(21, 25),
            From = Field(46, 25),
            Subject = Field(71, 25),
            NumBlocks = int.TryParse(Field(116, 2), out var b) ? b : 1,
            ConfNum = int.TryParse(Field(119, 7), out var c) ? c : 0,
        };
    }

    private static string DecodeBody(byte[] data, int start, int end)
    {
        var len = Math.Max(end - start, 0);
        var raw = RawBytes.GetString(data, start, len);
        return raw.Replace((char)SoftCr + "", "\r\n").TrimEnd(' ');
    }
}
