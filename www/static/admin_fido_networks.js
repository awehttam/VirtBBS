(function (window, document) {
  'use strict';

  var state = {
    network: '',
    data: null,
    i18n: {}
  };

  function t(key, fallback) {
    return state.i18n[key] || fallback || key;
  }

  function loadI18n() {
    var el = document.getElementById('admin-fido-networks-i18n');
    if (!el) return {};
    try { return JSON.parse(el.textContent); } catch (e) { return {}; }
  }

  function showAlert(message, isError) {
    var box = document.getElementById('fido-mappings-alert');
    if (!box) return;
    box.textContent = message;
    box.classList.remove('d-none', 'alert-success', 'alert-danger');
    box.classList.add(isError ? 'alert-danger' : 'alert-success');
    window.setTimeout(function () { box.classList.add('d-none'); }, 4000);
  }

  function apiFetch(url, options) {
    return fetch(url, options).then(function (res) {
      return res.json().then(function (body) {
        if (!res.ok) {
          var err = new Error((body && body.error) || res.statusText);
          err.status = res.status;
          throw err;
        }
        return body;
      });
    });
  }

  function modal(id) {
    var el = document.getElementById(id);
    if (!el || !window.bootstrap || !window.bootstrap.Modal) return null;
    return window.bootstrap.Modal.getOrCreateInstance(el);
  }

  function fillSelect(select, items, selectedId) {
    if (!select) return;
    select.innerHTML = '';
    (items || []).forEach(function (item) {
      var opt = document.createElement('option');
      opt.value = String(item.id);
      opt.textContent = item.id + ' — ' + item.name;
      if (selectedId != null && Number(item.id) === Number(selectedId)) {
        opt.selected = true;
      }
      select.appendChild(opt);
    });
  }

  function actionButtons(kind, key, label) {
    return '<div class="btn-group btn-group-sm" role="group">'
      + '<button type="button" class="btn btn-outline-secondary" data-fido-edit="' + kind + '" data-key="' + escapeAttr(key) + '">' + escapeHtml(t('edit', 'Edit')) + '</button>'
      + '<button type="button" class="btn btn-outline-danger" data-fido-delete="' + kind + '" data-key="' + escapeAttr(key) + '">' + escapeHtml(t('delete', 'Delete')) + '</button>'
      + '</div>';
  }

  function escapeHtml(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function escapeAttr(s) {
    return String(s || '').replace(/&/g, '&amp;').replace(/"/g, '&quot;');
  }

  function renderLists() {
    var d = state.data;
    if (!d) return;

    var echoList = document.getElementById('echo-areas-list');
    var fileList = document.getElementById('file-areas-list');
    var dlList = document.getElementById('downlinks-list');
    echoList.innerHTML = '';
    fileList.innerHTML = '';
    dlList.innerHTML = '';

    (d.echo_areas || []).forEach(function (row) {
      var li = document.createElement('li');
      li.className = 'list-group-item d-flex align-items-center justify-content-between gap-2';
      var label = row.conf_name ? row.tag + ' → ' + row.conf_id + ' (' + row.conf_name + ')' : row.tag + ' → ' + row.conf_id;
      li.innerHTML = '<span class="font-monospace small text-break">' + escapeHtml(label) + '</span>' + actionButtons('echo', row.tag, label);
      echoList.appendChild(li);
    });

    (d.file_areas || []).forEach(function (row) {
      var li = document.createElement('li');
      li.className = 'list-group-item d-flex align-items-center justify-content-between gap-2';
      var label = row.dir_name ? row.tag + ' → ' + row.dir_id + ' (' + row.dir_name + ')' : row.tag + ' → ' + row.dir_id;
      li.innerHTML = '<span class="font-monospace small text-break">' + escapeHtml(label) + '</span>' + actionButtons('file', row.tag, label);
      fileList.appendChild(li);
    });

    (d.downlinks || []).forEach(function (row) {
      var li = document.createElement('li');
      li.className = 'list-group-item d-flex align-items-center justify-content-between gap-2';
      var label = row.name + ' · ' + row.address;
      li.innerHTML = '<span class="small text-break"><strong>' + escapeHtml(row.name) + '</strong><br><code class="small">' + escapeHtml(row.address) + '</code></span>'
        + actionButtons('downlink', row.address, label);
      dlList.appendChild(li);
    });

    toggleEmpty('echo-areas-empty', !(d.echo_areas && d.echo_areas.length));
    toggleEmpty('file-areas-empty', !(d.file_areas && d.file_areas.length));
    toggleEmpty('downlinks-empty', !(d.downlinks && d.downlinks.length));
  }

  function toggleEmpty(id, show) {
    var el = document.getElementById(id);
    if (el) el.classList.toggle('d-none', !show);
  }

  function reload() {
    return apiFetch('/api/admin/fido/networks/mappings?network=' + encodeURIComponent(state.network))
      .then(function (data) {
        state.data = data;
        renderLists();
      });
  }

  function findEcho(tag) {
    return (state.data.echo_areas || []).find(function (r) { return r.tag === tag; });
  }

  function findFile(tag) {
    return (state.data.file_areas || []).find(function (r) { return r.tag === tag; });
  }

  function findDownlink(addr) {
    return (state.data.downlinks || []).find(function (r) { return r.address === addr; });
  }

  function openEchoModal(editTag) {
    var row = editTag ? findEcho(editTag) : null;
    document.getElementById('fido-echo-old-tag').value = editTag || '';
    document.getElementById('fido-echo-tag').value = row ? row.tag : '';
    document.getElementById('fido-echo-modal-title').textContent = row
      ? t('edit_echo', 'Edit echo area') : t('add_echo', 'Add echo area');
    fillSelect(document.getElementById('fido-echo-conf'), state.data.conferences, row ? row.conf_id : null);
    modal('fido-echo-modal').show();
  }

  function openFileModal(editTag) {
    var row = editTag ? findFile(editTag) : null;
    document.getElementById('fido-file-old-tag').value = editTag || '';
    document.getElementById('fido-file-tag').value = row ? row.tag : '';
    document.getElementById('fido-file-modal-title').textContent = row
      ? t('edit_file', 'Edit file area') : t('add_file', 'Add file area');
    fillSelect(document.getElementById('fido-file-dir'), state.data.file_dirs, row ? row.dir_id : null);
    modal('fido-file-modal').show();
  }

  function openDownlinkModal(editAddr) {
    var row = editAddr ? findDownlink(editAddr) : null;
    document.getElementById('fido-downlink-mode').value = row ? 'update' : 'add';
    document.getElementById('fido-downlink-name').value = row ? row.name : '';
    document.getElementById('fido-downlink-addr').value = row ? row.address : '';
    document.getElementById('fido-downlink-addr').readOnly = !!row;
    document.getElementById('fido-downlink-pw').value = '';
    document.getElementById('fido-downlink-pw-hint').classList.toggle('d-none', !row);
    document.getElementById('fido-downlink-modal-title').textContent = row
      ? t('edit_downlink', 'Edit downlink') : t('add_downlink', 'Add downlink');
    modal('fido-downlink-modal').show();
  }

  function saveEcho() {
    var tag = document.getElementById('fido-echo-tag').value.trim();
    var confId = parseInt(document.getElementById('fido-echo-conf').value, 10);
    var oldTag = document.getElementById('fido-echo-old-tag').value.trim();
    return apiFetch('/api/admin/fido/networks/areas', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ network: state.network, tag: tag, old_tag: oldTag, conf_id: confId })
    }).then(function (res) {
      modal('fido-echo-modal').hide();
      showAlert(res.message || t('echo_saved', 'Echo area saved.'));
      return reload();
    }).catch(function (err) { showAlert(err.message, true); });
  }

  function saveFile() {
    var tag = document.getElementById('fido-file-tag').value.trim();
    var dirId = parseInt(document.getElementById('fido-file-dir').value, 10);
    var oldTag = document.getElementById('fido-file-old-tag').value.trim();
    return apiFetch('/api/admin/fido/networks/file-areas', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ network: state.network, tag: tag, old_tag: oldTag, dir_id: dirId })
    }).then(function (res) {
      modal('fido-file-modal').hide();
      showAlert(res.message || t('file_saved', 'File area saved.'));
      return reload();
    }).catch(function (err) { showAlert(err.message, true); });
  }

  function saveDownlink() {
    var mode = document.getElementById('fido-downlink-mode').value;
    return apiFetch('/api/admin/fido/networks/downlinks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        network: state.network,
        action: mode,
        name: document.getElementById('fido-downlink-name').value.trim(),
        address: document.getElementById('fido-downlink-addr').value.trim(),
        password: document.getElementById('fido-downlink-pw').value.trim()
      })
    }).then(function (res) {
      modal('fido-downlink-modal').hide();
      showAlert(res.message || t('flash_added', 'Downlink added.'));
      return reload();
    }).catch(function (err) { showAlert(err.message, true); });
  }

  function deleteEcho(tag) {
    if (!window.confirm(t('delete_echo', 'Remove this echo area mapping?'))) return;
    apiFetch('/api/admin/fido/networks/areas?network=' + encodeURIComponent(state.network) + '&tag=' + encodeURIComponent(tag), { method: 'DELETE' })
      .then(function (res) { showAlert(res.message); return reload(); })
      .catch(function (err) { showAlert(err.message, true); });
  }

  function deleteFile(tag) {
    if (!window.confirm(t('delete_file', 'Remove this file area mapping?'))) return;
    apiFetch('/api/admin/fido/networks/file-areas?network=' + encodeURIComponent(state.network) + '&tag=' + encodeURIComponent(tag), { method: 'DELETE' })
      .then(function (res) { showAlert(res.message); return reload(); })
      .catch(function (err) { showAlert(err.message, true); });
  }

  function deleteDownlink(addr) {
    if (!window.confirm(t('confirm_remove', 'Remove this downlink and clear its subscriptions?'))) return;
    apiFetch('/api/admin/fido/networks/downlinks?network=' + encodeURIComponent(state.network) + '&address=' + encodeURIComponent(addr), { method: 'DELETE' })
      .then(function (res) { showAlert(res.message); return reload(); })
      .catch(function (err) { showAlert(err.message, true); });
  }

  function bindEvents() {
    document.querySelectorAll('[data-fido-action]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var action = btn.getAttribute('data-fido-action');
        if (action === 'add-echo') openEchoModal();
        if (action === 'add-file') openFileModal();
        if (action === 'add-downlink') openDownlinkModal();
      });
    });

    document.getElementById('fido-echo-save').addEventListener('click', saveEcho);
    document.getElementById('fido-file-save').addEventListener('click', saveFile);
    document.getElementById('fido-downlink-save').addEventListener('click', saveDownlink);

    var app = document.getElementById('fido-mappings-app');
    app.addEventListener('click', function (e) {
      var editBtn = e.target.closest('[data-fido-edit]');
      if (editBtn) {
        var kind = editBtn.getAttribute('data-fido-edit');
        var key = editBtn.getAttribute('data-key');
        if (kind === 'echo') openEchoModal(key);
        if (kind === 'file') openFileModal(key);
        if (kind === 'downlink') openDownlinkModal(key);
        return;
      }
      var delBtn = e.target.closest('[data-fido-delete]');
      if (delBtn) {
        var kind2 = delBtn.getAttribute('data-fido-delete');
        var key2 = delBtn.getAttribute('data-key');
        if (kind2 === 'echo') deleteEcho(key2);
        if (kind2 === 'file') deleteFile(key2);
        if (kind2 === 'downlink') deleteDownlink(key2);
      }
    });
  }

  function boot() {
    var app = document.getElementById('fido-mappings-app');
    if (!app) return;
    state.network = app.getAttribute('data-network') || '';
    state.i18n = loadI18n();
    if (!state.network) return;
    bindEvents();
    reload().catch(function (err) { showAlert(err.message, true); });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})(window, document);
