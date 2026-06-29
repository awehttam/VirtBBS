(function () {
  var listEl = document.getElementById('netmail-list');
  var paneEl = document.getElementById('netmail-pane');
  if (!listEl || !paneEl) return;

  var i18nEl = document.getElementById('netmail-i18n');
  var i18n = { empty: 'No netmail.', from: 'From %s · #%d', queued: 'Queued for next poll.', sendFailed: 'Send failed.', loadFailed: 'Could not load netmail.' };
  if (i18nEl) {
    try { i18n = JSON.parse(i18nEl.textContent); } catch (e) {}
  }

  function esc(s) {
    var d = document.createElement('div');
    d.textContent = s || '';
    return d.innerHTML;
  }

  function formatFrom(name, num) {
    var tpl = i18n.from || 'From %s · #%d';
    return tpl.replace('%s', name || '').replace('%d', String(num));
  }

  function loadList() {
    fetch('/api/netmail', { credentials: 'same-origin' })
      .then(function (r) {
        if (!r.ok) throw new Error('load failed');
        return r.json();
      })
      .then(function (msgs) {
        if (!msgs || !msgs.length) {
          listEl.innerHTML = '<p class="meta">' + esc(i18n.empty) + '</p>';
          return;
        }
        var html = '<ul class="netmail-items">';
        msgs.forEach(function (m) {
          html += '<li><a href="#" data-num="' + m.MsgNumber + '">' +
            esc(m.FromName) + ': ' + esc(m.Subject) + '</a></li>';
        });
        html += '</ul>';
        listEl.innerHTML = html;
        listEl.querySelectorAll('a[data-num]').forEach(function (a) {
          a.addEventListener('click', function (e) {
            e.preventDefault();
            loadMessage(a.getAttribute('data-num'));
          });
        });
      })
      .catch(function () {
        listEl.innerHTML = '<p class="meta">' + esc(i18n.loadFailed) + '</p>';
      });
  }

  function loadMessage(num) {
    fetch('/api/netmail?num=' + encodeURIComponent(num), { credentials: 'same-origin' })
      .then(function (r) {
        if (!r.ok) throw new Error('load failed');
        return r.json();
      })
      .then(function (m) {
        var bodyHtml = m.DisplayBody || '';
        var bodyBlock = bodyHtml.indexOf('<') >= 0
          ? '<div class="msg-body msg-body-formatted">' + bodyHtml + '</div>'
          : '<div class="msg-body">' + esc(bodyHtml || m.Body) + '</div>';
        paneEl.innerHTML = '<h3>' + esc(m.Subject) + '</h3>' +
          '<p class="meta">' + esc(formatFrom(m.FromName, m.MsgNumber)) +
          (m.LangLabel ? ' <span class="badge bg-secondary">' + esc(m.LangLabel) + '</span>' : '') +
          '</p>' + bodyBlock;
      })
      .catch(function () {
        paneEl.innerHTML = '<p class="meta">' + esc(i18n.loadFailed) + '</p>';
      });
  }

  var form = document.getElementById('netmail-compose');
  if (form) {
    form.addEventListener('submit', function (e) {
      e.preventDefault();
      var status = document.getElementById('nm-compose-status');
      fetch('/api/netmail/compose', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          to_name: document.getElementById('nm-to-name').value,
          to_addr: document.getElementById('nm-to-addr').value,
          subject: document.getElementById('nm-subject').value,
          body: document.getElementById('nm-body').value,
          crash: document.getElementById('nm-crash').checked
        })
      }).then(function (r) {
        if (!r.ok) throw new Error('send failed');
        return r.json();
      }).then(function () {
        status.textContent = i18n.queued;
        form.reset();
      }).catch(function () {
        status.textContent = i18n.sendFailed;
      });
    });
  }

  loadList();
})();
