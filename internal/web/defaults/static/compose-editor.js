(function (window, document) {
  'use strict';

  var MARKUP_KEY = 'virtbbs.compose.markup';
  var WRAP_KEY = 'virtbbs.compose.hardWrap';
  var MAX_BYTES = 16384;
  var WARN_BYTES = 14336;
  var booted = false;

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

  function reflowHardWrap(text, limit) {
    if (!limit || limit < 1) return text;
    return String(text || '').split('\n').map(function (line) {
      var out = [];
      while (line.length > limit) {
        var breakAt = line.lastIndexOf(' ', limit);
        if (breakAt <= 0) breakAt = limit;
        out.push(line.slice(0, breakAt).replace(/\s+$/, ''));
        line = line.slice(breakAt).replace(/^\s+/, '');
      }
      out.push(line);
      return out.join('\n');
    }).join('\n');
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

  function showBootstrapModal(id) {
    var el = document.getElementById(id);
    if (!el || !window.bootstrap || !window.bootstrap.Modal) return false;
    window.bootstrap.Modal.getOrCreateInstance(el).show();
    return true;
  }

  function ComposeEditor(root) {
    this.root = root;
    this.i18n = loadI18n();
    window.virtbbsComposeI18n = this.i18n;
    this.textarea = root.querySelector('textarea');
    this.markupSelect = root.querySelector('[data-compose-markup]');
    this.hardWrapSelect = root.querySelector('[data-compose-hard-wrap]');
    this.applyWrapBtn = root.querySelector('[data-compose-apply-wrap]');
    this.previewBtn = root.querySelector('[data-compose-preview]');
    this.statsLines = root.querySelector('[data-compose-lines]');
    this.statsBytes = root.querySelector('[data-compose-bytes]');
    this.sizeWarn = root.querySelector('[data-compose-size-warn]');
    this.ansiHost = root.querySelector('[data-compose-ansi-host]');
    this.editorBody = root.querySelector('.compose-editor-body');
    this.ansiEditor = null;
    this.editorType = root.getAttribute('data-editor-type') || 'simple';
    this.hardWrapLimit = 0;
    this.mode = 'plain';
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
    this.applyWrapVisual();

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
        self.applyWrapVisual();
        self.toggleApplyWrap();
        if (self.hardWrapLimit) self.applyHardWrapToTextarea();
      });
    }
    if (this.applyWrapBtn) {
      this.applyWrapBtn.addEventListener('click', function () {
        self.applyHardWrapToTextarea();
      });
    }
    if (this.previewBtn) {
      this.previewBtn.addEventListener('click', function () {
        self.openPreview();
      });
    }

    this.textarea.addEventListener('input', function () { self.updateStats(); });
    this.textarea.addEventListener('keydown', function (e) { self.onKeydown(e); });

    this.root.querySelectorAll('[data-sc-action]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        scAction(self.textarea, btn.getAttribute('data-sc-action'));
      });
    });

    this.updateStats();
    this.toggleApplyWrap();
  };

  ComposeEditor.prototype.toggleApplyWrap = function () {
    if (!this.applyWrapBtn) return;
    this.applyWrapBtn.classList.toggle('d-none', !this.hardWrapLimit);
  };

  ComposeEditor.prototype.applyWrapVisual = function () {
    if (!this.textarea) return;
    if (this.hardWrapLimit > 0) {
      this.textarea.style.maxWidth = (this.hardWrapLimit + 2) + 'ch';
      this.textarea.style.width = '100%';
      this.textarea.classList.add('compose-wrap-active');
      this.textarea.setAttribute('data-wrap-cols', String(this.hardWrapLimit));
    } else {
      this.textarea.style.maxWidth = '';
      this.textarea.classList.remove('compose-wrap-active');
      this.textarea.removeAttribute('data-wrap-cols');
    }
    if (this.ansiEditor && this.ansiEditor.syncRulerMetrics) {
      this.ansiEditor.syncRulerMetrics();
    }
  };

  ComposeEditor.prototype.applyHardWrapToTextarea = function () {
    if (!this.hardWrapLimit || !this.textarea) return;
    var start = this.textarea.selectionStart;
    var end = this.textarea.selectionEnd;
    this.textarea.value = reflowHardWrap(this.textarea.value, this.hardWrapLimit);
    this.textarea.selectionStart = start;
    this.textarea.selectionEnd = end;
    this.textarea.dispatchEvent(new Event('input', { bubbles: true }));
  };

  ComposeEditor.prototype.onKeydown = function (e) {
    if ((e.ctrlKey || e.metaKey) && this.mode === 'stylecodes') {
      if (e.key === 'b') { e.preventDefault(); scAction(this.textarea, 'bold'); return; }
      if (e.key === 'i') { e.preventDefault(); scAction(this.textarea, 'italic'); return; }
    }
    if (!this.hardWrapLimit) return;
    if (e.key !== 'Enter' || e.shiftKey || e.ctrlKey || e.metaKey || e.altKey) return;
    var val = this.textarea.value;
    var pos = this.textarea.selectionStart;
    if (pos !== this.textarea.selectionEnd) return;
    var lineStart = val.lastIndexOf('\n', pos - 1) + 1;
    var line = val.substring(lineStart, pos);
    if (line.length <= this.hardWrapLimit) return;
    e.preventDefault();
    var breakAt = line.lastIndexOf(' ', this.hardWrapLimit);
    if (breakAt <= 0) breakAt = this.hardWrapLimit;
    var absBreak = lineStart + breakAt;
    this.textarea.value = val.substring(0, absBreak) + '\n' + val.substring(absBreak).replace(/^\s+/, '');
    var newPos = absBreak + 1;
    this.textarea.selectionStart = this.textarea.selectionEnd = newPos;
    this.textarea.dispatchEvent(new Event('input', { bubbles: true }));
  };

  ComposeEditor.prototype.setMode = function (mode) {
    this.mode = mode || 'plain';
    this.root.querySelectorAll('[data-compose-toolbar]').forEach(function (tb) {
      tb.classList.toggle('d-none', tb.getAttribute('data-compose-toolbar') !== mode);
    });
    this.root.querySelectorAll('[data-compose-hint]').forEach(function (hint) {
      hint.classList.toggle('d-none', hint.getAttribute('data-compose-hint') !== mode);
    });
    if (mode === 'ansi') {
      this.mountAnsiEditor();
      this.textarea.classList.add('font-monospace');
    } else {
      this.unmountAnsiEditor();
      this.textarea.classList.toggle('font-monospace', mode === 'ansi');
    }
    if (this.ansiHost) this.ansiHost.classList.toggle('d-none', mode !== 'ansi');
    var self = this;
    setTimeout(function () {
      if (self.ansiEditor && self.ansiEditor.syncRulerMetrics) self.ansiEditor.syncRulerMetrics();
      self.applyWrapVisual();
    }, 0);
  };

  ComposeEditor.prototype.mountAnsiEditor = function () {
    if (!this.ansiHost || !window.AnsiEditor) return;
    if (this.ansiEditor) return;
    this.ansiHost.classList.remove('d-none');
    this.ansiHost.appendChild(this.textarea);
    this.ansiEditor = window.AnsiEditor.create(this.ansiHost, { hidePreview: true });
  };

  ComposeEditor.prototype.unmountAnsiEditor = function () {
    if (!this.ansiHost || !this.editorBody) return;
    if (this.textarea.parentNode === this.ansiHost) {
      this.editorBody.insertBefore(this.textarea, this.ansiHost);
    }
    this.ansiHost.innerHTML = '';
    this.ansiHost.classList.add('d-none');
    this.ansiEditor = null;
  };

  ComposeEditor.prototype.openPreview = function () {
    var body = document.getElementById('compose-preview-body');
    if (!body) return;
    var content = this.textarea ? this.textarea.value : '';
    if (window.virtbbsComposePreview) {
      window.virtbbsComposePreview.render(body, content, this.mode);
    } else if (window.virtbbsAnsiPreview) {
      window.virtbbsAnsiPreview.renderPreview(body, content);
    } else {
      body.innerHTML = '<pre class="mb-0">' + content.replace(/</g, '&lt;') + '</pre>';
    }
    showBootstrapModal('compose-preview-modal');
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

  function boot() {
    if (booted) return;
    booted = true;
    document.querySelectorAll('[data-compose-editor]').forEach(function (el) {
      new ComposeEditor(el);
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})(window, document);
