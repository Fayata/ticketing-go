// tickets/static/js/dashboard.js

document.addEventListener('DOMContentLoaded', function() {
    // Sidebar Toggle
    initSidebarToggle();
    
    // Notification Button
    initNotificationButton();
    
    // Ticket Item Click Handlers
    initTicketClickHandlers();
    
    // Scroll Animations
    initScrollAnimations();
    
    // Handle Window Resize
    handleWindowResize();

    // User Dashboard Live Sync
    initUserDashboardLiveSync();
});

// Sidebar Toggle Functionality
function initSidebarToggle() {
    const sidebarToggle = document.getElementById('sidebarToggle');
    const sidebar = document.getElementById('sidebar');
    const mobileMenuToggle = document.getElementById('mobileMenuToggle');

    if (sidebarToggle && sidebar) {
        // Load saved sidebar state
        const sidebarState = localStorage.getItem('sidebarCollapsed');
        if (sidebarState === 'true') {
            sidebar.classList.add('collapsed');
        }

        // Toggle sidebar
        sidebarToggle.addEventListener('click', function() {
            sidebar.classList.toggle('collapsed');
            localStorage.setItem('sidebarCollapsed', sidebar.classList.contains('collapsed'));
        });
    }

    // Mobile Menu Toggle
    if (mobileMenuToggle && sidebar) {
        let overlay = null;
        function closeMobileSidebar() {
            sidebar.classList.remove('mobile-open');
            if (overlay && overlay.parentNode) overlay.parentNode.removeChild(overlay);
            overlay = null;
        }
        mobileMenuToggle.addEventListener('click', function() {
            if (window.innerWidth > 768) return;
            sidebar.classList.toggle('mobile-open');
            if (sidebar.classList.contains('mobile-open')) {
                overlay = document.createElement('div');
                overlay.setAttribute('aria-hidden', 'true');
                overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.4);z-index:199;';
                overlay.addEventListener('click', closeMobileSidebar);
                document.body.appendChild(overlay);
            } else {
                closeMobileSidebar();
            }
        });
        document.addEventListener('click', function(e) {
            if (window.innerWidth <= 768 && !sidebar.classList.contains('mobile-open')) return;
            if (window.innerWidth <= 768 && !sidebar.contains(e.target) && !mobileMenuToggle.contains(e.target) && e.target !== overlay) {
                closeMobileSidebar();
            }
        });
    }
}

// Notification Button
function initNotificationButton() {
    const notificationBtn = document.getElementById('notificationBtn');
    if (notificationBtn) {
        // Notification popover sudah di-handle di template (`base.html`).
        // Jangan timpa click handler dengan toast lama.
        const notifPanel = document.getElementById('notifPanel');
        if (notifPanel) {
            return;
        }

        notificationBtn.addEventListener('click', function() {
            // TODO: Implement notification panel
            window.commonUtils.showToast('Fitur notifikasi akan segera hadir!', 'info');
        });
    }
}

// Ticket Item Click Handlers
function initTicketClickHandlers() {
    const ticketItems = document.querySelectorAll('.ticket-item');
    ticketItems.forEach(item => {
        // Add cursor pointer
        item.style.cursor = 'pointer';
        
        // Add transition
        item.style.transition = 'all var(--transition-fast)';
        
        // Hover effect
        item.addEventListener('mouseenter', function() {
            this.style.backgroundColor = 'var(--bg-hover)';
        });
        
        item.addEventListener('mouseleave', function() {
            this.style.backgroundColor = 'transparent';
        });
        
        // Click handler
        item.addEventListener('click', function(e) {
            // Don't navigate if clicking on a link or button inside
            if (e.target.tagName === 'A' || e.target.tagName === 'BUTTON') {
                return;
            }
            
            // Get ticket ID from onclick attribute or data attribute
            const onclickAttr = this.getAttribute('onclick');
            if (onclickAttr) {
                eval(onclickAttr);
            }
        });
    });
}

// Scroll Animations
function initScrollAnimations() {
    const observerOptions = {
        threshold: 0.1,
        rootMargin: '0px 0px -50px 0px'
    };

    const observer = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                entry.target.classList.add('fade-in-up');
                observer.unobserve(entry.target);
            }
        });
    }, observerOptions);

    // Observe all cards and stats
    document.querySelectorAll('.stat-card, .card').forEach(el => {
        observer.observe(el);
    });
}

// Handle Window Resize
function handleWindowResize() {
    let resizeTimer;
    const sidebar = document.getElementById('sidebar');
    
    window.addEventListener('resize', function() {
        clearTimeout(resizeTimer);
        resizeTimer = setTimeout(function() {
            if (window.innerWidth > 768 && sidebar) {
                sidebar.classList.remove('mobile-open');
            }
        }, 250);
    });
}

