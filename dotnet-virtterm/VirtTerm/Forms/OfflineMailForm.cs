using System.Text;
using VirtTerm.Net;
using VirtTerm.Qwk;
using VirtTerm.Settings;

namespace VirtTerm.Forms;

public class OfflineMailForm : Form
{
    private readonly QwkSession _session = new();
    private readonly AppSettings? _settings;

    private QwkMessage? _selectedMessage;
    private int? _selectedConferenceId;
    private bool _updatingMessageList;

    private ToolStripMenuItem _saveRepItem = null!;
    private ToolStripMenuItem _downloadBbsItem = null!;
    private ToolStripMenuItem _uploadBbsItem = null!;
    private ToolStripMenuItem _newMessageItem = null!;
    private ToolStripMenuItem _replyItem = null!;
    private Button _replyButton = null!;
    private Button _newButton = null!;

    private ListBox _conferenceList = null!;
    private ListBox _messageList = null!;
    private Label _messageHeader = null!;
    private Label _detailFrom = null!;
    private Label _detailTo = null!;
    private Label _detailSubject = null!;
    private Label _detailDate = null!;
    private TextBox _detailBody = null!;
    private ToolStripStatusLabel _statusLabel = null!;

    public OfflineMailForm() : this(null) { }

    public OfflineMailForm(AppSettings? settings)
    {
        _settings = settings;
        Text = "Offline Mail (QWK)";
        StartPosition = FormStartPosition.CenterScreen;
        ClientSize = new Size(960, 640);

        BuildMenu();
        BuildLayout();
        WireEvents();
        UpdateUiState();
        SetStatus("Open a QWK packet to begin. No BBS connection required.");
    }

    private void BuildMenu()
    {
        var menu = new MenuStrip();

        var fileMenu = new ToolStripMenuItem("&File");
        fileMenu.DropDownItems.Add(new ToolStripMenuItem("&Open QWK Packet...", null, (_, _) => OpenQwk()));
        _saveRepItem = new ToolStripMenuItem("Save &REP Packet...", null, (_, _) => SaveRep());
        fileMenu.DropDownItems.Add(_saveRepItem);
        fileMenu.DropDownItems.Add(new ToolStripSeparator());
        _downloadBbsItem = new ToolStripMenuItem("&Download QWK from BBS...", null, (_, _) => _ = DownloadFromBbsAsync());
        fileMenu.DropDownItems.Add(_downloadBbsItem);
        _uploadBbsItem = new ToolStripMenuItem("&Upload REP to BBS...", null, (_, _) => _ = UploadToBbsAsync());
        fileMenu.DropDownItems.Add(_uploadBbsItem);
        fileMenu.DropDownItems.Add(new ToolStripSeparator());
        fileMenu.DropDownItems.Add(new ToolStripMenuItem("E&xit", null, (_, _) => Close()));
        menu.Items.Add(fileMenu);

        var messageMenu = new ToolStripMenuItem("&Message");
        _newMessageItem = new ToolStripMenuItem("&New Message...", null, (_, _) => ComposeNew());
        messageMenu.DropDownItems.Add(_newMessageItem);
        _replyItem = new ToolStripMenuItem("&Reply...", null, (_, _) => ComposeReply());
        messageMenu.DropDownItems.Add(_replyItem);
        menu.Items.Add(messageMenu);

        var queueMenu = new ToolStripMenuItem("&Queue");
        queueMenu.DropDownItems.Add(new ToolStripMenuItem("&View Pending Replies...", null, (_, _) => ShowQueue()));
        menu.Items.Add(queueMenu);

        MainMenuStrip = menu;
        Controls.Add(menu);
    }

