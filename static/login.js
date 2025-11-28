// Login Page JavaScript

document.addEventListener('DOMContentLoaded', function() {
    // Password Toggle Functionality
    const passwordInput = document.getElementById('id_password');
    const togglePassword = document.getElementById('togglePassword');
    const eyeOpen = togglePassword.querySelector('.eye-open');
    const eyeClosed = togglePassword.querySelector('.eye-closed');

    if (togglePassword && passwordInput) {
        togglePassword.addEventListener('click', function(e) {
            e.preventDefault();
            const type = passwordInput.getAttribute('type') === 'password' ? 'text' : 'password';
            passwordInput.setAttribute('type', type);

            if (type === 'text') {
                eyeOpen.style.display = 'none';
                eyeClosed.style.display = 'block';
                togglePassword.setAttribute('aria-label', 'Sembunyikan password');
            } else {
                eyeOpen.style.display = 'block';
                eyeClosed.style.display = 'none';
                togglePassword.setAttribute('aria-label', 'Tampilkan password');
            }
        });
    }

    // Form Validation & Loading State
    const loginForm = document.getElementById('loginForm');
    const loginButton = document.getElementById('loginButton');
    const buttonText = loginButton.querySelector('.button-text');
    const buttonLoader = loginButton.querySelector('.button-loader');

    if (loginForm) {
        loginForm.addEventListener('submit', function(e) {
            e.preventDefault(); 

            const username = document.getElementById('id_username').value.trim();
            const password = document.getElementById('id_password').value;

            if (!username || !password) {
                showError('Harap isi semua field yang diperlukan.');
                return false; 
            }

            loginButton.disabled = true;
            loginButton.classList.add('loading');
            buttonText.style.opacity = '0';
            buttonLoader.style.display = 'block';

            loginForm.submit();
        });
    }

    // Forgot Password Link (placeholder functionality)
    const forgotPasswordLink = document.getElementById('forgotPassword');
    if (forgotPasswordLink) {
        forgotPasswordLink.addEventListener('click', function(e) {
            e.preventDefault();
            // You can implement forgot password functionality here
            alert('Fitur lupa password belum tersedia. Silakan hubungi administrator untuk reset password.');
        });
    }

    // Auto-focus on username field if empty
    const usernameInput = document.getElementById('id_username');
    if (usernameInput && !usernameInput.value) {
        usernameInput.focus();
    }

    // Enter key to submit form
    const inputs = document.querySelectorAll('.form-input');
    inputs.forEach(input => {
        input.addEventListener('keypress', function(e) {
            if (e.key === 'Enter' && loginForm) {
                loginForm.dispatchEvent(new Event('submit'));
            }
        });
    });

    // Input field animations
    inputs.forEach(input => {
        // Add focus animation
        input.addEventListener('focus', function() {
            this.parentElement.classList.add('focused');
        });

        input.addEventListener('blur', function() {
            if (!this.value) {
                this.parentElement.classList.remove('focused');
            }
        });

        // Check if input has value on load
        if (input.value) {
            input.parentElement.classList.add('focused');
        }
    });

    // Remember me checkbox enhancement
    const rememberMe = document.getElementById('remember_me');
    if (rememberMe) {
        // Check if remember me was previously checked
        const savedRememberMe = localStorage.getItem('remember_me');
        if (savedRememberMe === 'true') {
            rememberMe.checked = true;
        }

        rememberMe.addEventListener('change', function() {
            localStorage.setItem('remember_me', this.checked);
        });
    }

    // Auto-dismiss alerts after 5 seconds
    const alerts = document.querySelectorAll('.alert');
    alerts.forEach(alert => {
        setTimeout(() => {
            alert.style.transition = 'opacity 0.3s ease-out, transform 0.3s ease-out';
            alert.style.opacity = '0';
            alert.style.transform = 'translateX(-10px)';
            setTimeout(() => {
                alert.remove();
            }, 300);
        }, 5000);
    });

    // Smooth scroll to error if present
    const errorAlert = document.querySelector('.alert-error');
    if (errorAlert) {
        errorAlert.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }

    // Password strength indicator (optional enhancement)
    if (passwordInput) {
        passwordInput.addEventListener('input', function() {
            const password = this.value;
            // You can add password strength indicator here if needed
        });
    }

    // Form input validation feedback
    inputs.forEach(input => {
        input.addEventListener('invalid', function(e) {
            e.preventDefault();
            this.classList.add('error');
            showFieldError(this, 'Field ini wajib diisi.');
        });

        input.addEventListener('input', function() {
            this.classList.remove('error');
            const errorMsg = this.parentElement.querySelector('.field-error');
            if (errorMsg) {
                errorMsg.remove();
            }
        });
    });
});

// Helper function to show error message
function showError(message) {
    const alertContainer = document.querySelector('.alert-container') || createAlertContainer();
    const alert = document.createElement('div');
    alert.className = 'alert alert-error';
    alert.innerHTML = `
        <svg class="alert-icon" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="2"/>
            <path d="M12 8V12" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
            <path d="M12 16H12.01" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
        </svg>
        <span>${message}</span>
    `;
    
    alertContainer.insertBefore(alert, alertContainer.firstChild);
    
    // Auto-remove after 5 seconds
    setTimeout(() => {
        alert.style.transition = 'opacity 0.3s ease-out, transform 0.3s ease-out';
        alert.style.opacity = '0';
        alert.style.transform = 'translateX(-10px)';
        setTimeout(() => alert.remove(), 300);
    }, 5000);
}

// Helper function to create alert container if it doesn't exist
function createAlertContainer() {
    const container = document.createElement('div');
    container.className = 'alert-container';
    const form = document.getElementById('loginForm');
    if (form) {
        form.parentElement.insertBefore(container, form);
    }
    return container;
}

// Helper function to show field-specific error
function showFieldError(input, message) {
    const errorMsg = document.createElement('span');
    errorMsg.className = 'field-error';
    errorMsg.style.color = 'var(--error-color)';
    errorMsg.style.fontSize = '0.75rem';
    errorMsg.style.marginTop = '0.25rem';
    errorMsg.textContent = message;
    input.parentElement.appendChild(errorMsg);
}

// Prevent form resubmission on page refresh
if (window.history.replaceState) {
    window.history.replaceState(null, null, window.location.href);
}

