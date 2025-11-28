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
        mobileMenuToggle.addEventListener('click', function() {
            sidebar.classList.toggle('mobile-open');
        });

        // Close sidebar when clicking outside on mobile
        document.addEventListener('click', function(e) {
            if (window.innerWidth <= 768) {
                if (!sidebar.contains(e.target) && !mobileMenuToggle.contains(e.target)) {
                    sidebar.classList.remove('mobile-open');
                }
            }
        });
    }
}

// Notification Button
function initNotificationButton() {
    const notificationBtn = document.getElementById('notificationBtn');
    if (notificationBtn) {
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