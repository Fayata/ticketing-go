/**
 * Theme (dark/light) handler.
 * Dipakai di halaman setelah login (staff/admin) agar tombol toggle tersedia di dalam aplikasi.
 */
(function () {
  var STORAGE_KEY = 'theme';

  function getTheme() {
    var t = 'dark';
    try {
      t = localStorage.getItem(STORAGE_KEY) || 'dark';
    } catch (e) {}
    return t === 'light' ? 'light' : 'dark';
  }

  function setTheme(theme) {
    var t = theme === 'light' ? 'light' : 'dark';
    document.documentElement.setAttribute('data-theme', t);
    try {
      localStorage.setItem(STORAGE_KEY, t);
    } catch (e) {}
    syncToggles(t);
  }

  function syncToggles(theme) {
    var toggles = document.querySelectorAll('[data-theme-toggle], #themeToggle');
    toggles.forEach(function (btn) {
      if (!btn) return;
      btn.textContent = theme === 'light' ? '\u263C' : '\u263E';
      btn.setAttribute(
        'aria-label',
        theme === 'light' ? 'Mode terang (klik untuk gelap)' : 'Mode gelap (klik untuk terang)'
      );
      btn.setAttribute('aria-pressed', theme === 'dark' ? 'true' : 'false');
    });
  }

  function toggleTheme() {
    var next = getTheme() === 'dark' ? 'light' : 'dark';
    setTheme(next);
  }

  // Expose minimal API (kadang dipakai onclick)
  window.Theme = {
    get: getTheme,
    set: setTheme,
    toggle: toggleTheme,
  };

  document.addEventListener('DOMContentLoaded', function () {
    syncToggles(getTheme());
    document.addEventListener('click', function (e) {
      var target = e.target;
      if (!target) return;
      var btn = target.closest ? target.closest('[data-theme-toggle], #themeToggle') : null;
      if (!btn) return;
      e.preventDefault();
      toggleTheme();
    });
  });
})();

