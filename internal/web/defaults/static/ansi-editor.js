(function (window, document) {
  'use strict';

  var PRESET_GROUPS = [
    {
      labelKey: 'formatting',
      fallback: 'Formatting',
      options: [
        { value: '0m', key: 'sequence_reset', fallback: 'Reset (ESC[0m)' },
        { value: '1m', key: 'sequence_bold', fallback: 'Bold (ESC[1m)' },
        { value: '5m', key: 'sequence_blink', fallback: 'Blink (ESC[5m)' },
        { value: '7m', key: 'sequence_reverse', fallback: 'Reverse Video (ESC[7m)' }
      ]
    },
    {
      labelKey: 'foreground',
      fallback: 'Text Colors',
      options: [
        { value: '30m', key: 'sequence_fg_black', fallback: 'Black (ESC[30m)' },
        { value: '31m', key: 'sequence_fg_red', fallback: 'Red (ESC[31m)' },
        { value: '32m', key: 'sequence_fg_green', fallback: 'Green (ESC[32m)' },
        { value: '33m', key: 'sequence_fg_yellow', fallback: 'Yellow (ESC[33m)' },
        { value: '34m', key: 'sequence_fg_blue', fallback: 'Blue (ESC[34m)' },
        { value: '35m', key: 'sequence_fg_magenta', fallback: 'Magenta (ESC[35m)' },
        { value: '36m', key: 'sequence_fg_cyan', fallback: 'Cyan (ESC[36m)' },
        { value: '37m', key: 'sequence_fg_white', fallback: 'White (ESC[37m)' }
      ]
    },
    {
      labelKey: 'background',
      fallback: 'Background Colors',
      options: [
        { value: '40m', key: 'sequence_bg_black', fallback: 'Black Background (ESC[40m)' },
        { value: '41m', key: 'sequence_bg_red', fallback: 'Red Background (ESC[41m)' },
        { value: '42m', key: 'sequence_bg_green', fallback: 'Green Background (ESC[42m)' },
        { value: '43m', key: 'sequence_bg_yellow', fallback: 'Yellow Background (ESC[43m)' },
        { value: '44m', key: 'sequence_bg_blue', fallback: 'Blue Background (ESC[44m)' },
        { value: '45m', key: 'sequence_bg_magenta', fallback: 'Magenta Background (ESC[45m)' },
        { value: '46m', key: 'sequence_bg_cyan', fallback: 'Cyan Background (ESC[46m)' },
        { value: '47m', key: 'sequence_bg_white', fallback: 'White Background (ESC[47m)' }
      ]
    },
    {
      labelKey: 'cursor',
      fallback: 'Screen and Cursor',
      options: [
        { value: '2J', key: 'sequence_clear_screen', fallback: 'Clear Screen (ESC[2J)' },
        { value: 'K', key: 'sequence_clear_line', fallback: 'Clear Line (ESC[K)' },
        { value: 'H', key: 'sequence_cursor_home', fallback: 'Cursor Home (ESC[H)' },
        { value: 's', key: 'sequence_cursor_save', fallback: 'Save Cursor (ESC[s)' },
        { value: 'u', key: 'sequence_cursor_restore', fallback: 'Restore Cursor (ESC[u)' },
        { value: '1A', key: 'sequence_cursor_up', fallback: 'Cursor Up (ESC[1A)' },
        { value: '1B', key: 'sequence_cursor_down', fallback: 'Cursor Down (ESC[1B)' },
        { value: '1C', key: 'sequence_cursor_right', fallback: 'Cursor Right (ESC[1C)' },
        { value: '1D', key: 'sequence_cursor_left', fallback: 'Cursor Left (ESC[1D)' }
      ]
    }
  ];

  var CHEATSHEET_ROWS = [
    { sequence: '\x1b[0m', key: 'sequence_reset', fallback: 'Reset (ESC[0m)', sample: '\x1b[1;31mReset sample\x1b[0m normal' },
    { sequence: '\x1b[1m', key: 'sequence_bold', fallback: 'Bold (ESC[1m)', sample: '\x1b[1mBold text\x1b[0m' },
    { sequence: '\x1b[5m', key: 'sequence_blink', fallback: 'Blink (ESC[5m)', sample: '\x1b[5mBlink text\x1b[0m' },
    { sequence: '\x1b[7m', key: 'sequence_reverse', fallback: 'Reverse Video (ESC[7m)', sample: '\x1b[7mReverse text\x1b[0m' },
    { sequence: '\x1b[31m', key: 'sequence_fg_red', fallback: 'Red (ESC[31m)', sample: '\x1b[31mRed text\x1b[0m' },
    { sequence: '\x1b[32m', key: 'sequence_fg_green', fallback: 'Green (ESC[32m)', sample: '\x1b[32mGreen text\x1b[0m' },
    { sequence: '\x1b[33m', key: 'sequence_fg_yellow', fallback: 'Yellow (ESC[33m)', sample: '\x1b[33mYellow text\x1b[0m' },
    { sequence: '\x1b[34m', key: 'sequence_fg_blue', fallback: 'Blue (ESC[34m)', sample: '\x1b[34mBlue text\x1b[0m' },
    { sequence: '\x1b[35m', key: 'sequence_fg_magenta', fallback: 'Magenta (ESC[35m)', sample: '\x1b[35mMagenta text\x1b[0m' },
    { sequence: '\x1b[36m', key: 'sequence_fg_cyan', fallback: 'Cyan (ESC[36m)', sample: '\x1b[36mCyan text\x1b[0m' },
    { sequence: '\x1b[37m', key: 'sequence_fg_white', fallback: 'White (ESC[37m)', sample: '\x1b[37mWhite text\x1b[0m' },
    { sequence: '\x1b[41m', key: 'sequence_bg_red', fallback: 'Red Background (ESC[41m)', sample: '\x1b[41;37m Red bg \x1b[0m' },
    { sequence: '\x1b[42m', key: 'sequence_bg_green', fallback: 'Green Background (ESC[42m)', sample: '\x1b[42;30m Green bg \x1b[0m' },
    { sequence: '\x1b[44m', key: 'sequence_bg_blue', fallback: 'Blue Background (ESC[44m)', sample: '\x1b[44;37m Blue bg \x1b[0m' },
    { sequence: '\x1b[2J', key: 'sequence_clear_screen', fallback: 'Clear Screen (ESC[2J)', sample: 'Clears the screen before following output' },
    { sequence: '\x1b[K', key: 'sequence_clear_line', fallback: 'Clear Line (ESC[K)', sample: 'Clears from cursor to end of line' },
    { sequence: '\x1b[H', key: 'sequence_cursor_home', fallback: 'Cursor Home (ESC[H)', sample: 'Moves cursor to row 1, column 1' },
    { sequence: '\x1b[s', key: 'sequence_cursor_save', fallback: 'Save Cursor (ESC[s)', sample: 'Saves the current cursor position' },
    { sequence: '\x1b[u', key: 'sequence_cursor_restore', fallback: 'Restore Cursor (ESC[u)', sample: 'Restores the saved cursor position' },
    { sequence: '\x1b[10;20H', key: 'example_position', fallback: 'Position Cursor (ESC[10;20H)', sample: 'Moves cursor to row 10, column 20' }
  ];

  function t(key, fallback) {
    var i18n = window.virtbbsComposeI18n || {};
    return i18n[key] || fallback || key;
  }

  function escapeHtml(value) {
    return String(value || '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function normalizeSequenceSuffix(value) {
    var suffix = String(value || '').trim();
    if (!suffix) return '';
    suffix = suffix.replace(/^ESC\[/i, '');
    suffix = suffix.replace(/^\u001b\[/, '');
    suffix = suffix.replace(/^\[/, '');
    return suffix;
  }

  function buildColumnRuler(maxColumns) {
    var width = Math.max(1, Number(maxColumns) || 132);
    var numberLine = new Array(width);
    var tickLine = new Array(width);
    var column;
    for (column = 0; column < width; column++) {
      numberLine[column] = ' ';
      tickLine[column] = '.';
    }
    for (column = 1; column <= width; column++) {
      if (column === 1 || column % 10 === 0) {
        var label = String(column);
        var start = column - 1;
        for (var i = 0; i < label.length && (start + i) < width; i++) {
          numberLine[start + i] = label.charAt(i);
        }
      }
      if (column === 1 || column % 10 === 0) {
        tickLine[column - 1] = '|';
      } else if (column % 5 === 0) {
        tickLine[column - 1] = ':';
      }
    }
    return numberLine.join('') + '\n' + tickLine.join('');
  }

  var previewModalInstance = null;
  var cheatsheetModalInstance = null;

  function ensurePreviewModal() {
    var modal = document.getElementById('ansiEditorPreviewModal');
    if (!modal) {
      modal = document.createElement('div');
      modal.className = 'modal fade';
      modal.id = 'ansiEditorPreviewModal';
      modal.tabIndex = -1;
      modal.innerHTML = '<div class="modal-dialog modal-xl modal-dialog-scrollable"><div class="modal-content">'
        + '<div class="modal-header"><h5 class="modal-title" id="ansiEditorPreviewModalTitle"></h5>'
        + '<button type="button" class="btn-close" data-bs-dismiss="modal"></button></div>'
        + '<div class="modal-body" id="ansiEditorPreviewModalBody"></div>'
        + '<div class="modal-footer"><button type="button" class="btn btn-secondary" data-bs-dismiss="modal">'
        + escapeHtml(t('close', 'Close')) + '</button></div></div></div>';
      document.body.appendChild(modal);
    }
    if (!previewModalInstance && window.bootstrap && window.bootstrap.Modal) {
      previewModalInstance = new window.bootstrap.Modal(modal);
    }
    return {
      title: document.getElementById('ansiEditorPreviewModalTitle'),
      body: document.getElementById('ansiEditorPreviewModalBody')
    };
  }

  function openPreview(options) {
    var modal = ensurePreviewModal();
    if (!modal.body || !modal.title) return;
    modal.title.textContent = (options && options.title) || t('preview', 'Preview');
    if (window.virtbbsAnsiPreview) {
      window.virtbbsAnsiPreview.renderPreview(modal.body, options && options.content ? options.content : '');
    } else {
      modal.body.innerHTML = '<pre class="mb-0">' + escapeHtml(options && options.content ? options.content : '') + '</pre>';
    }
    if (previewModalInstance) previewModalInstance.show();
  }

  function ensureCheatsheetModal() {
    var modal = document.getElementById('ansiEditorCheatsheetModal');
    if (!modal) {
      modal = document.createElement('div');
      modal.className = 'modal fade';
      modal.id = 'ansiEditorCheatsheetModal';
      modal.tabIndex = -1;
      modal.innerHTML = '<div class="modal-dialog modal-xl modal-dialog-scrollable"><div class="modal-content">'
        + '<div class="modal-header"><h5 class="modal-title">' + escapeHtml(t('cheatsheet_title', 'ANSI Cheatsheet')) + '</h5>'
        + '<button type="button" class="btn-close" data-bs-dismiss="modal"></button></div>'
        + '<div class="modal-body" id="ansiEditorCheatsheetModalBody"></div>'
        + '<div class="modal-footer"><button type="button" class="btn btn-secondary" data-bs-dismiss="modal">'
        + escapeHtml(t('close', 'Close')) + '</button></div></div></div>';
      document.body.appendChild(modal);
    }
    if (!cheatsheetModalInstance && window.bootstrap && window.bootstrap.Modal) {
      cheatsheetModalInstance = new window.bootstrap.Modal(modal);
    }
    return document.getElementById('ansiEditorCheatsheetModalBody');
  }

  function renderCheatsheetTable(container) {
    if (!container) return;
    var rowsHtml = CHEATSHEET_ROWS.map(function (row) {
      var label = t(row.key, row.fallback);
      var sampleHtml = (window.virtbbsAnsiPreview && /\x1b\[/.test(row.sample))
        ? '<div class="ansi-screen p-2">' + window.virtbbsAnsiPreview.ansiToHTML(row.sample) + '</div>'
        : '<code>' + escapeHtml(row.sample) + '</code>';
      return '<tr><td><code>' + escapeHtml(row.sequence.replace(/\x1b/g, 'ESC')) + '</code></td>'
        + '<td>' + escapeHtml(label) + '</td><td>' + sampleHtml + '</td></tr>';
    }).join('');
    container.innerHTML = '<p class="meta">' + escapeHtml(t('cheatsheet_help',
      'Use these sequences in the editor. ESC means the escape character (ASCII 27).')) + '</p>'
      + '<table class="table table-sm table-dark"><thead><tr><th>'
      + escapeHtml(t('cheatsheet_sequence', 'Sequence')) + '</th><th>'
      + escapeHtml(t('cheatsheet_description', 'Description')) + '</th><th>'
      + escapeHtml(t('cheatsheet_preview', 'Preview')) + '</th></tr></thead><tbody>' + rowsHtml + '</tbody></table>';
  }

  function openCheatsheet() {
    var body = ensureCheatsheetModal();
    renderCheatsheetTable(body);
    if (cheatsheetModalInstance) cheatsheetModalInstance.show();
  }

  function AnsiEditor(root, options) {
    this.root = root;
    this.options = options || {};
    this.textarea = root ? root.querySelector('textarea') : null;
    this.presetSelect = null;
    this.customInput = null;
    this.rulerTextarea = null;
    this.rulerWrap = null;
    if (!this.root || !this.textarea) return;
    this.buildUi();
    this.bindEvents();
  }

  AnsiEditor.prototype.buildUi = function () {
    var controls = document.createElement('div');
    controls.className = 'ansi-editor-controls row g-2 mb-2';
    controls.innerHTML = '<div class="col-md-4"><select class="form-select form-select-sm" data-ansi-editor-preset></select></div>'
      + '<div class="col-md-3"><input class="form-control form-control-sm font-monospace" data-ansi-editor-custom></div>'
      + '<div class="col-auto"><button type="button" class="btn btn-sm btn-outline-secondary" data-ansi-editor-insert-esc">'
      + escapeHtml(t('insert_escape_prefix', 'Insert ESC[')) + '</button></div>'
      + '<div class="col-auto"><button type="button" class="btn btn-sm btn-outline-secondary" data-ansi-editor-insert-seq">'
      + escapeHtml(t('insert_sequence', 'Insert Sequence')) + '</button></div>'
      + '<div class="col-auto"><button type="button" class="btn btn-sm btn-outline-secondary" data-ansi-editor-cheatsheet-btn">'
      + escapeHtml(t('cheatsheet_title', 'ANSI Cheatsheet')) + '</button></div>'
      + '<div class="col-auto"><button type="button" class="btn btn-sm btn-outline-secondary" data-ansi-editor-insert-file-btn">'
      + escapeHtml(t('insert_file', 'Insert File')) + '</button><input type="file" class="d-none" data-ansi-editor-file-input accept=".ans,.asc,.txt,text/*"></div>'
      + '<div class="col-auto"><button type="button" class="btn btn-sm btn-outline-primary" data-ansi-editor-preview-btn">'
      + escapeHtml(t('preview', 'Preview')) + '</button></div>';
    this.root.insertBefore(controls, this.textarea);

    var rulerWrap = document.createElement('div');
    rulerWrap.className = 'ansi-editor-ruler border rounded mb-2';
    rulerWrap.innerHTML = '<textarea class="ansi-editor-ruler-text font-monospace" readonly tabindex="-1" data-ansi-editor-ruler rows="2"></textarea>';
    this.root.insertBefore(rulerWrap, this.textarea);
    this.rulerWrap = rulerWrap;
    this.rulerTextarea = rulerWrap.querySelector('[data-ansi-editor-ruler]');
    if (this.rulerTextarea) {
      this.rulerTextarea.wrap = 'off';
      var rulerContent = buildColumnRuler(132);
      this.rulerTextarea.value = rulerContent;
      this.rulerTextarea.defaultValue = rulerContent;
    }
    this.textarea.wrap = 'off';
    this.textarea.classList.add('font-monospace');
    this.syncRulerMetrics();

    this.presetSelect = controls.querySelector('[data-ansi-editor-preset]');
    this.customInput = controls.querySelector('[data-ansi-editor-custom]');
    this.customInput.placeholder = t('custom_sequence_placeholder', 'e.g. 10;20H or 44m');
    var placeholder = document.createElement('option');
    placeholder.value = '';
    placeholder.textContent = t('select_sequence', 'Select ANSI sequence');
    this.presetSelect.appendChild(placeholder);
    PRESET_GROUPS.forEach(function (group) {
      var optgroup = document.createElement('optgroup');
      optgroup.label = t(group.labelKey, group.fallback);
      group.options.forEach(function (option) {
        var opt = document.createElement('option');
        opt.value = option.value;
        opt.textContent = t(option.key, option.fallback);
        optgroup.appendChild(opt);
      });
      this.presetSelect.appendChild(optgroup);
    }, this);
  };

  AnsiEditor.prototype.bindEvents = function () {
    var self = this;
    this.root.querySelector('[data-ansi-editor-insert-esc]').addEventListener('click', function () {
      self.insertAtCaret('\x1b[');
    });
    this.root.querySelector('[data-ansi-editor-insert-seq]').addEventListener('click', function () {
      var suffix = normalizeSequenceSuffix(self.customInput.value || self.presetSelect.value);
      if (!suffix) return;
      self.insertAtCaret('\x1b[' + suffix);
      self.customInput.value = '';
      self.presetSelect.value = '';
    });
    this.root.querySelector('[data-ansi-editor-preview-btn]').addEventListener('click', function () {
      self.openPreview();
    });
    this.root.querySelector('[data-ansi-editor-cheatsheet-btn]').addEventListener('click', openCheatsheet);
    var fileInput = this.root.querySelector('[data-ansi-editor-file-input]');
    this.root.querySelector('[data-ansi-editor-insert-file-btn]').addEventListener('click', function () {
      if (fileInput) fileInput.click();
    });
    if (fileInput) {
      fileInput.addEventListener('change', function () {
        var file = fileInput.files && fileInput.files[0];
        if (!file) return;
        var reader = new FileReader();
        reader.onload = function () {
          self.insertAtCaret(typeof reader.result === 'string' ? reader.result : '');
          fileInput.value = '';
        };
        reader.onerror = function () { fileInput.value = ''; };
        reader.readAsText(file);
      });
    }
    this.textarea.addEventListener('scroll', function () { self.syncRulerScroll(); });
    window.addEventListener('resize', function () { self.syncRulerMetrics(); });
  };

  AnsiEditor.prototype.insertAtCaret = function (text) {
    var start = this.textarea.selectionStart != null ? this.textarea.selectionStart : this.textarea.value.length;
    var end = this.textarea.selectionEnd != null ? this.textarea.selectionEnd : this.textarea.value.length;
    this.textarea.value = this.textarea.value.slice(0, start) + text + this.textarea.value.slice(end);
    this.textarea.focus();
    var next = start + text.length;
    this.textarea.setSelectionRange(next, next);
    this.textarea.dispatchEvent(new Event('input', { bubbles: true }));
  };

  AnsiEditor.prototype.syncRulerMetrics = function () {
    if (!this.textarea || !this.rulerTextarea || !this.rulerWrap || !this.textarea.offsetParent) return;
    var styles = window.getComputedStyle(this.textarea);
    ['fontFamily', 'fontSize', 'fontWeight', 'lineHeight', 'letterSpacing', 'tabSize',
      'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft', 'boxSizing'].forEach(function (prop) {
      this.rulerTextarea.style[prop] = styles[prop];
    }, this);
    this.rulerTextarea.style.height = 'calc((' + styles.lineHeight + ' * 2) + ' + styles.paddingTop + ' + ' + styles.paddingBottom + ')';
    this.rulerWrap.scrollLeft = this.textarea.scrollLeft;
  };

  AnsiEditor.prototype.syncRulerScroll = function () {
    if (!this.textarea || !this.rulerWrap) return;
    this.rulerWrap.scrollLeft = this.textarea.scrollLeft;
  };

  AnsiEditor.prototype.openPreview = function () {
    openPreview({ title: t('preview', 'Preview'), content: this.textarea.value || '' });
  };

  window.AnsiEditor = {
    create: function (root, options) {
      if (!root) return null;
      return new AnsiEditor(root, options);
    },
    openPreview: openPreview,
    openCheatsheet: openCheatsheet
  };
})(window, document);
