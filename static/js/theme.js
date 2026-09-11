/**
 * Theme (dark/light) handler.
 * Dipakai di halaman setelah login (staff/admin) agar tombol toggle tersedia di dalam aplikasi.
 */
(function () {
  var STORAGE_KEY = 'theme';

  function getTheme() {
    try {
      var stored = localStorage.getItem(STORAGE_KEY);
      if (stored === 'light' || stored === 'dark') return stored;
    } catch (e) {}
    var current = document.documentElement.getAttribute('data-theme');
    if (current === 'light' || current === 'dark') return current;
    return 'light';
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

  // Auto-reload on browser back/forward navigation (bfcache) to prevent stale state and unread badges
  window.addEventListener('pageshow', function (event) {
    if (event.persisted) {
      window.location.reload();
      return;
    }
    if (window.performance) {
      var navEntries = window.performance.getEntriesByType && window.performance.getEntriesByType('navigation');
      if (navEntries && navEntries.length > 0 && navEntries[0].type === 'back_forward') {
        window.location.reload();
        return;
      }
      if (window.performance.navigation && window.performance.navigation.type === 2) {
        window.location.reload();
        return;
      }
    }
  });

  // Heartbeat to maintain online / delivered status while browser tab is open
  function sendHeartbeat() {
    if (document.visibilityState === 'visible') {
      var base = document.querySelector('base');
      var prefix = (base && base.getAttribute('href')) ? base.getAttribute('href').replace(/\/$/, '') : '';
      if (!prefix && window.location.pathname.indexOf('/Ticketing') !== -1) {
        prefix = '/Ticketing';
      }
      fetch(prefix + '/api/heartbeat', { method: 'POST', credentials: 'same-origin' }).catch(function(){});
    }
  }
  setTimeout(sendHeartbeat, 3000);
  setInterval(sendHeartbeat, 45000);
})();


