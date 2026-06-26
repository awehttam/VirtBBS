// VirtTermMac — TerminalConnection.cs
//
// Owns the TLS socket to VirtBBS's internal/virtterm listener — the live
// 80x25 terminal pane's transport. Runs a dedicated background read thread
// that feeds bytes straight into an AnsiScreen; critically, that thread
// NEVER touches WinForms controls directly (AnsiScreen.Feed only mutates
// its own cell grid and raises an event — TerminalControl marshals its own
// repaint via Control.Invoke). Writes (keystrokes) are synchronous sends on
// the calling thread, which is fine since they're tiny and infrequent.
//
// Server certificates are self-signed (internal/virtterm generates one on
// first run) — there's no CA to validate against, so the validation
// callback accepts any certificate. This is the same trust model as SSH's
// host-key-on-first-connect; a future version could pin the cert fingerprint
// after the first successful connect, but that's out of scope for Phase 2/3.
using System;
using System.Net.Security;
using System.Net.Sockets;
using System.Security.Cryptography.X509Certificates;
using System.Threading;
using VirtTermMac.Terminal;

namespace VirtTermMac.Net;

public class TerminalConnection : IDisposable
{
    private readonly AnsiScreen _screen;
    private TcpClient? _tcp;
    private SslStream? _ssl;
    private Thread? _readThread;
    private volatile bool _running;

    public bool IsConnected => _ssl != null && _tcp is { Connected: true };

    public event Action? Disconnected;
    public event Action<Exception>? ConnectionError;

    public TerminalConnection(AnsiScreen screen)
    {
        _screen = screen;
    }

    public void Connect(string host, int port)
    {
        Disconnect();

        _tcp = new TcpClient { NoDelay = true };
        _tcp.Connect(host, port);

        // Self-signed cert, no CA — accept unconditionally (see file header).
        _ssl = new SslStream(_tcp.GetStream(), false, (_, _, _, _) => true);
        _ssl.AuthenticateAsClient(host);

        _running = true;
        _readThread = new Thread(ReadLoop) { IsBackground = true, Name = "VirtTermMac-ReadLoop" };
        _readThread.Start();
    }

    private void ReadLoop()
    {
        var buf = new byte[4096];
        try
        {
            while (_running)
            {
                int n = _ssl!.Read(buf, 0, buf.Length);
                if (n == 0) break; // remote closed
                _screen.Feed(buf, n); // AnsiScreen.Feed only mutates its own state + raises an event
            }
        }
        catch (Exception ex)
        {
            if (_running) ConnectionError?.Invoke(ex);
        }
        finally
        {
            _running = false;
            Disconnected?.Invoke();
        }
    }

    /// <summary>Sends raw bytes (typed keystrokes, or a menu-generated single keystroke) as-is.</summary>
    public void Send(byte[] data)
    {
        if (_ssl == null) return;
        try { _ssl.Write(data, 0, data.Length); }
        catch (Exception ex) { ConnectionError?.Invoke(ex); }
    }

    public void Disconnect()
    {
        _running = false;
        try { _ssl?.Close(); } catch { /* ignore */ }
        try { _tcp?.Close(); } catch { /* ignore */ }
        _ssl = null;
        _tcp = null;
    }

    public void Dispose() => Disconnect();
}
