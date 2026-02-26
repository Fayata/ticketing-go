/* Sidebar collapse + mobile menu — halaman departemen */
(function(){
    var sb = document.getElementById('deptSidebar');
    var btn = document.getElementById('deptSidebarToggle');
    var mobileBtn = document.getElementById('deptMobileMenuToggle');
    if (sb && btn) {
        var stored = localStorage.getItem('deptSidebarCollapsed');
        if (stored === '1') sb.classList.add('collapsed');
        btn.addEventListener('click', function() {
            if (window.innerWidth <= 768) {
                sb.classList.toggle('mobile-open');
                if (sb.classList.contains('mobile-open')) {
                    var ov = document.createElement('div');
                    ov.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.4);z-index:199;';
                    ov.addEventListener('click', function() { sb.classList.remove('mobile-open'); if (ov.parentNode) ov.parentNode.removeChild(ov); });
                    document.body.appendChild(ov);
                }
            } else {
                sb.classList.toggle('collapsed');
                localStorage.setItem('deptSidebarCollapsed', sb.classList.contains('collapsed') ? '1' : '0');
            }
        });
    }
    if (mobileBtn && sb) {
        var overlay = null;
        mobileBtn.addEventListener('click', function() {
            if (window.innerWidth > 768) return;
            sb.classList.toggle('mobile-open');
            if (sb.classList.contains('mobile-open')) {
                overlay = document.createElement('div');
                overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.4);z-index:199;';
                overlay.addEventListener('click', function() { sb.classList.remove('mobile-open'); if (overlay && overlay.parentNode) overlay.parentNode.removeChild(overlay); overlay = null; });
                document.body.appendChild(overlay);
            } else if (overlay && overlay.parentNode) { overlay.parentNode.removeChild(overlay); overlay = null; }
        });
    }
})();
