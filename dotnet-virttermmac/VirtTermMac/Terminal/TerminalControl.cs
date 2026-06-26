// VirtTermMac — TerminalControl.cs
//
// Avalonia port of dotnet-virtterm/VirtTerm's WinForms TerminalControl —
// same AnsiScreen model, same CP437/ANSI rendering intent, just drawn via
// Avalonia's DrawingContext instead of System.Drawing/GDI+, and using
// Avalonia's input events instead of WinForms' OnKeyDown/OnKeyPress.
//
// For best fidelity, install a real DOS-VGA font such as "Px437 IBM VGA8"
// or "Perfect DOS VGA 437"; this falls back to the platform's standard
// monospace font (Menlo on macOS, Consolas on Windows, a generic
// monospace elsewhere) when none of those are installed.
using System;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Media;
using Avalonia.Threading;

namespace VirtTermMac.Terminal;

public class TerminalControl : Control
{
    private static readonly FontFamily PreferredFontFamily =
        new("Px437 IBM VGA8,Perfect DOS VGA 437,Menlo,Consolas,monospace");

    private readonly AnsiScreen _screen;
    private readonly Typeface _typeface;
    private readonly double _fontSize = 16.0;
    private Size _cellSize;

    // Classic 16-color ANSI palette (matches SyncTerm/PCBoard conventions).
    private static readonly IBrush[] Palette =
    {
        new SolidColorBrush(Color.FromRgb(0, 0, 0)),       new SolidColorBrush(Color.FromRgb(170, 0, 0)),
        new SolidColorBrush(Color.FromRgb(0, 170, 0)),     new SolidColorBrush(Color.FromRgb(170, 85, 0)),
        new SolidColorBrush(Color.FromRgb(0, 0, 170)),     new SolidColorBrush(Color.FromRgb(170, 0, 170)),
        new SolidColorBrush(Color.FromRgb(0, 170, 170)),   new SolidColorBrush(Color.FromRgb(170, 170, 170)),
        new SolidColorBrush(Color.FromRgb(85, 85, 85)),    new SolidColorBrush(Color.FromRgb(255, 85, 85)),
        new SolidColorBrush(Color.FromRgb(85, 255, 85)),   new SolidColorBrush(Color.FromRgb(255, 255, 85)),
        new SolidColorBrush(Color.FromRgb(85, 85, 255)),   new SolidColorBrush(Color.FromRgb(255, 85, 255)),
        new SolidColorBrush(Color.FromRgb(85, 255, 255)),  new SolidColorBrush(Color.FromRgb(255, 255, 255)),
    };

    private static readonly IBrush CursorBrush = new SolidColorBrush(Colors.White, 0.47);

    /// <summary>Raised for every keystroke typed while focused — raw bytes to send as-is.</summary>
    public event Action<byte[]>? KeyInput;

    public TerminalControl(AnsiScreen screen)
    {
        _screen = screen;
        _screen.Changed += () => Dispatcher.UIThread.Post(InvalidateVisual);

        Focusable = true;
        _typeface = new Typeface(PreferredFontFamily);

        // Measure a full block glyph to get a stable monospace cell size.
        var probe = new FormattedText("█", System.Globalization.CultureInfo.CurrentCulture,
            FlowDirection.LeftToRight, _typeface, _fontSize, Brushes.White);
        _cellSize = new Size(Math.Ceiling(probe.Width), Math.Ceiling(probe.Height));
    }

    protected override Size MeasureOverride(Size availableSize) =>
        new(_cellSize.Width * AnsiScreen.Cols, _cellSize.Height * AnsiScreen.Rows);

    public override void Render(DrawingContext context)
    {
        context.FillRectangle(Brushes.Black, new Rect(Bounds.Size));

        for (int r = 0; r < AnsiScreen.Rows; r++)
        {
            for (int c = 0; c < AnsiScreen.Cols; c++)
            {
                var cell = _screen.GetCell(r, c);
                double x = c * _cellSize.Width;
                double y = r * _cellSize.Height;

                var bg = Palette[cell.Bg & 0x07];
                if (!ReferenceEquals(bg, Palette[0]))
                    context.FillRectangle(bg, new Rect(x, y, _cellSize.Width, _cellSize.Height));

                if (cell.Ch != ' ')
                {
                    var fg = Palette[cell.Fg & 0x0F];
                    var ft = new FormattedText(cell.Ch.ToString(), System.Globalization.CultureInfo.CurrentCulture,
                        FlowDirection.LeftToRight, _typeface, _fontSize, fg);
                    context.DrawText(ft, new Point(x, y));
                }
            }
        }

        double cx = _screen.CursorCol * _cellSize.Width;
        double cy = _screen.CursorRow * _cellSize.Height;
        context.FillRectangle(CursorBrush, new Rect(cx, cy, _cellSize.Width, _cellSize.Height));
    }

    protected override void OnPointerPressed(PointerPressedEventArgs e)
    {
        Focus();
        base.OnPointerPressed(e);
    }

    // Arrows and the non-textual control keys (Enter/Backspace/Tab/Escape) are
    // handled here — they produce no OnTextInput. Printable characters arrive
    // via OnTextInput below; handling both here AND there would double-send.
    protected override void OnKeyDown(KeyEventArgs e)
    {
        byte[]? seq = e.Key switch
        {
            Key.Up => new byte[] { 0x1B, (byte)'[', (byte)'A' },
            Key.Down => new byte[] { 0x1B, (byte)'[', (byte)'B' },
            Key.Right => new byte[] { 0x1B, (byte)'[', (byte)'C' },
            Key.Left => new byte[] { 0x1B, (byte)'[', (byte)'D' },
            Key.Enter => new byte[] { (byte)'\r' },
            Key.Back => new byte[] { 0x08 },
            Key.Tab => new byte[] { 0x09 },
            Key.Escape => new byte[] { 0x1B },
            _ => null,
        };
        if (seq != null)
        {
            KeyInput?.Invoke(seq);
            e.Handled = true;
        }
    }

    protected override void OnTextInput(TextInputEventArgs e)
    {
        if (!string.IsNullOrEmpty(e.Text))
        {
            var bytes = new byte[e.Text.Length];
            for (int i = 0; i < e.Text.Length; i++) bytes[i] = (byte)e.Text[i];
            KeyInput?.Invoke(bytes);
        }
        e.Handled = true;
    }
}
