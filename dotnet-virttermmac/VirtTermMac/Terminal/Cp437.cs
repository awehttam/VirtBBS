// VirtTermMac — Cp437.cs
//
// Full IBM Code Page 437 -> Unicode mapping, used to render the byte stream
// coming from VirtBBS (box-drawing, block elements, line art) the same way
// a classic ANSI terminal (SyncTerm, NetRunner) would. Bytes 0x20-0x7E map
// directly to ASCII; 0x80-0xFF map to their CP437 glyphs below.
//
// Counterpart of the server's internal/ansi/cp437.go (which goes the other
// direction, Unicode -> CP437, for outbound display files) — this table
// must stay in sync with that one's glyph choices for the characters they
// both cover (box-drawing, block elements).

namespace VirtTermMac.Terminal;

public static class Cp437
{
    // Index 0-255 -> Unicode character. 0x00-0x1F are control codes that
    // should never reach the renderer directly (the ANSI parser consumes
    // them), but are mapped to space as a safe fallback.
    private static readonly char[] Table = BuildTable();

    public static char ToChar(byte b) => Table[b];

    private static char[] BuildTable()
    {
        var t = new char[256];

        // 0x00-0x1F: control codes — not normally rendered, map to space.
        for (int i = 0; i < 0x20; i++) t[i] = ' ';

        // 0x20-0x7E: identical to ASCII.
        for (int i = 0x20; i <= 0x7E; i++) t[i] = (char)i;

        t[0x7F] = '⌂'; // DEL is conventionally the "house" glyph in CP437 fonts

        // 0x80-0x9F
        string hi1 = "ÇüéâäàåçêëèïîìÄÅÉæÆôöòûùÿÖÜ¢£¥₧ƒ";
        for (int i = 0; i < hi1.Length; i++) t[0x80 + i] = hi1[i];

        // 0xA0-0xBF
        string hi2 = "áíóúñÑªº¿⌐¬½¼¡«»░▒▓│┤╡╢╖╕╣║╗╝╜╛┐";
        for (int i = 0; i < hi2.Length; i++) t[0xA0 + i] = hi2[i];

        // 0xC0-0xDF
        string hi3 = "└┴┬├─┼╞╟╚╔╩╦╠═╬╧╨╤╥╙╘╒╓╫╪┘┌█▄▌▐▀";
        for (int i = 0; i < hi3.Length; i++) t[0xC0 + i] = hi3[i];

        // 0xE0-0xFF
        string hi4 = "αßΓπΣσµτΦΘΩδ∞φε∩≡±≥≤⌠⌡÷≈°∙·√ⁿ²■ ";
        for (int i = 0; i < hi4.Length; i++) t[0xE0 + i] = hi4[i];

        return t;
    }
}
