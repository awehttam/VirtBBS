// VirtTermMac — AnsiScreen.cs
//
// An 80x25 character-cell screen buffer plus a small ANSI/VT100 escape-code
// state machine, fed raw bytes exactly as they arrive from the VirtTermMac TLS
// connection (CP437 text + ANSI escape sequences — the same byte stream a
// Telnet client like SyncTerm would receive, since internal/virtterm hands
// connections to the unmodified session.Run()).
//
// VirtBBS's screen layout is hard-baked to 80x24/80x25 throughout
// internal/session/session.go (banner widths, listing columns) — there is
// no resize negotiation to honor, so this grid is intentionally fixed-size.
//
// Also tracks a small rolling tail of raw incoming bytes so MainForm's
// dynamic menu can cheaply detect "are we sitting at the main Command:
// prompt right now" via a literal substring check — not full ANSI/state
// parsing — per the plan's explicit "remote control" design (clicking a
// menu macro while mid-flow at some other prompt would inject the keystroke
// into the wrong field, so menu items only act when this is true).
using System;
using System.Text;

namespace VirtTermMac.Terminal;

public struct Cell
{
    public char Ch;
    public byte Fg; // 0-15 (bit 3 = bright/bold)
    public byte Bg; // 0-7
    public bool Reverse;

    public static Cell Blank => new() { Ch = ' ', Fg = 7, Bg = 0 };
}

public class AnsiScreen
{
    public const int Cols = 80;
    public const int Rows = 25;

    private readonly Cell[,] _grid = new Cell[Rows, Cols];
    private int _cursorRow;
    private int _cursorCol;

    private byte _curFg = 7;
    private byte _curBg = 0;
    private bool _bold;
    private bool _reverse;

    // Escape-sequence parse state.
    private enum ParseState { Normal, Esc, Csi }
    private ParseState _state = ParseState.Normal;
    private readonly StringBuilder _csiParams = new();

    // Saved cursor position for CSI s / CSI u.
    private int _savedRow, _savedCol;

    // Rolling tail of the last raw bytes seen, ANSI sequences included, for
    // the "Command: " prompt-detection substring check described above.
    private readonly byte[] _tail = new byte[64];
    private int _tailLen;

    public int CursorRow => _cursorRow;
    public int CursorCol => _cursorCol;

    /// <summary>Raised whenever the screen contents change, so the UI can repaint.</summary>
    public event Action? Changed;

    /// <summary>
    /// True once the rolling byte tail ends with the literal main-menu
    /// prompt string VirtBBS's mainMenu() prints ("\r\nCommand: ").
    /// </summary>
    public bool IsAtCommandPrompt { get; private set; }

    public AnsiScreen()
    {
        Clear();
    }

    public Cell GetCell(int row, int col) => _grid[row, col];

    public void Clear()
    {
        for (int r = 0; r < Rows; r++)
            for (int c = 0; c < Cols; c++)
                _grid[r, c] = Cell.Blank;
        _cursorRow = 0;
        _cursorCol = 0;
        Changed?.Invoke();
    }

    /// <summary>Feeds a chunk of raw bytes from the socket into the screen buffer.</summary>
    public void Feed(byte[] data, int count)
    {
        for (int i = 0; i < count; i++)
        {
            byte b = data[i];
            UpdateTail(b);
            ProcessByte(b);
        }
        Changed?.Invoke();
    }

    private void UpdateTail(byte b)
    {
        if (_tailLen < _tail.Length)
        {
            _tail[_tailLen++] = b;
        }
        else
        {
            Array.Copy(_tail, 1, _tail, 0, _tail.Length - 1);
            _tail[_tail.Length - 1] = b;
        }

        const string marker = "\r\nCommand: ";
        if (_tailLen < marker.Length) { IsAtCommandPrompt = false; return; }

        // Search anywhere in the rolling tail, not just at the very end.
        // ansi.Prompt() on the server always appends a trailing ANSI reset
        // sequence ("\x1b[0m") right after the marker text, and Changed only
        // fires once per Feed() call (after the whole chunk, including that
        // trailing reset, has been processed) — an exact suffix match would
        // therefore always see the marker already pushed out of the tail end
        // by those few extra bytes, making this permanently false.
        bool found = false;
        for (int start = 0; start <= _tailLen - marker.Length; start++)
        {
            bool match = true;
            for (int j = 0; j < marker.Length; j++)
            {
                if (_tail[start + j] != (byte)marker[j]) { match = false; break; }
            }
            if (match) { found = true; break; }
        }
        IsAtCommandPrompt = found;
    }

    private void ProcessByte(byte b)
    {
        switch (_state)
        {
            case ParseState.Normal:
                ProcessNormalByte(b);
                return;

            case ParseState.Esc:
                if (b == (byte)'[') { _state = ParseState.Csi; _csiParams.Clear(); return; }
                _state = ParseState.Normal; // unsupported escape — drop it
                return;

            case ParseState.Csi:
                if (b >= 0x30 && b <= 0x3F) { _csiParams.Append((char)b); return; } // digits, ';', etc.
                ExecuteCsi((char)b, _csiParams.ToString());
                _state = ParseState.Normal;
                return;
        }
    }

