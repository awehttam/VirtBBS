using System.Text;
using Avalonia.Controls;
using Avalonia.Interactivity;
using Avalonia.Platform.Storage;
using VirtTermMac.Net;
using VirtTermMac.Qwk;
using VirtTermMac.Settings;

namespace VirtTermMac.Views;

public partial class OfflineMailWindow : Window
{
    private readonly QwkSession _session = new();
    private readonly AppSettings? _settings;
    private QwkMessage? _selectedMessage;
    private int? _selectedConferenceId;
    private bool _updatingMessageList;

    public OfflineMailWindow() : this(null) { }

    public OfflineMailWindow(AppSettings? settings)
    {
        _settings = settings;
        InitializeComponent();
        WireEvents();
        UpdateUiState();
        SetStatus("Open a QWK packet to begin. No BBS connection required.");
    }

    private void WireEvents()
    {
        OpenQwkItem.Click += (_, _) => _ = OpenQwkAsync();
        SaveRepItem.Click += (_, _) => _ = SaveRepAsync();
        DownloadBbsItem.Click += (_, _) => _ = DownloadFromBbsAsync();
        UploadBbsItem.Click += (_, _) => _ = UploadToBbsAsync();
        CloseItem.Click += (_, _) => Close();
        NewMessageItem.Click += (_, _) => ComposeNew();
        ReplyItem.Click += (_, _) => ComposeReply();
        ViewQueueItem.Click += (_, _) => ShowQueue();
        ReplyButton.Click += (_, _) => ComposeReply();
        NewButton.Click += (_, _) => ComposeNew();
        ConferenceList.SelectionChanged += (_, _) => OnConferenceSelected();
        MessageList.SelectionChanged += (_, _) => OnMessageSelected();
    }

    private void OnConferenceSelected()
    {
        if (ConferenceList.SelectedItem is not QwkConference conf)
        {
            _selectedConferenceId = null;
            MessageList.ItemsSource = null;
            return;
        }

        _selectedConferenceId = conf.Id;
        MessageHeader.Text = $"Messages — {conf.Name}";
        _updatingMessageList = true;
        try
        {
            MessageList.ItemsSource = _session.MessagesInConference(conf.Id).ToList();
        }
        finally
        {
            _updatingMessageList = false;
        }
        ClearDetail();
    }

    private void OnMessageSelected()
    {
        if (_updatingMessageList) return;
        if (MessageList.SelectedItem is not QwkMessage msg) return;

        _session.MarkRead(msg);
        _selectedMessage = _session.MessagesInConference(msg.ConferenceId)
            .FirstOrDefault(m => m.MsgNumber == msg.MsgNumber) ?? msg;

        DetailFrom.Text = $"From: {_selectedMessage.FromName}";
        DetailTo.Text = $"To: {_selectedMessage.ToName}";
        DetailSubject.Text = _selectedMessage.Subject;
        DetailDate.Text = $"{_selectedMessage.Date} {_selectedMessage.Time}  (#{_selectedMessage.MsgNumber})";
        DetailBody.Text = _selectedMessage.Body.Replace("\r", "");
        RefreshMessageList();
    }

    private void RefreshMessageList()
    {
        if (_selectedConferenceId is not int cid) return;

        _updatingMessageList = true;
        try
        {
            var selectedNum = _selectedMessage?.MsgNumber;
            MessageList.ItemsSource = _session.MessagesInConference(cid).ToList();
            if (selectedNum != null)
            {
                MessageList.SelectedItem = _session.MessagesInConference(cid)
                    .FirstOrDefault(m => m.MsgNumber == selectedNum);
            }
        }
        finally
        {
            _updatingMessageList = false;
        }
    }

    private void ClearDetail()
    {
        _selectedMessage = null;
        DetailFrom.Text = DetailTo.Text = DetailSubject.Text = DetailDate.Text = "";
        DetailBody.Text = "";
    }

