(function () {
  function setBadge(id, n) {
    var el = document.getElementById(id);
    if (!el) return;
    if (n > 0) {
      el.textContent = n > 99 ? '99+' : String(n);
      el.hidden = false;
    } else {
      el.hidden = true;
    }
  }

  function applyNotify(d) {
    if (!d || d.error) return;
    setBadge('nav-msg-badge', d.new_messages || 0);
    setBadge('nav-net-badge', d.netmail || 0);
  }

  function poll() {
    fetch('/api/notify', { credentials: 'same-origin' })
      .then(function (r) { return r.json(); })
      .then(applyNotify)
      .catch(function () {});
  }

  if (typeof EventSource !== 'undefined') {
    try {
      var es = new EventSource('/api/stream');
      es.addEventListener('notify', function (e) {
        try { applyNotify(JSON.parse(e.data)); } catch (err) {}
      });
      es.onerror = function () {
        es.close();
        poll();
        setInterval(poll, 60000);
      };
    } catch (err) {
      poll();
      setInterval(poll, 60000);
    }
  } else {
    poll();
    setInterval(poll, 60000);
  }
})();