// Stats Animation (Number Count Up)
function animateValue(element, start, end, duration) {
    let startTimestamp = null;
    const step = (timestamp) => {
        if (!startTimestamp) startTimestamp = timestamp;
        const progress = Math.min((timestamp - startTimestamp) / duration, 1);
        const value = Math.floor(progress * (end - start) + start);
        element.textContent = value;
        if (progress < 1) {
            window.requestAnimationFrame(step);
        }
    };
    window.requestAnimationFrame(step);
}

// Initialize stat animations when visible
const statObserver = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
        if (entry.isIntersecting && !entry.target.dataset.animated) {
            const statNumber = entry.target.querySelector('.stat-info h3');
            if (statNumber && !isNaN(parseInt(statNumber.textContent))) {
                const endValue = parseInt(statNumber.textContent);
                statNumber.textContent = '0';
                animateValue(statNumber, 0, endValue, 1000);
                entry.target.dataset.animated = 'true';
            }
        }
    });
}, { threshold: 0.5 });

document.querySelectorAll('.stat-card').forEach(card => {
    statObserver.observe(card);
});

// Export functions
window.dashboardUtils = {
    animateValue
};

// User Dashboard Live Sync
function initUserDashboardLiveSync() {
    const ticketListEl = document.getElementById('userTicketList');
    if (!ticketListEl) return;

    const POLL_INTERVAL = 4000;
    let pollTimer = null;
    let isFetching = false;
    let lastDataSignature = '';

    function escapeHtml(str) {
        if (!str) return '';
        return String(str)
            .replace(/&/g, '&amp;')
            .replace(/&/g, '&lt;')
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

    function renderRecentTickets(tickets) {
        const container = document.getElementById('userTicketList');
        if (!container) return;

        if (!tickets || tickets.length === 0) {
            container.innerHTML = `
                <div style="text-align: center; padding: 3rem; color: #6b7280;">
                    <img src="static/icons/ticket.png?v=2" alt="Belum ada tiket" width="56" height="56" class="empty-ticket-icon" style="margin: 0 auto 1rem; display: block; opacity: 0.65;">
                    <p style="font-size: 1.125rem; font-weight: 600; margin-bottom: 0.5rem;">Belum ada tiket</p>
                    <p style="font-size: 0.875rem;">Buat tiket pertama Anda untuk mendapatkan bantuan</p>
                </div>`;
            return;
        }

        let html = '';
        tickets.forEach(item => {
            html += `
                <div class="ticket-item" onclick="window.location.href='tiket/${item.id}'">
                    <div class="ticket-header">
                        <div>
                            <div class="ticket-id-status">
                                <span class="ticket-id">${escapeHtml(item.ticket_number)}</span>
                                <span class="status-badge ${escapeHtml(item.status_class)}">
                                    ${escapeHtml(item.status_display)}
                                </span>
                            </div>
                            <h3 class="ticket-title">${escapeHtml(item.title)}</h3>
                            <div class="ticket-meta">
                                <span class="ticket-meta-item">
                                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
                                    </svg>
                                    ${escapeHtml(item.department_name || 'Umum')}
                                </span>
                                <span class="ticket-meta-item priority-${escapeHtml(item.priority_class)}">
                                    ${escapeHtml(item.priority_display)} Priority
                                </span>
                                <span class="ticket-meta-item">
                                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                        <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path>
                                    </svg>
                                    ${item.reply_count} balasan
                                </span>
                            </div>
                        </div>
                    </div>
                    <div class="ticket-footer">
                        <span>Dibuat: ${escapeHtml(item.created_at)}</span>
                        <span>Update terakhir: ${escapeHtml(item.created_at_ago)} lalu</span>
                    </div>
                </div>`;
        });
        container.innerHTML = html;
        initTicketClickHandlers();
    }

    function applyData(data) {
        if (!data) return;

        const newSig = JSON.stringify({
            waiting: data.waiting_tickets,
            progress: data.in_progress_tickets,
            closed: data.closed_tickets,
            total: data.total_tickets,
            tickets: (data.recent_tickets || []).map(t => ({ id: t.id, st: t.status, rc: t.reply_count, ago: t.created_at_ago }))
        });

        if (newSig === lastDataSignature) {
            return;
        }
        lastDataSignature = newSig;

        // Update KPIs
        updateTextContent('userKpiWaiting', data.waiting_tickets ?? 0);
        updateTextContent('userKpiProgress', data.in_progress_tickets ?? 0);
        updateTextContent('userKpiClosed', data.closed_tickets ?? 0);
        updateTextContent('userKpiTotal', data.total_tickets ?? 0);

        // Update Tickets
        renderRecentTickets(data.recent_tickets || []);
    }

    async function fetchLiveDashboard() {
        if (isFetching || document.hidden) return;
        isFetching = true;

        try {
            const resp = await fetch('api/dashboard/live', {
                headers: { 'Accept': 'application/json' },
                cache: 'no-store'
            });

            if (resp.ok) {
                const data = await resp.json();
                applyData(data);
            }
        } catch (err) {
            // Silently suppress polling network errors
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