    private async Task OpenQwkAsync()
    {
        var files = await StorageProvider.OpenFilePickerAsync(new FilePickerOpenOptions
        {
            Title = "Open QWK packet",
            AllowMultiple = false,
            FileTypeFilter =
            [
                new FilePickerFileType("QWK packets") { Patterns = ["*.qwk", "*.zip", "*.QWK", "*.ZIP"] },
                new FilePickerFileType("All files") { Patterns = ["*.*"] },
            ],
        });
        if (files.Count == 0) return;

        try
        {
            var path = files[0].Path.LocalPath;
            _session.LoadFromFile(path);
            Title = $"Offline Mail — {_session.Control.BbsName}";
            ConferenceList.ItemsSource = _session.Conferences().ToList();
            if (_session.Conferences().Any())
                ConferenceList.SelectedIndex = 0;
            SetStatus($"Loaded {path} — {_session.Messages.Count} message(s), {_session.PendingReplies.Count} queued reply(ies).");
            UpdateUiState();
        }
        catch (Exception ex)
        {
            await ShowError($"Could not open QWK packet:\n{ex.Message}");
        }
    }

    private async Task SaveRepAsync()
    {
        if (_session.PendingReplies.Count == 0)
        {
            await ShowError("No replies queued. Reply or compose a message first.");
            return;
        }

        var file = await StorageProvider.SaveFilePickerAsync(new FilePickerSaveOptions
        {
            Title = "Save REP packet",
            SuggestedFileName = "REPLIES.REP",
            DefaultExtension = "rep",
            FileTypeChoices =
            [
                new FilePickerFileType("REP packet") { Patterns = ["*.rep", "*.zip"] },
            ],
        });
        if (file == null) return;

        try
        {
            var bytes = _session.BuildRepBytes();
            await File.WriteAllBytesAsync(file.Path.LocalPath, bytes);
            _session.ClearRepliesAfterExport();
            SetStatus($"Saved REP packet ({bytes.Length:N0} bytes). Queue cleared.");
            UpdateUiState();
        }
        catch (Exception ex)
        {
            await ShowError($"Could not save REP:\n{ex.Message}");
        }
    }

    private async Task DownloadFromBbsAsync()
    {
        if (_settings == null || string.IsNullOrWhiteSpace(_settings.Host) || string.IsNullOrWhiteSpace(_settings.Token))
        {
            await ShowError("Configure host and API token in Connection → Logon first, or open a local QWK file.");
            return;
        }

        try
        {
            SetStatus("Downloading QWK from BBS…");
            var api = new UserApiClient
            {
                Host = _settings.Host,
                Port = _settings.UserApiPort,
                Token = _settings.Token,
            };
            var result = await api.CallAsync("qwk.download");
            var b64 = result?["data"]?.GetValue<string>()
                ?? throw new InvalidDataException("Empty qwk.download response.");
            var bytes = Convert.FromBase64String(b64);
            _session.LoadFromBytes(bytes, $"BBS:{_settings.Host}");
            Title = $"Offline Mail — {_session.Control.BbsName}";
            ConferenceList.ItemsSource = _session.Conferences().ToList();
            if (_session.Conferences().Any()) ConferenceList.SelectedIndex = 0;
            SetStatus($"Downloaded QWK from BBS — {_session.Messages.Count} new message(s).");
            UpdateUiState();
        }
        catch (Exception ex)
        {
            await ShowError($"QWK download failed:\n{ex.Message}");
        }
    }

    private async Task UploadToBbsAsync()
    {
        if (_session.PendingReplies.Count == 0)
        {
            await ShowError("No replies queued.");
            return;
        }
        if (_settings == null || string.IsNullOrWhiteSpace(_settings.Host) || string.IsNullOrWhiteSpace(_settings.Token))
        {
            await ShowError("Configure host and API token in Connection → Logon first, or save a REP file for manual upload.");
            return;
        }

        try
        {
            SetStatus("Uploading REP to BBS…");
            var api = new UserApiClient
            {
                Host = _settings.Host,
                Port = _settings.UserApiPort,
                Token = _settings.Token,
            };
            var b64 = Convert.ToBase64String(_session.BuildRepBytes());
            var result = await api.CallAsync("qwk.upload", new { Data = b64 });
            var posted = result?["posted"]?.GetValue<int>() ?? _session.PendingReplies.Count;
            var rejected = result?["rejected"]?.GetValue<int>() ?? 0;
            _session.ClearRepliesAfterExport();
            SetStatus($"Uploaded REP: {posted} posted, {rejected} rejected.");
            UpdateUiState();
        }
        catch (Exception ex)
        {
            await ShowError($"REP upload failed:\n{ex.Message}");
        }
    }

