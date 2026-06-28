using System.Drawing;
using System.Windows.Forms;

namespace VirtTerm.Forms;

public class ComposeMessageForm : Form
{
    private readonly TextBox _to = new();
    private readonly TextBox _from = new();
    private readonly TextBox _subject = new();
    private readonly TextBox _body = new();

    public bool Accepted { get; private set; }
    public string ToName => _to.Text.Trim();
    public string FromName => _from.Text.Trim();
    public string Subject => _subject.Text.Trim();
    public string Body => _body.Text;

    public ComposeMessageForm(string title, string to, string subject, string from)
    {
        Text = title;
        FormBorderStyle = FormBorderStyle.FixedDialog;
        StartPosition = FormStartPosition.CenterParent;
        MaximizeBox = false;
        MinimizeBox = false;
        ClientSize = new Size(480, 420);

        var layout = new TableLayoutPanel
        {
            Dock = DockStyle.Fill,
            Padding = new Padding(12),
            ColumnCount = 1,
            RowCount = 5,
        };
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.Absolute, 28));
        layout.RowStyles.Add(new RowStyle(SizeType.Absolute, 28));
        layout.RowStyles.Add(new RowStyle(SizeType.Absolute, 28));
        layout.RowStyles.Add(new RowStyle(SizeType.Percent, 100));

        var titleLabel = new Label { Text = title, Font = new Font(Font, FontStyle.Bold), AutoSize = true };
        _to.Text = to;
        _from.Text = from;
        _subject.Text = subject;
        _body.Multiline = true;
        _body.ScrollBars = ScrollBars.Vertical;
        _body.AcceptsReturn = true;
        _body.WordWrap = true;
        _to.Dock = _from.Dock = _subject.Dock = _body.Dock = DockStyle.Fill;

        layout.Controls.Add(titleLabel, 0, 0);
        layout.Controls.Add(MakeField("To", _to), 0, 1);
        layout.Controls.Add(MakeField("From", _from), 0, 2);
        layout.Controls.Add(MakeField("Subject", _subject), 0, 3);
        layout.Controls.Add(_body, 0, 4);

        var buttons = new FlowLayoutPanel
        {
            Dock = DockStyle.Bottom,
            FlowDirection = FlowDirection.RightToLeft,
            Height = 40,
            Padding = new Padding(0, 4, 12, 4),
        };
        var queue = new Button { Text = "Queue", DialogResult = DialogResult.None };
        var cancel = new Button { Text = "Cancel", DialogResult = DialogResult.Cancel };
        queue.Click += (_, _) => OnQueue();
        buttons.Controls.Add(queue);
        buttons.Controls.Add(cancel);
        AcceptButton = queue;
        CancelButton = cancel;

        Controls.Add(layout);
        Controls.Add(buttons);
    }

    private static Control MakeField(string label, Control input)
    {
        var panel = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, RowCount = 1 };
        panel.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 60));
        panel.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        panel.Controls.Add(new Label { Text = label, TextAlign = ContentAlignment.MiddleLeft, Dock = DockStyle.Fill }, 0, 0);
        panel.Controls.Add(input, 1, 0);
        return panel;
    }

    private void OnQueue()
    {
        if (string.IsNullOrWhiteSpace(Subject) || string.IsNullOrWhiteSpace(Body))
            return;
        Accepted = true;
        DialogResult = DialogResult.OK;
        Close();
    }
}
