/* global: showCustomAlert — modal alert (dipanggil dari dashboard.js dll) */
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
