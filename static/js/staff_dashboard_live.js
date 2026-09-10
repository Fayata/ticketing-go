// static/js/staff_dashboard_live.js
// Real-time auto-update for Staff Department Dashboard with smart DOM diffing & tab-visibility throttling.

(function() {
    'use strict';

    const POLL_INTERVAL = 3000; // 3 seconds
    let pollTimer = null;
    let isFetching = false;
    let lastDataSignature = '';

    function escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#039;');
    }

    function updateTextContent(elementId, text) {
        const el = document.getElementById(elementId);
        if (el && el.textContent !== String(text)) {
            el.textContent = text;
        }
    }

    function renderPool(pool) {
        const container = document.getElementById('staffPoolList');
        if (!container) return;

        if (!pool || pool.length === 0) {
            container.innerHTML = `
                <div class="staff-empty-state">
                    <svg class="staff-empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M5 13l4 4L19 7"/></svg>
                    <p>Tidak ada tiket baru di pool departemen ini.</p>
                </div>`;
            return;
        }

        let html = '';
        pool.forEach(item => {
            const slaHtml = (item.sla_badge_class && item.sla_badge_class !== 'sla-badge-gray')
                ? `<span class="sla-badge ${escapeHtml(item.sla_badge_class)}" title="${escapeHtml(item.sla_badge_detail || '')}">${escapeHtml(item.sla_badge_label)}</span>`
                : '';

            html += `
                <div class="staff-ticket-item staff-pool-item" onclick="window.location.href='departement/tiket/${item.id}';">
                    <div class="staff-ticket-bar staff-priority-${escapeHtml(item.priority)}"></div>
                    <div class="staff-ticket-inner">
                        <div class="staff-ticket-meta">
                            <span class="staff-ticket-id">${escapeHtml(item.ticket_number)}</span>
                            <span class="staff-badge staff-badge-${escapeHtml(item.priority)}">${escapeHtml(item.priority_display || item.priority)}</span>
                            ${slaHtml}
                            <span class="staff-ticket-dept-inline">${escapeHtml(item.department_name || '—')}</span>
                        </div>
                        <div class="staff-ticket-title">${escapeHtml(item.title)}</div>
                        <div class="staff-ticket-meta-footer">
                            <span>${escapeHtml(item.creator_name || '')}</span>
                            <span class="staff-ticket-time">${escapeHtml(item.created_at_ago)}</span>
                        </div>
                        <a href="departement/tiket/claim/${item.id}" class="staff-btn staff-btn-claim" onclick="event.stopPropagation();">Ambil Tiket</a>
                    </div>
                </div>`;
        });
        container.innerHTML = html;
    }

    function renderBreached(breached) {
        const container = document.getElementById('staffBreachedList');
        if (!container) return;

        if (!breached || breached.length === 0) {
            container.innerHTML = `
                <div class="staff-empty-state staff-empty-center">
                    <svg class="staff-empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M5 13l4 4L19 7"/></svg>
                    <p>Semua tiket direspon tepat waktu</p>
                </div>`;
            return;
        }

        let html = '';
        const limit = Math.min(breached.length, 5);
        for (let i = 0; i < limit; i++) {
            const item = breached[i];
            html += `
                <div class="staff-mini-item staff-ticket-breached" onclick="window.location.href='departement/tiket/${item.id}';">
                    <div class="staff-mini-icon staff-priority-${escapeHtml(item.priority)}">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
                    </div>
                    <div class="staff-mini-body">
                        <div class="staff-mini-title">${escapeHtml(item.title)}</div>
                        <div class="staff-mini-meta">${escapeHtml(item.ticket_number)} · <span class="staff-ticket-overdue">Terlambat: ${escapeHtml(item.overdue_detail)}</span></div>
                    </div>
                    <span class="staff-badge staff-badge-${escapeHtml(item.priority)}">${escapeHtml(item.priority_display || item.priority)}</span>
                </div>`;
        }
        container.innerHTML = html;
    }

    function renderMyActive(myActive) {
        const container = document.getElementById('staffMyActiveList');
        if (!container) return;

        if (!myActive || myActive.length === 0) {
            container.innerHTML = `
                <div class="staff-empty-state">
                    <p>Anda tidak sedang mengerjakan tiket.</p>
                    <p class="staff-empty-hint">Ambil tiket dari pool di sebelah.</p>
                </div>`;
            return;
        }

        let html = '';
        myActive.forEach(item => {
            html += `
                <div class="staff-ticket-item" onclick="window.location.href='departement/tiket/${item.id}';">
                    <div class="staff-ticket-bar staff-priority-${escapeHtml(item.priority)}"></div>
                    <div class="staff-ticket-inner">
                        <div class="staff-ticket-meta">
                            <span class="staff-ticket-id">${escapeHtml(item.ticket_number)}</span>
                            <span class="staff-badge staff-badge-${escapeHtml(item.priority)}">${escapeHtml(item.priority_display || item.priority)}</span>
                        </div>
                        <div class="staff-ticket-title">${escapeHtml(item.title)}</div>
                        <div class="staff-ticket-dept">${escapeHtml(item.department_name || '—')}</div>
                        <div class="staff-ticket-actions">
                            <span class="staff-ticket-date">${escapeHtml(item.updated_at_ago)}</span>
                            <a href="departement/tiket/release/${item.id}" class="staff-btn staff-btn-release" onclick="event.stopPropagation(); event.preventDefault(); if (typeof showCustomConfirm === 'function') { showCustomConfirm('Kembalikan tiket ini ke pool? Tiket akan bisa diambil lagi oleh staff lain.', 'Lepas ke Pool', function(){ window.location.href='departement/tiket/release/${item.id}'; }); } else if (confirm('Kembalikan tiket ini ke pool?')) { window.location.href='departement/tiket/release/${item.id}'; } return false;">Lepas ke Pool</a>
                        </div>
                    </div>
                </div>`;
        });
        container.innerHTML = html;
    }

    function updateBadges(poolCount, breachedCount, myActiveCount) {
        const poolBadge = document.getElementById('staffPoolBadge');
        if (poolBadge) {
            poolBadge.textContent = poolCount;
        }

        const myActiveBadge = document.getElementById('staffMyActiveBadge');
        if (myActiveBadge) {
            myActiveBadge.textContent = myActiveCount;
        }

        const breachedBadge = document.getElementById('staffBreachedBadge');
        if (breachedBadge) {
            breachedBadge.textContent = breachedCount;
            breachedBadge.style.display = breachedCount > 0 ? '' : 'none';
        }
    }

    function applyData(data) {
        if (!data) return;

        // Smart Diff: Compare hash/signature
        const newSig = JSON.stringify({
            kpi: data.kpi,
            pool: (data.pool || []).map(p => ({ id: p.id, pri: p.priority, sla: p.sla_badge_label })),
            breached: (data.breached || []).map(b => ({ id: b.id, ov: b.overdue_detail })),
            my_active: (data.my_active || []).map(m => ({ id: m.id, st: m.status, up: m.updated_at_ago }))
        });

        if (newSig === lastDataSignature) {
            return; // No data changes, don't touch DOM
        }
        lastDataSignature = newSig;

        // 1. Update KPIs
        if (data.kpi) {
            updateTextContent('staffKpiWaiting', data.kpi.WaitingCount ?? 0);
            updateTextContent('staffKpiProgress', data.kpi.ProgressCount ?? 0);
            updateTextContent('staffKpiClosedToday', data.kpi.ClosedTodayCount ?? 0);
            updateTextContent('staffKpiClosedMonth', data.kpi.ClosedMonthCount ?? 0);
            if (data.kpi.AvgRating !== undefined) {
                const rating = Number(data.kpi.AvgRating);
                updateTextContent('staffKpiAvgRating', rating.toFixed(1) + ' ★');
            }
            if (data.kpi.RatedCount !== undefined) {
                updateTextContent('staffKpiRatedCount', data.kpi.RatedCount + ' tiket dinilai');
            }
        }

        // 2. Update Badges
        const poolLen = (data.pool || []).length;
        const breachedLen = (data.breached || []).length;
        const myActiveLen = (data.my_active || []).length;
        updateBadges(poolLen, breachedLen, myActiveLen);

        // 3. Update Lists
        renderPool(data.pool || []);
        renderBreached(data.breached || []);
        renderMyActive(data.my_active || []);
    }

    async function fetchLiveDashboard() {
        if (isFetching || document.hidden) return;
        isFetching = true;

        try {
            const resp = await fetch('departement/api/live', {
                headers: { 'Accept': 'application/json' },
                cache: 'no-store'
            });

            if (resp.ok) {
                const data = await resp.json();
                applyData(data);
            }
        } catch (err) {
            // Silently suppress network error during live poll
        } finally {
            isFetching = false;
        }
    }

    function startPolling() {
        if (pollTimer) clearInterval(pollTimer);
        pollTimer = setInterval(fetchLiveDashboard, POLL_INTERVAL);
    }

    function stopPolling() {
        if (pollTimer) {
            clearInterval(pollTimer);
            pollTimer = null;
        }
    }

    // Init when DOM ready
    function init() {
        // Only run if staff pool list is present on the page
        if (!document.getElementById('staffPoolList')) return;

        // Visibility Change Handler (pause when tab hidden to save CPU/battery)
        document.addEventListener('visibilitychange', function() {
            if (document.hidden) {
                stopPolling();
            } else {
                fetchLiveDashboard();
                startPolling();
            }
        });

        startPolling();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