    private void BuildLayout()
    {
        var status = new StatusStrip();
        _statusLabel = new ToolStripStatusLabel { Spring = true, TextAlign = ContentAlignment.MiddleLeft };
        status.Items.Add(_statusLabel);
        Controls.Add(status);

        var splitOuter = new SplitContainer
        {
            Dock = DockStyle.Fill,
            SplitterDistance = 220,
            FixedPanel = FixedPanel.Panel1,
        };

        var splitInner = new SplitContainer
        {
            Dock = DockStyle.Fill,
            SplitterDistance = 400,
            FixedPanel = FixedPanel.Panel2,
        };

        var confPanel = new Panel { Dock = DockStyle.Fill, Padding = new Padding(8) };
        confPanel.Controls.Add(new Label
        {
            Text = "Conferences",
            Dock = DockStyle.Top,
            Height = 20,
            Font = new Font(Font, FontStyle.Bold),
        });
        _conferenceList = new ListBox { Dock = DockStyle.Fill };
        confPanel.Controls.Add(_conferenceList);

        var msgPanel = new Panel { Dock = DockStyle.Fill, Padding = new Padding(8) };
        _messageHeader = new Label
        {
            Text = "Messages",
            Dock = DockStyle.Top,
            Height = 20,
            Font = new Font(Font, FontStyle.Bold),
        };
        _messageList = new ListBox { Dock = DockStyle.Fill };
        msgPanel.Controls.Add(_messageList);
        msgPanel.Controls.Add(_messageHeader);

        var detailPanel = new Panel { Dock = DockStyle.Fill, Padding = new Padding(8) };
        var buttonRow = new FlowLayoutPanel
        {
            Dock = DockStyle.Top,
            Height = 32,
            FlowDirection = FlowDirection.LeftToRight,
            WrapContents = false,
        };
        _replyButton = new Button { Text = "Reply", AutoSize = true };
        _newButton = new Button { Text = "New", AutoSize = true };
        buttonRow.Controls.Add(_replyButton);
        buttonRow.Controls.Add(_newButton);

        _detailFrom = new Label { Dock = DockStyle.Top, Height = 20, Font = new Font(Font, FontStyle.Bold) };
        _detailTo = new Label { Dock = DockStyle.Top, Height = 20 };
        _detailSubject = new Label { Dock = DockStyle.Top, Height = 20, Font = new Font(Font, FontStyle.Bold) };
        _detailDate = new Label { Dock = DockStyle.Top, Height = 20, ForeColor = System.Drawing.Color.Gray };
        _detailBody = new TextBox
        {
            Dock = DockStyle.Fill,
            Multiline = true,
            ReadOnly = true,
            ScrollBars = ScrollBars.Vertical,
            WordWrap = true,
        };

        var detailStack = new Panel { Dock = DockStyle.Fill };
        detailStack.Controls.Add(_detailBody);
        detailStack.Controls.Add(_detailDate);
        detailStack.Controls.Add(_detailSubject);
        detailStack.Controls.Add(_detailTo);
        detailStack.Controls.Add(_detailFrom);

        detailPanel.Controls.Add(detailStack);
        detailPanel.Controls.Add(buttonRow);

        splitOuter.Panel1.Controls.Add(confPanel);
        splitInner.Panel1.Controls.Add(msgPanel);
        splitInner.Panel2.Controls.Add(detailPanel);
        splitOuter.Panel2.Controls.Add(splitInner);

        Controls.Add(splitOuter);
    }

    private void WireEvents()
    {
        _conferenceList.SelectedIndexChanged += (_, _) => OnConferenceSelected();
        _messageList.SelectedIndexChanged += (_, _) => OnMessageSelected();
        _replyButton.Click += (_, _) => ComposeReply();
        _newButton.Click += (_, _) => ComposeNew();
    }

    private void OnConferenceSelected()
    {
        if (_conferenceList.SelectedItem is not QwkConference conf)
        {
            _selectedConferenceId = null;
            _messageList.DataSource = null;
            return;
        }

        _selectedConferenceId = conf.Id;
        _messageHeader.Text = $"Messages — {conf.Name}";
        _updatingMessageList = true;
        try
        {
            _messageList.DataSource = _session.MessagesInConference(conf.Id).ToList();
        }
        finally
        {
            _updatingMessageList = false;
        }
        ClearDetail();
        UpdateUiState();
    }

