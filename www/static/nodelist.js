(function () {
  'use strict';

  var booted = false;

  function boot() {
    if (booted) {
      return;
    }
    var cfg = (function () {
      var el = document.getElementById('nodelist-page-config');
      if (el) {
        try {
          return JSON.parse(el.textContent);
        } catch (e) {
          return window.nodelistPage || {};
        }
      }
      return window.nodelistPage || {};
    })();
    var i18n = cfg.i18n || {};
    var modalEl = document.getElementById('nodelist-detail-modal');
    if (!modalEl || typeof bootstrap === 'undefined') {
      return;
    }
    booted = true;

    var modal = bootstrap.Modal.getOrCreateInstance(modalEl);
    var bodyEl = document.getElementById('nodelist-detail-body');
    var footerEl = document.getElementById('nodelist-detail-footer');
    var titleEl = document.getElementById('nodelist-detail-title');
    var currentNode = null;
    var editMode = false;

    function esc(s) {
      if (s === null || s === undefined) {
        return '';
      }
      var d = document.createElement('div');
      d.textContent = String(s);
      return d.innerHTML;
    }

    function detailRow(label, value) {
      return '<div class="nodelist-detail-row"><span class="nodelist-detail-label">' + esc(label) +
        '</span><span class="nodelist-detail-value">' + (value ? esc(value) : '—') + '</span></div>';
    }

    function renderFlagDetails(flags) {
      if (!flags || !flags.length) {
        return '';
      }
      var html = '<div class="nodelist-detail-row"><span class="nodelist-detail-label">' +
        esc(i18n.capabilities || 'Capabilities') + '</span><div class="nodelist-detail-value">';
      flags.forEach(function (f) {
        html += '<div class="nodelist-flag-item"><strong>' + esc(f.code) + '</strong> — ' + esc(f.description);
        if (f.value) {
          html += ' <span class="meta">(' + esc(f.value) + ')</span>';
        }
        html += '</div>';
      });
      html += '</div></div>';
      return html;
    }

    function renderView(node) {
      var html = '';
      html += detailRow(i18n.network, node.network);
      html += detailRow(i18n.address, node.address);
      if (node.aka) {
        html += detailRow(i18n.aka, node.aka);
      }
      html += detailRow(i18n.name, node.name);
      html += detailRow(i18n.location, node.location);
      html += detailRow(i18n.sysop, node.sysop);
      html += detailRow(i18n.phone, node.phone);
      html += detailRow(i18n.baud, node.baud ? String(node.baud) : '');
      html += detailRow(i18n.type, node.type);
      html += detailRow(i18n.active, node.active ? i18n.yes : i18n.no);
      html += detailRow(i18n.flags, node.flags);
      html += renderFlagDetails(node.flag_details);
      return html;
    }

    function renderEditForm(node) {
      var types = ['Node', 'Host', 'Hub', 'Region', 'Zone', 'Pvt', 'Hold', 'Down', 'Boss'];
      var typeOpts = types.map(function (t) {
        var sel = (node.type || 'Node') === t ? ' selected' : '';
        return '<option value="' + esc(t) + '"' + sel + '>' + esc(t) + '</option>';
      }).join('');
      return '<form id="nodelist-edit-form">' +
        '<input type="hidden" name="network" value="' + esc(node.network) + '">' +
        '<input type="hidden" name="action" value="save_node">' +
        '<input type="hidden" name="q" value="' + esc(cfg.query || '') + '">' +
        detailRow(i18n.address, node.address) +
        '<input type="hidden" name="address" value="' + esc(node.address) + '">' +
        '<div class="nodelist-detail-row"><label class="nodelist-detail-label" for="nl-name">' + esc(i18n.name) +
        '</label><input class="form-control" id="nl-name" name="name" value="' + esc(node.name) + '"></div>' +
        '<div class="nodelist-detail-row"><label class="nodelist-detail-label" for="nl-location">' + esc(i18n.location) +
        '</label><input class="form-control" id="nl-location" name="location" value="' + esc(node.location) + '"></div>' +
        '<div class="nodelist-detail-row"><label class="nodelist-detail-label" for="nl-sysop">' + esc(i18n.sysop) +
        '</label><input class="form-control" id="nl-sysop" name="sysop" value="' + esc(node.sysop) + '"></div>' +
        '<div class="nodelist-detail-row"><label class="nodelist-detail-label" for="nl-phone">' + esc(i18n.phone) +
        '</label><input class="form-control" id="nl-phone" name="phone" value="' + esc(node.phone) + '"></div>' +
        '<div class="nodelist-detail-row"><label class="nodelist-detail-label" for="nl-baud">' + esc(i18n.baud) +
        '</label><input class="form-control" id="nl-baud" name="baud" type="number" value="' + esc(node.baud || '') + '"></div>' +
        '<div class="nodelist-detail-row"><label class="nodelist-detail-label" for="nl-type">' + esc(i18n.type) +
        '</label><select class="form-select" id="nl-type" name="type">' + typeOpts + '</select></div>' +
        '<div class="nodelist-detail-row"><label class="nodelist-detail-label" for="nl-flags">' + esc(i18n.flags) +
        '</label><input class="form-control" id="nl-flags" name="flags" value="' + esc(node.flags) + '">' +
        (i18n.flags_help ? '<span class="form-text">' + esc(i18n.flags_help) + '</span>' : '') + '</div>' +
        '<div class="nodelist-detail-row form-check">' +
        '<input class="form-check-input" type="checkbox" name="active" value="1" id="nl-active"' +
        (node.active ? ' checked' : '') + '>' +
        '<label class="form-check-label" for="nl-active">' + esc(i18n.active) + '</label></div>' +
        (i18n.commit ? '<div class="nodelist-detail-row form-check">' +
        '<input class="form-check-input" type="checkbox" name="commit" value="1" id="nl-commit">' +
        '<label class="form-check-label" for="nl-commit">' + esc(i18n.commit) + '</label></div>' : '') +
        renderFlagDetails(node.flag_details) +
        '</form>';
    }

    function setFooterView() {
      footerEl.innerHTML = '';
      var closeBtn = document.createElement('button');
      closeBtn.type = 'button';
      closeBtn.className = 'btn btn-secondary';
      closeBtn.setAttribute('data-bs-dismiss', 'modal');
      closeBtn.textContent = i18n.close || 'Close';
      footerEl.appendChild(closeBtn);
      if (cfg.editable) {
        var editBtn = document.createElement('button');
        editBtn.type = 'button';
        editBtn.className = 'btn btn-primary';
        editBtn.textContent = i18n.edit || 'Edit';
        editBtn.addEventListener('click', function () {
          editMode = true;
          bodyEl.innerHTML = renderEditForm(currentNode);
          setFooterEdit();
        });
        footerEl.appendChild(editBtn);
      }
    }

    function setFooterEdit() {
      footerEl.innerHTML = '';
      var cancelBtn = document.createElement('button');
      cancelBtn.type = 'button';
      cancelBtn.className = 'btn btn-secondary';
      cancelBtn.textContent = i18n.close || 'Close';
      cancelBtn.addEventListener('click', function () {
        editMode = false;
        bodyEl.innerHTML = renderView(currentNode);
        setFooterView();
      });
      var saveBtn = document.createElement('button');
      saveBtn.type = 'button';
      saveBtn.className = 'btn btn-primary';
      saveBtn.textContent = i18n.save || 'Save';
      saveBtn.addEventListener('click', submitEdit);
      footerEl.appendChild(cancelBtn);
      footerEl.appendChild(saveBtn);
    }

    function submitEdit() {
      var form = document.getElementById('nodelist-edit-form');
      if (!form || !cfg.saveUrl) {
        return;
      }
      form.method = 'post';
      form.action = cfg.saveUrl;
      form.submit();
    }

    function loadNode(network, addr, startEdit) {
      editMode = !!startEdit;
      titleEl.textContent = i18n.loading || 'Loading…';
      bodyEl.innerHTML = '<p class="meta">' + esc(i18n.loading || 'Loading…') + '</p>';
      footerEl.innerHTML = '';
      modal.show();
      var url = cfg.apiUrl + '?network=' + encodeURIComponent(network) + '&addr=' + encodeURIComponent(addr);
      fetch(url, { credentials: 'same-origin' })
        .then(function (r) {
          if (!r.ok) {
            throw new Error('load failed');
          }
          return r.json();
        })
        .then(function (node) {
          currentNode = node;
          titleEl.textContent = (node.address || addr) + (node.name ? ' — ' + node.name : '');
          if (editMode && cfg.editable) {
            bodyEl.innerHTML = renderEditForm(node);
            setFooterEdit();
          } else {
            bodyEl.innerHTML = renderView(node);
            setFooterView();
          }
        })
        .catch(function () {
          titleEl.textContent = i18n.load_error || 'Error';
          bodyEl.innerHTML = '<p class="alert alert-danger">' + esc(i18n.load_error || 'Could not load node.') + '</p>';
          footerEl.innerHTML = '';
          var closeBtn = document.createElement('button');
          closeBtn.type = 'button';
          closeBtn.className = 'btn btn-secondary';
          closeBtn.setAttribute('data-bs-dismiss', 'modal');
          closeBtn.textContent = i18n.close || 'Close';
          footerEl.appendChild(closeBtn);
        });
    }

    document.querySelectorAll('.nodelist-view-btn').forEach(function (btn) {
      btn.addEventListener('click', function () {
        loadNode(btn.getAttribute('data-network'), btn.getAttribute('data-addr'), false);
      });
    });

    document.querySelectorAll('.nodelist-edit-btn').forEach(function (btn) {
      btn.addEventListener('click', function () {
        loadNode(btn.getAttribute('data-network'), btn.getAttribute('data-addr'), true);
      });
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
