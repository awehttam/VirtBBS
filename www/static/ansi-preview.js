(function (window) {
  'use strict';

  var FG = {
    '30': 'ansi-fg-black', '31': 'ansi-fg-red', '32': 'ansi-fg-green', '33': 'ansi-fg-yellow',
    '34': 'ansi-fg-blue', '35': 'ansi-fg-magenta', '36': 'ansi-fg-cyan', '37': 'ansi-fg-white',
    '90': 'ansi-fg-bright-black', '91': 'ansi-fg-bright-red', '92': 'ansi-fg-bright-green',
    '93': 'ansi-fg-bright-yellow', '94': 'ansi-fg-bright-blue', '95': 'ansi-fg-bright-magenta',
    '96': 'ansi-fg-bright-cyan', '97': 'ansi-fg-bright-white'
  };

  function escapeHtml(text) {
    return String(text || '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function ansiToHTML(raw) {
    var s = String(raw || '');
    s = s.replace(/\x1b\[[0-9;]*[HJ]/g, '');
    s = s.replace(/\r\n/g, '\n').replace(/\r/g, '\n');

    var state = { bold: false, fg: '' };
    var out = '';
    var re = /\x1b\[([0-9;]*)m/g;
    var pos = 0;
    var m;

    function classes() {
      var list = [];
      if (state.bold) list.push('ansi-bold');
      if (state.fg) list.push(state.fg);
      return list.join(' ');
    }

    function apply(code) {
      if (!code || code === '0') {
        state.bold = false;
        state.fg = '';
        return;
      }
      code.split(';').forEach(function (part) {
        if (part === '1') state.bold = true;
        else if (part === '22') state.bold = false;
        else if (part === '39') state.fg = '';
        else if (FG[part]) state.fg = FG[part];
      });
    }

    function flush(text) {
      if (!text) return;
      var escaped = escapeHtml(text).replace(/\n/g, '<br>');
      var cls = classes();
      if (cls) {
        out += '<span class="' + cls + '">' + escaped + '</span>';
      } else {
        out += escaped;
      }
    }

    while ((m = re.exec(s)) !== null) {
      if (m.index > pos) flush(s.slice(pos, m.index));
      apply(m[1]);
      pos = m.index + m[0].length;
    }
    flush(s.slice(pos));
    return out;
  }

  window.virtbbsAnsiPreview = {
    ansiToHTML: ansiToHTML,
    renderPreview: function (container, content) {
      if (!container) return;
      var source = String(content || '');
      if (!source.trim()) {
        container.innerHTML = '<p class="meta p-3 mb-0">—</p>';
        return;
      }
      if (/\x1b\[/.test(source)) {
        container.innerHTML = '<div class="ansi-screen p-3">' + ansiToHTML(source) + '</div>';
      } else {
        container.innerHTML = '<pre class="mb-0 p-3">' + escapeHtml(source) + '</pre>';
      }
    }
  };
})(window);
