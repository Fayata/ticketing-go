

// Auto-dismiss alerts after specified time
function autoDismissAlerts() {
    const alerts = document.querySelectorAll('.alert');
    alerts.forEach(alert => {
        const autoDismiss = alert.dataset.autoDismiss !== 'false';
        if (autoDismiss) {
            setTimeout(() => {
                alert.style.transition = 'opacity 0.3s ease-out, transform 0.3s ease-out';
                alert.style.opacity = '0';
                alert.style.transform = 'translateX(10px)';
                setTimeout(() => alert.remove(), 300);
            }, 5000);
        }
    });
}

// Format relative time
function getRelativeTime(date) {
    const now = new Date();
    const diffInSeconds = Math.floor((now - date) / 1000);
    
    if (diffInSeconds < 60) {
        return 'Baru saja';
    } else if (diffInSeconds < 3600) {
        const minutes = Math.floor(diffInSeconds / 60);
        return `${minutes} menit lalu`;
    } else if (diffInSeconds < 86400) {
        const hours = Math.floor(diffInSeconds / 3600);
        return `${hours} jam lalu`;
    } else if (diffInSeconds < 604800) {
        const days = Math.floor(diffInSeconds / 86400);
        return `${days} hari lalu`;
    } else {
        return date.toLocaleDateString('id-ID', { 
            year: 'numeric', 
            month: 'short', 
            day: 'numeric' 
        });
    }
}

// Show toast notification
function showToast(message, type = 'info') {
    const toast = document.createElement('div');
    toast.className = `alert alert-${type}`;
    toast.style.cssText = `
        position: fixed;
        bottom: 2rem;
        right: 2rem;
        max-width: 400px;
        z-index: 9999;
        animation: slideInRight 0.3s ease-out;
    `;
    
    const iconMap = {
        success: '<svg class="alert-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.01"></polyline></svg>',
        error: '<svg class="alert-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>',
        warning: '<svg class="alert-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>',
        info: '<svg class="alert-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>'
    };
    
    toast.innerHTML = `
        ${iconMap[type] || iconMap.info}
        <span>${message}</span>
    `;
    
    document.body.appendChild(toast);
    
    setTimeout(() => {
        toast.style.animation = 'slideOutRight 0.3s ease-out';
        setTimeout(() => toast.remove(), 300);
    }, 3000);
}

// Handle form loading state
function setFormLoading(form, loading = true) {
    const submitBtn = form.querySelector('button[type="submit"]');
    if (!submitBtn) return;
    
    if (loading) {
        submitBtn.disabled = true;
        submitBtn.classList.add('loading');
        const originalText = submitBtn.innerHTML;
        submitBtn.dataset.originalText = originalText;
        submitBtn.innerHTML = '<span class="spinner"></span> Loading...';
    } else {
        submitBtn.disabled = false;
        submitBtn.classList.remove('loading');
        if (submitBtn.dataset.originalText) {
            submitBtn.innerHTML = submitBtn.dataset.originalText;
        }
    }
}

// Prevent form resubmission on page refresh
function preventFormResubmission() {
    if (window.history.replaceState) {
        window.history.replaceState(null, null, window.location.href);
    }
}

// Smooth scroll to element
function smoothScrollTo(element, offset = 0) {
    if (typeof element === 'string') {
        element = document.querySelector(element);
    }
    if (!element) return;
    
    const elementPosition = element.getBoundingClientRect().top;
    const offsetPosition = elementPosition + window.pageYOffset - offset;
    
    window.scrollTo({
        top: offsetPosition,
        behavior: 'smooth'
    });
}

// Debounce function for search inputs
function debounce(func, wait = 300) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

// Initialize common functionality
document.addEventListener('DOMContentLoaded', function() {
    // Auto-dismiss alerts
    autoDismissAlerts();
    
    // Prevent form resubmission
    preventFormResubmission();
    
    // Format relative times
    const timeElements = document.querySelectorAll('[data-timestamp]');
    timeElements.forEach(el => {
        const timestamp = el.dataset.timestamp;
        if (timestamp) {
            el.textContent = getRelativeTime(new Date(timestamp));
        }
    });
    
    // Smooth scroll for anchor links
    document.querySelectorAll('a[href^="#"]').forEach(anchor => {
        anchor.addEventListener('click', function(e) {
            const href = this.getAttribute('href');
            if (href === '#') {
                e.preventDefault();
                return;
            }
            
            e.preventDefault();
            const target = document.querySelector(href);
            if (target) {
                smoothScrollTo(target, 80);
            }
        });
    });
});

// Export functions for use in other scripts
window.commonUtils = {
    showToast,
    getRelativeTime,
    setFormLoading,
    smoothScrollTo,
    debounce
};