    private void ProcessNormalByte(byte b)
    {
        switch (b)
        {
            case 0x1B: // ESC
                _state = ParseState.Esc;
                return;
            case (byte)'\r':
                _cursorCol = 0;
                return;
            case (byte)'\n':
                LineFeed();
                return;
            case 0x08: // backspace
                if (_cursorCol > 0) _cursorCol--;
                return;
            case 0x09: // tab
                _cursorCol = Math.Min(Cols - 1, (_cursorCol / 8 + 1) * 8);
                return;
            case 0x07: // bell
                return;
            default:
                PutChar(Cp437.ToChar(b));
                return;
        }
    }

    private void PutChar(char ch)
    {
        var (fg, bg) = _reverse ? (_curBg, (byte)(_curFg & 0x07)) : (_curFg, _curBg);
        _grid[_cursorRow, _cursorCol] = new Cell { Ch = ch, Fg = fg, Bg = bg };
        _cursorCol++;
        if (_cursorCol >= Cols)
        {
            _cursorCol = 0;
            LineFeed();
        }
    }

    private void LineFeed()
    {
        if (_cursorRow < Rows - 1)
        {
            _cursorRow++;
            return;
        }
        // Scroll the whole buffer up one row.
        for (int r = 0; r < Rows - 1; r++)
            for (int c = 0; c < Cols; c++)
                _grid[r, c] = _grid[r + 1, c];
        for (int c = 0; c < Cols; c++)
            _grid[Rows - 1, c] = Cell.Blank;
    }

    private void ExecuteCsi(char final, string paramsStr)
    {
        var parts = paramsStr.Split(';');
        int P(int idx, int def = 0)
        {
            if (idx >= parts.Length || parts[idx].Length == 0) return def;
            return int.TryParse(parts[idx], out var v) ? v : def;
        }

        switch (final)
        {
            case 'H':
            case 'f':
                _cursorRow = Math.Clamp(P(0, 1) - 1, 0, Rows - 1);
                _cursorCol = Math.Clamp(P(1, 1) - 1, 0, Cols - 1);
                break;
            case 'A':
                _cursorRow = Math.Max(0, _cursorRow - P(0, 1));
                break;
            case 'B':
                _cursorRow = Math.Min(Rows - 1, _cursorRow + P(0, 1));
                break;
            case 'C':
                _cursorCol = Math.Min(Cols - 1, _cursorCol + P(0, 1));
                break;
            case 'D':
                _cursorCol = Math.Max(0, _cursorCol - P(0, 1));
                break;
            case 'J':
                EraseDisplay(P(0, 0));
                break;
            case 'K':
                EraseLine(P(0, 0));
                break;
            case 's':
                _savedRow = _cursorRow; _savedCol = _cursorCol;
                break;
            case 'u':
                _cursorRow = _savedRow; _cursorCol = _savedCol;
                break;
            case 'm':
                ApplySgr(parts);
                break;
            default:
                // Unsupported CSI final byte — ignore.
                break;
        }
    }

    private void EraseDisplay(int mode)
    {
        switch (mode)
        {
            case 2: // entire screen
                Clear();
                break;
            case 1: // start to cursor
                for (int r = 0; r < _cursorRow; r++)
                    for (int c = 0; c < Cols; c++) _grid[r, c] = Cell.Blank;
                for (int c = 0; c <= _cursorCol; c++) _grid[_cursorRow, c] = Cell.Blank;
                break;
            default: // 0: cursor to end
                for (int c = _cursorCol; c < Cols; c++) _grid[_cursorRow, c] = Cell.Blank;
                for (int r = _cursorRow + 1; r < Rows; r++)
                    for (int c = 0; c < Cols; c++) _grid[r, c] = Cell.Blank;
                break;
        }
    }

    private void EraseLine(int mode)
    {
        switch (mode)
        {
            case 2:
                for (int c = 0; c < Cols; c++) _grid[_cursorRow, c] = Cell.Blank;
                break;
            case 1:
                for (int c = 0; c <= _cursorCol; c++) _grid[_cursorRow, c] = Cell.Blank;
                break;
            default:
                for (int c = _cursorCol; c < Cols; c++) _grid[_cursorRow, c] = Cell.Blank;
                break;
        }
    }

    private void ApplySgr(string[] parts)
    {
        if (parts.Length == 1 && parts[0].Length == 0)
        {
            ResetAttrs();
            return;
        }
        foreach (var p in parts)
        {
            if (!int.TryParse(p, out int code)) continue;
            switch (code)
            {
                case 0: ResetAttrs(); break;
                case 1: _bold = true; break;
                case 7: _reverse = true; break;
                case 22: _bold = false; break;
                case 27: _reverse = false; break;
                default:
                    if (code >= 30 && code <= 37) _curFg = (byte)((code - 30) | (_bold ? 0x08 : 0));
                    else if (code >= 40 && code <= 47) _curBg = (byte)(code - 40);
                    else if (code >= 90 && code <= 97) _curFg = (byte)(code - 90 + 8);
                    else if (code >= 100 && code <= 107) _curBg = (byte)(code - 100);
                    break;
            }
        }
        if (_bold) _curFg = (byte)(_curFg | 0x08);
    }

    private void ResetAttrs()
    {
        _curFg = 7;
        _curBg = 0;
        _bold = false;
        _reverse = false;
    }
}
