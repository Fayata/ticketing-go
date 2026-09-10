// static/js/admin_dashboard_live.js
// Real-time auto-update for Admin Dashboard with smart diffing & tab-visibility throttling.

(function() {
    'use strict';

    const POLL_INTERVAL = 4000; // 4 seconds
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

    function renderWaitingList(waiting) {
        const container = document.getElementById('adminWaitingList');
        if (!container) return;

        if (!waiting || waiting.length === 0) {
            container.innerHTML = '<p class="admin-empty-msg">Tidak ada tiket menunggu.</p>';
            return;
        }

        let html = '';
        waiting.forEach(item => {
            html += `
                <div class="admin-waiting-item">
                    <div class="admin-waiting-bar" style="--accent: #ef4444;"></div>
                    <div class="admin-waiting-content">
                        <div class="admin-waiting-meta">
                            <span class="admin-waiting-id">${escapeHtml(item.ticket_number)}</span>
                            <span class="admin-waiting-days">${escapeHtml(item.waiting_duration)}</span>
                        </div>
                        <div class="admin-waiting-title">${escapeHtml(item.title)}</div>
                        <div class="admin-waiting-dept">${escapeHtml(item.department || '—')}</div>
                    </div>
                </div>`;
        });
        container.innerHTML = html;
    }

    function renderRecentList(recent) {
        const tbody = document.getElementById('adminRecentList');
        if (!tbody) return;

        if (!recent || recent.length === 0) {
            tbody.innerHTML = '<tr><td colspan="6" class="admin-td-empty">Belum ada tiket.</td></tr>';
            return;
        }

        let html = '';
        recent.forEach(item => {
            html += `
                <tr>
                    <td class="admin-td-id">${escapeHtml(item.ticket_number)}</td>
                    <td class="admin-td-title">${escapeHtml(item.title)}</td>
                    <td><span class="admin-priority admin-priority-${escapeHtml(item.priority)}">${escapeHtml(item.priority)}</span></td>
                    <td><span class="admin-badge admin-badge-${escapeHtml(item.status)}">${escapeHtml(item.status)}</span></td>
                    <td>${escapeHtml(item.department || '—')}</td>
                    <td class="admin-td-date">${escapeHtml(item.created_at)}</td>
                </tr>`;
        });
        tbody.innerHTML = html;
    }

    function applyData(data) {
        if (!data) return;

        // Smart diff check
        const newSig = JSON.stringify({
            kpi: data.kpi,
            waiting: (data.waiting_tickets || []).map(w => ({ id: w.id, dur: w.waiting_duration })),
            recent: (data.recent_tickets || []).map(r => ({ id: r.id, st: r.status, pri: r.priority }))
        });

        if (newSig === lastDataSignature) {
            return;
        }
        lastDataSignature = newSig;

        // Update KPIs
        if (data.kpi) {
            updateTextContent('adminKpiWaiting', data.kpi.WaitingCount ?? 0);
            updateTextContent('adminKpiProgress', data.kpi.InProgressCount ?? 0);
            updateTextContent('adminKpiClosedToday', data.kpi.ClosedTodayCount ?? 0);
            if (data.kpi.AvgRating !== undefined) {
                const rating = Number(data.kpi.AvgRating);
                updateTextContent('adminKpiAvgRating', rating.toFixed(1) + ' ★');
            }
            if (data.kpi.RatedCount !== undefined) {
                updateTextContent('adminKpiRatedCount', data.kpi.RatedCount + ' tiket dinilai');
            }
            updateTextContent('adminKpiTotalMonth', data.kpi.TotalTicketsMonth ?? 0);
            updateTextContent('adminKpiTotalUsers', data.kpi.TotalUsersActive ?? 0);
            updateTextContent('adminKpiStaffActive', data.kpi.StaffActiveCount ?? 0);
            updateTextContent('adminKpiUnrated', data.kpi.UnratedCount ?? 0);
        }

        // Update Lists
        renderWaitingList(data.waiting_tickets || []);
        renderRecentList(data.recent_tickets || []);
    }

    async function fetchLiveDashboard() {
        if (isFetching || document.hidden) return;
        isFetching = true;

        try {
            const resp = await fetch('admin/api/live', {
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

    function init() {
        // Only run if admin dashboard elements exist
        if (!document.getElementById('adminKpiWaiting')) return;

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
