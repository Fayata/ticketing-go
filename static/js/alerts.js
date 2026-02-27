/* global: showCustomAlert, showCustomConfirm — modal alert/confirm (dashboard, staff, admin) */
function showCustomAlert(message) {
    var existing = document.getElementById('custom-alert-overlay');
    if (existing) existing.remove();

    var overlay = document.createElement('div');
    overlay.id = 'custom-alert-overlay';
    overlay.className = 'custom-alert-overlay';
    overlay.innerHTML = '<div class="custom-alert-box">' +
        '<div class="custom-alert-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">' +
        '<path d="M12 2L2 7L12 12L22 7L12 2Z"/><path d="M2 17L12 22L22 17"/><path d="M2 12L12 17L22 12"/>' +
        '</svg></div>' +
        '<div class="custom-alert-title">Informasi</div>' +
        '<div class="custom-alert-message">' + (message || '').replace(/</g, '&lt;').replace(/>/g, '&gt;') + '</div>' +
        '<button class="custom-alert-button" onclick="this.closest(\'.custom-alert-overlay\').remove()">Mengerti</button>' +
        '</div>';

    document.body.appendChild(overlay);

    overlay.addEventListener('click', function (e) {
        if (e.target === overlay) overlay.remove();
    });

    var handleEscape = function (e) {
        if (e.key === 'Escape') {
            overlay.remove();
            document.removeEventListener('keydown', handleEscape);
        }
    };
    document.addEventListener('keydown', handleEscape);
}

/** Konfirmasi interaktif (staff/admin): message + tombol Batal & Lanjutkan. onConfirm = function() dipanggil saat Lanjutkan. */
function showCustomConfirm(message, title, onConfirm) {
    if (typeof title !== 'string') { onConfirm = title; title = 'Konfirmasi'; }
    var existing = document.getElementById('custom-confirm-overlay');
    if (existing) existing.remove();

    var overlay = document.createElement('div');
    overlay.id = 'custom-confirm-overlay';
    overlay.className = 'custom-alert-overlay';
    var box = document.createElement('div');
    box.className = 'custom-alert-box';
    box.innerHTML =
        '<div class="custom-alert-icon custom-confirm-icon">' +
        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>' +
        '</div>' +
        '<div class="custom-alert-title">' + (title || 'Konfirmasi').replace(/</g, '&lt;').replace(/>/g, '&gt;') + '</div>' +
        '<div class="custom-alert-message">' + (message || '').replace(/</g, '&lt;').replace(/>/g, '&gt;') + '</div>' +
        '<div class="custom-confirm-actions">' +
        '<button type="button" class="custom-alert-button custom-confirm-cancel">Batal</button>' +
        '<button type="button" class="custom-alert-button custom-confirm-ok">Lanjutkan</button>' +
        '</div>';
    overlay.appendChild(box);

    function close() {
        overlay.remove();
        document.removeEventListener('keydown', handleEscape);
    }

    box.querySelector('.custom-confirm-cancel').addEventListener('click', close);
    box.querySelector('.custom-confirm-ok').addEventListener('click', function () {
        close();
        if (typeof onConfirm === 'function') onConfirm();
    });

    overlay.addEventListener('click', function (e) {
        if (e.target === overlay) close();
    });

    var handleEscape = function (e) {
        if (e.key === 'Escape') { close(); }
    };
    document.addEventListener('keydown', handleEscape);
    document.body.appendChild(overlay);
}
