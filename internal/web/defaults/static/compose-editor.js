(function (window, document) {
  'use strict';

  var MARKUP_KEY = 'virtbbs.compose.markup';
  var WRAP_KEY = 'virtbbs.compose.hardWrap';
  var MAX_BYTES = 16384;
  var WARN_BYTES = 14336;

  function loadI18n() {
    var el = document.getElementById('compose-i18n');
    if (!el) return {};
    try { return JSON.parse(el.textContent); } catch (e) { return {}; }
  }

  function t(i18n, key, fallback, params) {
    var text = i18n[key] || fallback || key;
    if (params) {
      Object.keys(params).forEach(function (k) {
        text = text.replace('{' + k + '}', params[k]);
      });
    }
    return text;
  }

  function byteLength(str) {
    if (window.TextEncoder) return new TextEncoder().encode(str || '').length;
    return unescape(encodeURIComponent(str || '')).length;
  }

  function defaultMarkup(editorType) {
    return editorType === 'full' ? 'ansi' : 'stylecodes';
  }

  function scAction(ta, action) {
    var ch = { bold: '*', italic: '/', underline: '_', inverse: '#' }[action];
    var ph = { bold: 'bold text', italic: 'italic text', underline: 'underlined text', inverse: 'inverse text' }[action];
    if (!ch || !ta) return;
    var start = ta.selectionStart;
    var end = ta.selectionEnd;
    var sel = ta.value.substring(start, end);
    var before = ta.value.substring(0, start);
    var after = ta.value.substring(end);
    if (sel) {
      ta.value = before + ch + sel + ch + after;
      ta.selectionStart = start + 1;
      ta.selectionEnd = end + 1;
    } else {
      ta.value = before + ch + ph + ch + after;
      ta.selectionStart = start + 1;
      ta.selectionEnd = start + 1 + ph.length;
    }
    ta.focus();
    ta.dispatchEvent(new Event('input', { bubbles: true }));
  }

  function bindHardWrap(ta, getLimit) {
    ta.addEventListener('keydown', function (e) {
      var hardWrapLimit = getLimit();
      if (!hardWrapLimit) return;
      if (e.key.length !== 1 || e.ctrlKey || e.metaKey || e.altKey) return;
      if (this.selectionStart !== this.selectionEnd) return;
      var val = this.value;
      var pos = this.selectionStart;
      var lineStart = val.lastIndexOf('\n', pos - 1) + 1;
      if (pos - lineStart < hardWrapLimit) return;
      e.preventDefault();
      if (e.key === ' ') {
        this.value = val.substring(0, pos) + '\n' + val.substring(pos);
        this.selectionStart = this.selectionEnd = pos + 1;
      } else {
        var lineText = val.substring(lineStart, pos);
        var lastSpaceRel = lineText.lastIndexOf(' ');
        if (lastSpaceRel >= 0) {
          var breakPos = lineStart + lastSpaceRel;
          this.value = val.substring(0, breakPos) + '\n' + val.substring(breakPos + 1, pos) + e.key + val.substring(pos);
          this.selectionStart = this.selectionEnd = pos + 1;
        } else {
          this.value = val.substring(0, pos) + '\n' + e.key + val.substring(pos);
          this.selectionStart = this.selectionEnd = pos + 2;
        }
      }
      this.dispatchEvent(new Event('input', { bubbles: true }));
    });
  }

  function ComposeEditor(root) {
    this.root = root;
    this.i18n = loadI18n();
    window.virtbbsComposeI18n = this.i18n;
    this.textarea = root.querySelector('textarea');
    this.markupSelect = root.querySelector('[data-compose-markup]');
    this.hardWrapSelect = root.querySelector('[data-compose-hard-wrap]');
    this.statsLines = root.querySelector('[data-compose-lines]');
    this.statsBytes = root.querySelector('[data-compose-bytes]');
    this.sizeWarn = root.querySelector('[data-compose-size-warn]');
    this.ansiHost = root.querySelector('[data-compose-ansi-host]');
    this.ansiEditor = null;
    this.editorType = root.getAttribute('data-editor-type') || 'simple';
    this.hardWrapLimit = 0;
    this.init();
  }

  ComposeEditor.prototype.init = function () {
    var self = this;
    var savedMarkup = localStorage.getItem(MARKUP_KEY);
    var savedWrap = localStorage.getItem(WRAP_KEY);
    var mode = savedMarkup || defaultMarkup(this.editorType);
    if (this.markupSelect) this.markupSelect.value = mode;
    if (savedWrap !== null && this.hardWrapSelect) {
      this.hardWrapSelect.value = savedWrap;
      this.hardWrapLimit = parseInt(savedWrap, 10) || 0;
    }
    this.setMode(mode);
    if (this.markupSelect) {
      this.markupSelect.addEventListener('change', function () {
        self.setMode(self.markupSelect.value);
        localStorage.setItem(MARKUP_KEY, self.markupSelect.value);
      });
    }
    if (this.hardWrapSelect) {
      this.hardWrapSelect.addEventListener('change', function () {
        self.hardWrapLimit = parseInt(self.hardWrapSelect.value, 10) || 0;
        localStorage.setItem(WRAP_KEY, String(self.hardWrapLimit));
      });
    }
    bindHardWrap(this.textarea, function () { return self.hardWrapLimit; });
    this.textarea.addEventListener('input', function () { self.updateStats(); });
    this.textarea.addEventListener('keydown', function (e) {
      if ((e.ctrlKey || e.metaKey) && self.markupSelect && self.markupSelect.value === 'stylecodes') {
        if (e.key === 'b') { e.preventDefault(); scAction(self.textarea, 'bold'); }
        if (e.key === 'i') { e.preventDefault(); scAction(self.textarea, 'italic'); }
      }
    });
    this.root.querySelectorAll('[data-sc-action]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        scAction(self.textarea, btn.getAttribute('data-sc-action'));
      });
    });
    this.updateStats();
  };

  ComposeEditor.prototype.setMode = function (mode) {
    var self = this;
    this.root.querySelectorAll('[data-compose-toolbar]').forEach(function (tb) {
      var active = tb.getAttribute('data-compose-toolbar') === mode;
      tb.classList.toggle('d-none', !active);
    });
    if (mode === 'ansi') {
      this.mountAnsiEditor();
      this.textarea.classList.add('font-monospace');
    } else {
      this.unmountAnsiEditor();
      if (mode !== 'ansi') this.textarea.classList.remove('font-monospace');
    }
    if (this.ansiHost) this.ansiHost.classList.toggle('d-none', mode !== 'ansi');
    setTimeout(function () {
      if (self.ansiEditor && self.ansiEditor.syncRulerMetrics) self.ansiEditor.syncRulerMetrics();
    }, 0);
  };

  ComposeEditor.prototype.mountAnsiEditor = function () {
    if (this.ansiEditor || !this.ansiHost || !window.AnsiEditor) return;
    this.ansiHost.classList.remove('d-none');
    this.ansiHost.appendChild(this.textarea);
    this.ansiEditor = window.AnsiEditor.create(this.ansiHost);
  };

  ComposeEditor.prototype.unmountAnsiEditor = function () {
    if (!this.ansiHost) return;
    if (this.textarea.parentNode === this.ansiHost) {
      this.ansiHost.parentNode.insertBefore(this.textarea, this.ansiHost);
    }
    this.ansiHost.innerHTML = '';
    this.ansiHost.classList.add('d-none');
    this.ansiEditor = null;
  };

  ComposeEditor.prototype.updateStats = function () {
    var text = this.textarea ? this.textarea.value : '';
    var lines = text ? text.split('\n').length : 0;
    var bytes = byteLength(text);
    if (this.statsLines) {
      this.statsLines.textContent = t(this.i18n, 'stats_lines', '{count} lines').replace('{count}', String(lines));
    }
    if (this.statsBytes) {
      this.statsBytes.textContent = t(this.i18n, 'stats_bytes', '{count} bytes').replace('{count}', String(bytes));
    }
    if (this.sizeWarn) {
      if (bytes > MAX_BYTES) {
        var kb = (bytes / 1024).toFixed(1);
        this.sizeWarn.textContent = t(this.i18n, 'body_too_large', 'Message body is {kb} KB and exceeds the 16 KB FidoNet limit.', { kb: kb });
        this.sizeWarn.classList.remove('d-none', 'text-warning');
        this.sizeWarn.classList.add('text-danger');
      } else if (bytes >= WARN_BYTES) {
        var kbWarn = (bytes / 1024).toFixed(1);
        this.sizeWarn.textContent = t(this.i18n, 'body_size_warning', 'Message body is {kb} KB — approaching the 16 KB FidoNet limit.', { kb: kbWarn });
        this.sizeWarn.classList.remove('d-none', 'text-danger');
        this.sizeWarn.classList.add('text-warning');
      } else {
        this.sizeWarn.textContent = '';
        this.sizeWarn.classList.add('d-none');
      }
    }
  };

  var root = null;
  document.querySelectorAll('[data-compose-editor]').forEach(function (el) {
    root = new ComposeEditor(el);
  });
})(window, document);