    private void ComposeReply()
    {
        if (_selectedMessage is not QwkMessage msg)
        {
            _ = ShowError("Select a message to reply to.");
            return;
        }
        if (_selectedConferenceId is not int cid) return;
        var dlg = new ComposeMessageWindow(
            "Reply",
            msg.ToName == "All" ? msg.FromName : msg.ToName,
            msg.Subject.StartsWith("Re:", StringComparison.OrdinalIgnoreCase) ? msg.Subject : $"Re: {msg.Subject}",
            _session.DefaultFromName);
        dlg.ShowDialog(this);
        if (dlg.Accepted)
        {
            _session.QueueReply(new QwkReply(cid, msg.MsgNumber, dlg.ToName, dlg.FromName, dlg.Subject, dlg.Body));
            SetStatus($"Reply queued ({_session.PendingReplies.Count} pending). Save REP or Upload to BBS.");
            UpdateUiState();
        }
    }

    private void ComposeNew()
    {
        if (_selectedConferenceId is not int cid)
        {
            _ = ShowError("Select a conference first.");
            return;
        }
        var dlg = new ComposeMessageWindow("New Message", "All", "", _session.DefaultFromName);
        dlg.ShowDialog(this);
        if (dlg.Accepted)
        {
            _session.QueueReply(new QwkReply(cid, 0, dlg.ToName, dlg.FromName, dlg.Subject, dlg.Body));
            SetStatus($"Message queued ({_session.PendingReplies.Count} pending).");
            UpdateUiState();
        }
    }

    private async void ShowQueue()
    {
        if (_session.PendingReplies.Count == 0)
        {
            await ShowError("No pending replies.");
            return;
        }

        var sb = new StringBuilder();
        foreach (var r in _session.PendingReplies)
        {
            var confName = _session.Control.ConferenceNames.GetValueOrDefault(r.ConferenceId, $"#{r.ConferenceId}");
            sb.AppendLine($"[{confName}] {r.Subject} → {r.ToName}");
            sb.AppendLine($"  ref #{r.RefNum}  from {r.FromName}");
            sb.AppendLine();
        }
        await ShowMessage("Pending Replies", sb.ToString());
    }

    private void UpdateUiState()
    {
        var hasPacket = _session.Messages.Count > 0 || !string.IsNullOrEmpty(_session.Control.BbsName);
        SaveRepItem.IsEnabled = _session.PendingReplies.Count > 0;
        UploadBbsItem.IsEnabled = _session.PendingReplies.Count > 0;
        ReplyItem.IsEnabled = ReplyButton.IsEnabled = _selectedMessage != null;
        NewMessageItem.IsEnabled = NewButton.IsEnabled = _selectedConferenceId != null;
        DownloadBbsItem.IsEnabled = _settings != null && !string.IsNullOrWhiteSpace(_settings.Token);
    }

    private void SetStatus(string text) => StatusText.Text = text;

    private async Task ShowError(string message) => await ShowMessage("Offline Mail", message);

    private async Task ShowMessage(string title, string message)
    {
        var ok = new Button { Content = "OK", HorizontalAlignment = Avalonia.Layout.HorizontalAlignment.Center, IsDefault = true };
        var panel = new DockPanel { Margin = new Avalonia.Thickness(16) };
        DockPanel.SetDock(ok, Dock.Bottom);
        panel.Children.Add(ok);
        panel.Children.Add(new ScrollViewer
        {
            Content = new TextBlock { Text = message, TextWrapping = Avalonia.Media.TextWrapping.Wrap },
        });
        var dlg = new Window
        {
            Title = title,
            Width = 420,
            Height = 220,
            WindowStartupLocation = WindowStartupLocation.CenterOwner,
            Content = panel,
        };
        ok.Click += (_, _) => dlg.Close();
        await dlg.ShowDialog(this);
    }
}
