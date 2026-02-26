(function() {
  var icons = {
    ticket: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>',
    reply:  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>',
    check:  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>',
    alert:  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
    info:   '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>'
  };
  var currentTab = 'semua';
  var notifications = [];
  var panel = document.getElementById('notifPanel');
  var btn = document.getElementById('notificationBtn');
  var wrap = document.querySelector('.notif-wrap');
  if (!panel || !btn) return;
  var backdrop = document.createElement('div');
  backdrop.className = 'notif-backdrop';
  document.body.appendChild(backdrop);

  function openPanel() {
    var rect = btn.getBoundingClientRect();
    panel.style.setProperty('--notif-portal-top', (rect.bottom + 10) + 'px');
    panel.style.setProperty('--notif-portal-right', (window.innerWidth - rect.right) + 'px');
    document.body.appendChild(panel);
    panel.classList.add('open', 'notif-panel-portal');
    backdrop.classList.add('show');
    btn.setAttribute('aria-expanded', 'true');
    loadNotifications();
  }

  function closePanel() {
    panel.classList.remove('open', 'notif-panel-portal');
    panel.style.removeProperty('--notif-portal-top');
    panel.style.removeProperty('--notif-portal-right');
    if (wrap) wrap.appendChild(panel);
    backdrop.classList.remove('show');
    btn.setAttribute('aria-expanded', 'false');
  }

  function filtered() {
    if (currentTab === 'belum') return notifications.filter(function(n) { return n.unread; });
    if (currentTab === 'tiket') return notifications.filter(function(n) { return ['tiket', 'reply', 'status'].indexOf(n.type) !== -1; });
    return notifications;
  }

  function renderList() {
    var list = document.getElementById('npList');
    var items = filtered();
    if (!items.length) {
      list.innerHTML = '<div class="np-empty"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg><p>Tidak ada notifikasi</p></div>';
      return;
    }
    list.innerHTML = items.map(function(n) {
      return '<div class="np-item ' + (n.unread ? 'unread' : '') + '" data-id="' + n.id + '" data-ticket-id="' + (n.ticket_id || '') + '">' +
        '<div class="np-icon ' + n.color + '">' + (icons[n.icon] || icons.info) + '</div>' +
        '<div class="np-body"><div class="np-notif-title">' + (n.title || '') + '</div><div class="np-notif-desc">' + (n.desc || '') + '</div><div class="np-notif-time">' + (n.time || '') + '</div></div>' +
        (n.unread ? '<div class="unread-dot"></div>' : '') + '</div>';
    }).join('');

    list.querySelectorAll('.np-item').forEach(function(el) {
      el.addEventListener('click', function() {
        var rawId = el.dataset.id;
        if (!rawId) return;
        var notif = notifications.find(function(n) { return n.id === +rawId || n.id === rawId; });
        if (notif && notif.unread) {
          markAsRead(notif.id).then(function(ok) {
            if (ok) {
              notif.unread = false;
              updateBadges();
              renderList();
            }
          });
        }
        var ticketId = el.dataset.ticketId;
        if (ticketId) window.location.href = 'tiket/' + ticketId;
      });
    });
  }

  function loadNotifications() {
    fetch('api/notifications?filter=' + currentTab, { credentials: 'same-origin' })
      .then(function(r) { return r.json(); })
      .then(function(data) {
        notifications = data.notifications || [];
        updateBadges(data.counts);
        renderList();
      })
      .catch(function(err) {
        console.error('Failed to load notifications:', err);
        document.getElementById('npList').innerHTML = '<div class="np-empty"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg><p>Gagal memuat notifikasi</p></div>';
      });
  }

  function updateBadges(counts) {
    var uc = (counts && counts.unread !== undefined) ? counts.unread : notifications.filter(function(n) { return n.unread; }).length;
    var countEl = document.getElementById('notifCount');
    var badgeEl = document.getElementById('badgeLabel');
    if (countEl) {
      countEl.textContent = uc;
      countEl.style.display = uc > 0 ? 'flex' : 'none';
    }
    if (badgeEl) {
      badgeEl.textContent = uc > 0 ? uc + ' baru' : 'semua dibaca';
      badgeEl.style.background = uc > 0 ? 'var(--notif-red)' : 'var(--notif-green)';
    }
    var tcBelum = document.getElementById('tcBelum');
    var tcSemua = document.getElementById('tcSemua');
    var tcTiket = document.getElementById('tcTiket');
    if (tcBelum) tcBelum.textContent = (counts && counts.unread !== undefined) ? counts.unread : uc;
    if (tcSemua) tcSemua.textContent = (counts && counts.all !== undefined) ? counts.all : notifications.length;
    if (tcTiket) tcTiket.textContent = (counts && counts.ticket !== undefined) ? counts.ticket : notifications.filter(function(n) { return ['tiket', 'reply', 'status'].indexOf(n.type) !== -1; }).length;
    btn.classList.toggle('active', uc > 0);
  }

  function markAsRead(id) {
    return fetch('api/notifications/read', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ id: String(id) }),
      credentials: 'same-origin'
    }).then(function(res) { return res.ok; }).catch(function(err) { console.error('Failed to mark as read:', err); return false; });
  }

  function markAllAsRead() {
    fetch('api/notifications/read-all', { method: 'POST', credentials: 'same-origin' })
      .then(function() {
        notifications.forEach(function(n) { n.unread = false; });
        updateBadges({ unread: 0, all: notifications.length, ticket: notifications.filter(function(n) { return ['tiket', 'reply', 'status'].indexOf(n.type) !== -1; }).length });
        renderList();
      })
      .catch(function(err) { console.error('Failed to mark all as read:', err); });
  }

  btn.addEventListener('click', function(e) {
    e.stopPropagation();
    if (panel.classList.contains('open')) closePanel(); else openPanel();
  });
  var closeBtn = document.getElementById('closePanel');
  if (closeBtn) closeBtn.addEventListener('click', closePanel);
  backdrop.addEventListener('click', closePanel);
  document.querySelectorAll('.np-tab').forEach(function(tab) {
    tab.addEventListener('click', function() {
      document.querySelectorAll('.np-tab').forEach(function(t) { t.classList.remove('active'); });
      tab.classList.add('active');
      currentTab = tab.dataset.tab;
      loadNotifications();
    });
  });
  var readAllBtn = document.getElementById('readAllBtn');
  if (readAllBtn) readAllBtn.addEventListener('click', markAllAsRead);
  document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') closePanel();
  });
  fetch('api/notifications/count', { credentials: 'same-origin' }).then(function(r) { return r.json(); }).then(function(data) {
    var uc = data.count || 0;
    var countEl = document.getElementById('notifCount');
    if (countEl && uc > 0) {
      countEl.textContent = uc;
      countEl.style.display = 'flex';
      btn.classList.add('active');
    }
  }).catch(function() {});
})();
