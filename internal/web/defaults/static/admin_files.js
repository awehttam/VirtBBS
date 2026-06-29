(function () {
  const modal = document.getElementById('file-desc-modal');
  if (!modal) return;

  modal.addEventListener('show.bs.modal', function (event) {
    const btn = event.relatedTarget;
    if (!btn || !btn.classList.contains('file-desc-edit')) return;

    const row = btn.closest('tr');
    const descEl = row ? row.querySelector('.file-desc-display') : null;

    document.getElementById('file-desc-dir-id').value = btn.getAttribute('data-dir-id') || '';
    const filename = btn.getAttribute('data-filename') || '';
    document.getElementById('file-desc-filename-input').value = filename;
    document.getElementById('file-desc-filename').textContent = filename;
    document.getElementById('file-desc-area').textContent = btn.getAttribute('data-dir-name') || '';
    document.getElementById('file-desc-text').value = descEl ? descEl.textContent : '';
  });
})();