    private void OnMessageSelected()
    {
        if (_updatingMessageList) return;
        if (_messageList.SelectedItem is not QwkMessage msg) return;

        _session.MarkRead(msg);
        _selectedMessage = _session.MessagesInConference(msg.ConferenceId)
            .FirstOrDefault(m => m.MsgNumber == msg.MsgNumber) ?? msg;

        _detailFrom.Text = $"From: {_selectedMessage.FromName}";
        _detailTo.Text = $"To: {_selectedMessage.ToName}";
        _detailSubject.Text = _selectedMessage.Subject;
        _detailDate.Text = $"{_selectedMessage.Date} {_selectedMessage.Time}  (#{_selectedMessage.MsgNumber})";
        _detailBody.Text = _selectedMessage.Body.Replace("\r", "");
        RefreshMessageList();
        UpdateUiState();
    }

    private void RefreshMessageList()
    {
        if (_selectedConferenceId is not int cid) return;

        _updatingMessageList = true;
        try
        {
            var selectedNum = _selectedMessage?.MsgNumber;
            _messageList.DataSource = _session.MessagesInConference(cid).ToList();
            if (selectedNum != null)
            {
                _messageList.SelectedItem = _session.MessagesInConference(cid)
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
        _detailFrom.Text = _detailTo.Text = _detailSubject.Text = _detailDate.Text = "";
        _detailBody.Text = "";
    }

    private void OpenQwk()
    {
        using var dlg = new OpenFileDialog
        {
            Title = "Open QWK packet",
            Filter = "QWK packets (*.qwk;*.zip)|*.qwk;*.zip;*.QWK;*.ZIP|All files (*.*)|*.*",
        };
        if (dlg.ShowDialog(this) != DialogResult.OK) return;

        try
        {
            _session.LoadFromFile(dlg.FileName);
            Text = $"Offline Mail — {_session.Control.BbsName}";
            _conferenceList.DataSource = _session.Conferences().ToList();
            if (_conferenceList.Items.Count > 0)
                _conferenceList.SelectedIndex = 0;
            SetStatus($"Loaded {dlg.FileName} — {_session.Messages.Count} message(s), {_session.PendingReplies.Count} queued reply(ies).");
            UpdateUiState();
        }
        catch (Exception ex)
        {
            MessageBox.Show(this, $"Could not open QWK packet:\n{ex.Message}", "Offline Mail",
                MessageBoxButtons.OK, MessageBoxIcon.Error);
        }
    }

    private void SaveRep()
    {
        if (_session.PendingReplies.Count == 0)
        {
            MessageBox.Show(this, "No replies queued. Reply or compose a message first.", "Offline Mail",
                MessageBoxButtons.OK, MessageBoxIcon.Information);
            return;
        }

        using var dlg = new SaveFileDialog
        {
            Title = "Save REP packet",
            FileName = "REPLIES.REP",
            Filter = "REP packet (*.rep)|*.rep;*.zip",
            DefaultExt = "rep",
        };
        if (dlg.ShowDialog(this) != DialogResult.OK) return;

        try
        {
            var bytes = _session.BuildRepBytes();
            File.WriteAllBytes(dlg.FileName, bytes);
            _session.ClearRepliesAfterExport();
            SetStatus($"Saved REP packet ({bytes.Length:N0} bytes). Queue cleared.");
            UpdateUiState();
        }
        catch (Exception ex)
        {
            MessageBox.Show(this, $"Could not save REP:\n{ex.Message}", "Offline Mail",
                MessageBoxButtons.OK, MessageBoxIcon.Error);
        }
    }

    private async Task DownloadFromBbsAsync()
    {
        if (_settings == null || string.IsNullOrWhiteSpace(_settings.Host) || string.IsNullOrWhiteSpace(_settings.Token))
        {
            MessageBox.Show(this,
                "Configure host and API token in Connection → Logon first, or open a local QWK file.",
                "Offline Mail", MessageBoxButtons.OK, MessageBoxIcon.Information);
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
            Text = $"Offline Mail — {_session.Control.BbsName}";
            _conferenceList.DataSource = _session.Conferences().ToList();
            if (_conferenceList.Items.Count > 0)
                _conferenceList.SelectedIndex = 0;
            SetStatus($"Downloaded QWK from BBS — {_session.Messages.Count} new message(s).");
            UpdateUiState();
        }
        catch (Exception ex)
        {
            MessageBox.Show(this, $"QWK download failed:\n{ex.Message}", "Offline Mail",
                MessageBoxButtons.OK, MessageBoxIcon.Error);
        }
    }

    private async Task UploadToBbsAsync()
    {
        if (_session.PendingReplies.Count == 0)
        {
            MessageBox.Show(this, "No replies queued.", "Offline Mail",
                MessageBoxButtons.OK, MessageBoxIcon.Information);
            return;
        }
        if (_settings == null || string.IsNullOrWhiteSpace(_settings.Host) || string.IsNullOrWhiteSpace(_settings.Token))
        {
            MessageBox.Show(this,
                "Configure host and API token in Connection → Logon first, or save a REP file for manual upload.",
                "Offline Mail", MessageBoxButtons.OK, MessageBoxIcon.Information);
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
            MessageBox.Show(this, $"REP upload failed:\n{ex.Message}", "Offline Mail",
                MessageBoxButtons.OK, MessageBoxIcon.Error);
        }
    }

    private void ComposeReply()
    {
        if (_selectedMessage is not QwkMessage msg)
        {
            MessageBox.Show(this, "Select a message to reply to.", "Offline Mail",
                MessageBoxButtons.OK, MessageBoxIcon.Information);
            return;
        }
        if (_selectedConferenceId is not int cid) return;

        using var dlg = new ComposeMessageForm(
            "Reply",
            msg.ToName == "All" ? msg.FromName : msg.ToName,
            msg.Subject.StartsWith("Re:", StringComparison.OrdinalIgnoreCase) ? msg.Subject : $"Re: {msg.Subject}",
            _session.DefaultFromName);
        if (dlg.ShowDialog(this) != DialogResult.OK || !dlg.Accepted) return;

        _session.QueueReply(new QwkReply(cid, msg.MsgNumber, dlg.ToName, dlg.FromName, dlg.Subject, dlg.Body));
        SetStatus($"Reply queued ({_session.PendingReplies.Count} pending). Save REP or Upload to BBS.");
        UpdateUiState();
    }

    private void ComposeNew()
    {
        if (_selectedConferenceId is not int cid)
        {
            MessageBox.Show(this, "Select a conference first.", "Offline Mail",
                MessageBoxButtons.OK, MessageBoxIcon.Information);
            return;
        }

        using var dlg = new ComposeMessageForm("New Message", "All", "", _session.DefaultFromName);
        if (dlg.ShowDialog(this) != DialogResult.OK || !dlg.Accepted) return;

        _session.QueueReply(new QwkReply(cid, 0, dlg.ToName, dlg.FromName, dlg.Subject, dlg.Body));
        SetStatus($"Message queued ({_session.PendingReplies.Count} pending).");
        UpdateUiState();
    }

    private void ShowQueue()
    {
        if (_session.PendingReplies.Count == 0)
        {
            MessageBox.Show(this, "No pending replies.", "Offline Mail",
                MessageBoxButtons.OK, MessageBoxIcon.Information);
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
        MessageBox.Show(this, sb.ToString(), "Pending Replies",
            MessageBoxButtons.OK, MessageBoxIcon.Information);
    }

    private void UpdateUiState()
    {
        _saveRepItem.Enabled = _session.PendingReplies.Count > 0;
        _uploadBbsItem.Enabled = _session.PendingReplies.Count > 0;
        _replyItem.Enabled = _replyButton.Enabled = _selectedMessage != null;
        _newMessageItem.Enabled = _newButton.Enabled = _selectedConferenceId != null;
        _downloadBbsItem.Enabled = _settings != null && !string.IsNullOrWhiteSpace(_settings.Token);
    }

    private void SetStatus(string text) => _statusLabel.Text = text;
}
