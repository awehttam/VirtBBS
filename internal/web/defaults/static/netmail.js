(function () {
  'use strict';

  var listEl = document.getElementById('netmail-list');
  var paneEl = document.getElementById('netmail-pane');
  var statsEl = document.getElementById('netmail-stats');
  var abEl = document.getElementById('netmail-addressbook');
  if (!listEl || !paneEl) return;

  var i18n = {};
  var i18nEl = document.getElementById('netmail-i18n');
  if (i18nEl) {
    try { i18n = JSON.parse(i18nEl.textContent); } catch (e) {}
  }
  function t(key, fallback) {
    return i18n[key] || fallback || key;
  }

  var messages = [];
  var filter = 'all';
  var selectedNum = null;
  var taglines = [];

  function esc(s) {
    var d = document.createElement('div');
    d.textContent = s || '';
    return d.innerHTML;
  }

  function formatDate(iso) {
    if (!iso) return '';
    try {
      var d = new Date(iso);
      return d.toLocaleString();
    } catch (e) {
      return iso;
    }
  }

  function bodyHtml(m) {
    var html = m.DisplayBody || '';
    if (html.indexOf('<') >= 0) {
      return '<div class="msg-body msg-body-formatted">' + html + '</div>';
    }
    return '<div class="msg-body">' + esc(html || m.Body) + '</div>';
  }

  function syncComposeEditor() {
    var body = document.getElementById('nm-body');
    if (body) body.dispatchEvent(new Event('input', { bubbles: true }));
  }

  function loadStats() {
    fetch('/api/netmail/stats', { credentials: 'same-origin' })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (st) {
        if (!st || !statsEl) return;
        var tpl = t('netmail.app.stats', 'Total: {total} · Unread: {unread}');
        statsEl.textContent = tpl.replace('{total}', String(st.total)).replace('{unread}', String(st.unread));
      });
  }

  function loadTaglines() {
    fetch('/api/netmail/taglines', { credentials: 'same-origin' })
      .then(function (r) { return r.ok ? r.json() : []; })
      .then(function (lines) {
        taglines = lines || [];
        var sel = document.getElementById('nm-tagline');
        if (!sel) return;
        taglines.forEach(function (line) {
          var opt = document.createElement('option');
          opt.value = line;
          opt.textContent = line.length > 48 ? line.slice(0, 45) + '…' : line;
          sel.appendChild(opt);
        });
      });
  }

  function renderList() {
    var visible = messages.filter(function (m) {
      return filter !== 'unread' || m.Unread;
    });
    if (!visible.length) {
      listEl.innerHTML = '<p class="meta small">' + esc(t('netmail.empty', 'No netmail.')) + '</p>';
      return;
    }
    var html = '<div class="list-group list-group-flush netmail-list-group">';
    visible.slice().reverse().forEach(function (m) {
      var active = selectedNum === m.MsgNumber ? ' active' : '';
      var unread = m.Unread ? ' fw-semibold' : '';
      html += '<button type="button" class="list-group-item list-group-item-action netmail-list-item' + active + unread + '" data-num="' + m.MsgNumber + '">' +
        '<div class="netmail-list-subject text-truncate">' + esc(m.Subject) + '</div>' +
        '<div class="meta small text-truncate">' + esc(m.FromName) + ' · ' + esc(formatDate(m.DatePosted)) + '</div>' +
        '</button>';
    });
    html += '</div>';
    listEl.innerHTML = html;
    listEl.querySelectorAll('[data-num]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        loadMessage(parseInt(btn.getAttribute('data-num'), 10));
      });
    });
  }

  function loadList() {
    fetch('/api/netmail', { credentials: 'same-origin' })
      .then(function (r) {
        if (!r.ok) throw new Error('load failed');
        return r.json();
      })
      .then(function (msgs) {
        messages = msgs || [];
        renderList();
        loadStats();
      })
      .catch(function () {
        listEl.innerHTML = '<p class="meta">' + esc(t('netmail.app.load_failed', 'Could not load netmail.')) + '</p>';
      });
  }

  function showCompose(prefill) {
    var panel = document.getElementById('netmail-compose-panel');
    if (panel) panel.classList.remove('d-none');
    if (prefill) {
      document.getElementById('nm-to-name').value = prefill.to_name || '';
      document.getElementById('nm-to-addr').value = prefill.to_addr || '';
      document.getElementById('nm-subject').value = prefill.subject || '';
      var body = document.getElementById('nm-body');
      if (body) {
        body.value = prefill.body || '';
        body.dispatchEvent(new Event('input', { bubbles: true }));
      }
    }
    panel && panel.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }

  function hideCompose() {
    var panel = document.getElementById('netmail-compose-panel');
    if (panel) panel.classList.add('d-none');
  }

  function renderMessage(m) {
    selectedNum = m.MsgNumber;
    renderList();
    var fromLine = t('netmail.app.from', 'From %s · #%d')
      .replace('%s', m.FromName || '')
      .replace('%d', String(m.MsgNumber));
    var originBlock = '';
    if (m.FidoOrigin) {
      originBlock = '<p class="meta small d-flex flex-wrap align-items-center gap-2 mb-2">' +
        esc(t('read.fido_origin', 'Origin')) + ' <code>' + esc(m.FidoOrigin) + '</code>' +
        '<button type="button" class="btn btn-sm btn-outline-secondary" id="netmail-add-contact-btn">' +
        esc(t('netmail.app.add_to_contacts', 'Add to contacts')) + '</button></p>';
    }
    paneEl.innerHTML =
      '<div class="card-body">' +
      '<div class="d-flex flex-wrap justify-content-between align-items-start gap-2 mb-2">' +
      '<h3 class="h5 mb-0">' + esc(m.Subject) + '</h3>' +
      '<div class="btn-group btn-group-sm">' +
      '<button type="button" class="btn btn-outline-primary" id="netmail-reply-btn">' + esc(t('common.reply', 'Reply')) + '</button>' +
      '<button type="button" class="btn btn-outline-danger" id="netmail-delete-btn">' + esc(t('common.delete', 'Delete')) + '</button>' +
      '</div></div>' +
      '<p class="meta">' + esc(fromLine) +
      ' · ' + esc(t('netmail.app.to_prefix', 'To')) + ' <strong>' + esc(m.ToName) + '</strong>' +
      ' · ' + esc(formatDate(m.DatePosted)) +
      (m.LangLabel ? ' <span class="badge bg-secondary">' + esc(m.LangLabel) + '</span>' : '') +
      '</p>' + originBlock + bodyHtml(m) + '</div>';

    document.getElementById('netmail-reply-btn').addEventListener('click', function () {
      if (m.Reply) showCompose(m.Reply);
    });
    document.getElementById('netmail-delete-btn').addEventListener('click', function () {
      deleteMessage(m.MsgNumber);
    });
    var addContactBtn = document.getElementById('netmail-add-contact-btn');
    if (addContactBtn) {
      addContactBtn.addEventListener('click', function () {
        openContactModal(null, {
          name: m.FromName || '',
          fido_addr: m.FidoOrigin || '',
          email: '',
          notes: '',
          language: m.LangCode || ''
        });
      });
    }

    messages.forEach(function (item) {
      if (item.MsgNumber === m.MsgNumber) item.Unread = false;
    });
    loadStats();
  }

  function loadMessage(num) {
    paneEl.innerHTML = '<div class="card-body"><p class="meta">' + esc(t('common.loading', 'Loading…')) + '</p></div>';
    fetch('/api/netmail?num=' + encodeURIComponent(num), { credentials: 'same-origin' })
      .then(function (r) {
        if (!r.ok) throw new Error('load failed');
        return r.json();
      })
      .then(renderMessage)
      .catch(function () {
        paneEl.innerHTML = '<div class="card-body"><p class="meta">' + esc(t('netmail.app.load_failed', 'Could not load netmail.')) + '</p></div>';
      });
  }

  function deleteMessage(num) {
    if (!window.confirm(t('netmail.app.delete_confirm', 'Delete this message?'))) return;
    fetch('/api/netmail/delete', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ num: num })
    }).then(function (r) {
      if (!r.ok) throw new Error('delete failed');
      messages = messages.filter(function (m) { return m.MsgNumber !== num; });
      selectedNum = null;
      paneEl.innerHTML = '<div class="card-body"><p class="meta mb-0">' + esc(t('netmail.app.select', 'Select a message')) + '</p></div>';
      renderList();
      loadStats();
    }).catch(function () {
      window.alert(t('netmail.app.delete_failed', 'Delete failed.'));
    });
  }

  function contactModal() {
    var el = document.getElementById('netmail-contact-modal');
    if (!el || !window.bootstrap || !window.bootstrap.Modal) return null;
    return window.bootstrap.Modal.getOrCreateInstance(el);
  }

  function openContactModal(entry, prefill) {
    var data = entry || prefill || {};
    var editing = !!(entry && entry.id);
    document.getElementById('netmail-contact-id').value = editing ? String(entry.id) : '';
    document.getElementById('netmail-contact-name').value = data.name || '';
    document.getElementById('netmail-contact-addr').value = data.fido_addr || '';
    document.getElementById('netmail-contact-email').value = data.email || '';
    document.getElementById('netmail-contact-notes').value = data.notes || '';
    document.getElementById('netmail-contact-language').value = data.language || '';
    document.getElementById('netmail-contact-modal-title').textContent = editing
      ? t('edit_contact', 'Edit contact') : t('add_contact', 'Add contact');
    var modal = contactModal();
    if (modal) modal.show();
  }

  function saveContact() {
    var id = parseInt(document.getElementById('netmail-contact-id').value, 10);
    var payload = {
      name: document.getElementById('netmail-contact-name').value.trim(),
      fido_addr: document.getElementById('netmail-contact-addr').value.trim(),
      email: document.getElementById('netmail-contact-email').value.trim(),
      notes: document.getElementById('netmail-contact-notes').value.trim(),
      language: document.getElementById('netmail-contact-language').value.trim()
    };
    if (!payload.name) {
      window.alert(t('name_required', 'Name is required.'));
      return;
    }
    var method = id > 0 ? 'PUT' : 'POST';
    if (id > 0) payload.id = id;
    fetch('/api/addressbook', {
      method: method,
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    }).then(function (r) {
      if (!r.ok) {
        return r.text().then(function (txt) { throw new Error(txt || 'save failed'); });
      }
      return r.json();
    }).then(function () {
      var modal = contactModal();
      if (modal) modal.hide();
      loadAddressBook(abSearch ? abSearch.value.trim() : '');
    }).catch(function (err) {
      window.alert(err.message || t('save_failed', 'Save failed.'));
    });
  }

  function deleteContact(id) {
    if (!window.confirm(t('delete_confirm', 'Delete this contact?'))) return;
    fetch('/api/addressbook?id=' + encodeURIComponent(id), {
      method: 'DELETE',
      credentials: 'same-origin'
    }).then(function (r) {
      if (!r.ok) throw new Error('delete failed');
      loadAddressBook(abSearch ? abSearch.value.trim() : '');
    }).catch(function () {
      window.alert(t('delete_failed', 'Delete failed.'));
    });
  }

  function loadAddressBook(q) {
    if (!abEl) return;
    var url = '/api/addressbook';
    if (q) url += '?q=' + encodeURIComponent(q);
    fetch(url, { credentials: 'same-origin' })
      .then(function (r) { return r.ok ? r.json() : []; })
      .then(function (entries) {
        if (!entries.length) {
          abEl.innerHTML = '<p class="meta small">' + esc(t('addressbook.empty', 'No contacts.')) + '</p>';
          return;
        }
        var html = '<div class="list-group list-group-flush">';
        entries.forEach(function (e) {
          html += '<div class="list-group-item py-2 netmail-ab-row">' +
            '<div class="d-flex align-items-start gap-2">' +
            '<button type="button" class="btn btn-link p-0 text-start flex-grow-1 netmail-ab-compose">' +
            '<div class="small fw-semibold text-truncate">' + esc(e.name) + '</div>' +
            '<div class="meta small text-truncate"><code>' + esc(e.fido_addr) + '</code></div>' +
            (e.language ? '<div class="meta small">' + esc(e.language) + '</div>' : '') +
            '</button>' +
            '<div class="btn-group btn-group-sm flex-shrink-0">' +
            '<button type="button" class="btn btn-outline-secondary netmail-ab-edit" title="' + esc(t('edit', 'Edit')) + '">✎</button>' +
            '<button type="button" class="btn btn-outline-danger netmail-ab-delete" title="' + esc(t('delete', 'Delete')) + '">×</button>' +
            '</div></div></div>';
        });
        html += '</div>';
        abEl.innerHTML = html;
        abEl.querySelectorAll('.netmail-ab-row').forEach(function (row, i) {
          var e = entries[i];
          row.querySelector('.netmail-ab-compose').addEventListener('click', function () {
            showCompose({ to_name: e.name, to_addr: e.fido_addr, subject: '', body: '' });
          });
          row.querySelector('.netmail-ab-edit').addEventListener('click', function () {
            openContactModal(e);
          });
          row.querySelector('.netmail-ab-delete').addEventListener('click', function () {
            deleteContact(e.id);
          });
        });
      });
  }

  function searchNodelist() {
    var network = document.getElementById('nm-nodelist-network').value;
    var q = document.getElementById('nm-nodelist-query').value.trim();
    var results = document.getElementById('netmail-nodelist-results');
    results.innerHTML = '<p class="meta">' + esc(t('common.loading', 'Loading…')) + '</p>';
    fetch('/api/nodelist/search?network=' + encodeURIComponent(network) + '&q=' + encodeURIComponent(q), { credentials: 'same-origin' })
      .then(function (r) { return r.ok ? r.json() : []; })
      .then(function (nodes) {
        if (!nodes.length) {
          results.innerHTML = '<p class="meta">' + esc(t('netmail.app.nodelist_empty', 'No nodes found.')) + '</p>';
          return;
        }
        var html = '<div class="table-responsive"><table class="table table-sm table-hover align-middle mb-0"><thead><tr>' +
          '<th>' + esc(t('nodelist.col.address', 'Address')) + '</th>' +
          '<th>' + esc(t('common.name', 'Name')) + '</th>' +
          '<th>' + esc(t('nodelist.col.sysop', 'Sysop')) + '</th><th></th></tr></thead><tbody>';
        nodes.forEach(function (n, idx) {
          html += '<tr><td><code>' + esc(n.addr) + '</code></td><td>' + esc(n.name) + '</td><td>' + esc(n.sysop) + '</td>' +
            '<td><button type="button" class="btn btn-sm btn-outline-primary nm-pick-node">' +
            esc(t('netmail.app.use_contact', 'Use')) + '</button></td></tr>';
        });
        html += '</tbody></table></div>';
        results.innerHTML = html;
        results.querySelectorAll('.nm-pick-node').forEach(function (btn, i) {
          var n = nodes[i];
          btn.addEventListener('click', function () {
            document.getElementById('nm-to-name').value = n.sysop || n.name || '';
            document.getElementById('nm-to-addr').value = n.addr || '';
            var modal = bootstrap.Modal.getInstance(document.getElementById('netmail-nodelist-modal'));
            if (modal) modal.hide();
            showCompose();
          });
        });
      });
  }

  document.querySelectorAll('#netmail-filters [data-filter]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      filter = btn.getAttribute('data-filter');
      document.querySelectorAll('#netmail-filters .btn').forEach(function (b) { b.classList.remove('active'); });
      btn.classList.add('active');
      renderList();
    });
  });

  var composeBtn = document.getElementById('netmail-compose-btn');
  if (composeBtn) composeBtn.addEventListener('click', function () { showCompose(); });

  var composeClose = document.getElementById('netmail-compose-close');
  if (composeClose) composeClose.addEventListener('click', hideCompose);

  var abSearch = document.getElementById('netmail-ab-search');
  if (abSearch) {
    var abTimer;
    abSearch.addEventListener('input', function () {
      clearTimeout(abTimer);
      abTimer = setTimeout(function () { loadAddressBook(abSearch.value.trim()); }, 250);
    });
  }

  var abAddBtn = document.getElementById('netmail-ab-add');
  if (abAddBtn) abAddBtn.addEventListener('click', function () { openContactModal(); });

  var contactSave = document.getElementById('netmail-contact-save');
  if (contactSave) contactSave.addEventListener('click', saveContact);

  var nodelistBtn = document.getElementById('nm-nodelist-btn');
  if (nodelistBtn) {
    nodelistBtn.addEventListener('click', function () {
      var modal = new bootstrap.Modal(document.getElementById('netmail-nodelist-modal'));
      modal.show();
    });
  }
  var nodelistSearch = document.getElementById('nm-nodelist-search');
  if (nodelistSearch) nodelistSearch.addEventListener('click', searchNodelist);
  var nodelistQuery = document.getElementById('nm-nodelist-query');
  if (nodelistQuery) {
    nodelistQuery.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') { e.preventDefault(); searchNodelist(); }
    });
  }

  var form = document.getElementById('netmail-compose');
  if (form) {
    form.addEventListener('submit', function (e) {
      e.preventDefault();
      syncComposeEditor();
      var status = document.getElementById('nm-compose-status');
      var bodyText = document.getElementById('nm-body').value;
      var tagline = document.getElementById('nm-tagline').value;
      if (tagline) {
        bodyText = bodyText.replace(/\s+$/, '') + '\r\n\r\n-- \r\n' + tagline + '\r\n';
      }
      fetch('/api/netmail/compose', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          to_name: document.getElementById('nm-to-name').value,
          to_addr: document.getElementById('nm-to-addr').value,
          subject: document.getElementById('nm-subject').value,
          body: bodyText,
          crash: document.getElementById('nm-crash').checked
        })
      }).then(function (r) {
        if (!r.ok) throw new Error('send failed');
        return r.json();
      }).then(function () {
        status.textContent = t('netmail.app.queued', 'Queued for next poll.');
        form.reset();
        var bodyEl = document.getElementById('nm-body');
        if (bodyEl) {
          bodyEl.value = '';
          bodyEl.dispatchEvent(new Event('input', { bubbles: true }));
        }
        hideCompose();
      }).catch(function () {
        status.textContent = t('netmail.app.send_failed', 'Send failed.');
      });
    });
  }

  var params = new URLSearchParams(window.location.search);
  var openNum = parseInt(params.get('num') || '0', 10);

  loadList();
  loadAddressBook('');
  loadTaglines();
  if (openNum > 0) loadMessage(openNum);
})();
