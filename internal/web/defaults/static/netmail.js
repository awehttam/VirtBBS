(function () {
  var listEl = document.getElementById('netmail-list');
  var paneEl = document.getElementById('netmail-pane');
  if (!listEl || !paneEl) return;

  function esc(s) {
    var d = document.createElement('div');
    d.textContent = s || '';
    return d.innerHTML;
  }

  function loadList() {
    fetch('/api/netmail', { credentials: 'same-origin' })
      .then(function (r) { return r.json(); })
      .then(function (msgs) {
        if (!msgs.length) {
          listEl.innerHTML = '<p class="meta">No netmail.</p>';
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
      });
  }

  function loadMessage(num) {
    fetch('/api/netmail?num=' + encodeURIComponent(num), { credentials: 'same-origin' })
      .then(function (r) { return r.json(); })
      .then(function (m) {
        paneEl.innerHTML = '<h3>' + esc(m.Subject) + '</h3>' +
          '<p class="meta">From ' + esc(m.FromName) + ' · #' + m.MsgNumber + '</p>' +
          '<div class="msg-body">' + esc(m.Body) + '</div>';
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
          body: document.getElementById('nm-body').value
        })
      }).then(function (r) {
        if (!r.ok) throw new Error('send failed');
        return r.json();
      }).then(function () {
        status.textContent = 'Queued for next poll.';
        form.reset();
      }).catch(function () {
        status.textContent = 'Send failed.';
      });
    });
  }

  loadList();
})();
