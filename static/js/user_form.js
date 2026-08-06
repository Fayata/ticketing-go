(function(){
    var usernameEl = document.getElementById('ufUsername');
    var emailEl = document.getElementById('ufEmail');
    var roleRadios = document.querySelectorAll('input[name="role"]');
    var roleOptions = document.querySelectorAll('.uf-role-option');
    var deptContainer = document.getElementById('ufDeptContainer');
    var pvAvatar = document.getElementById('pvAvatar');
    var pvName = document.getElementById('pvName');
    var pvEmail = document.getElementById('pvEmail');
    var pvRole = document.getElementById('pvRole');
    var pvAccess = document.getElementById('pvAccess');

    var roleMap = { user: 'User', staff: 'Staff', admin: 'Admin' };
    var accessMap = { user: 'Miliknya sendiri', staff: 'Tiket departemen', admin: 'Semua tiket' };

    function updatePreview() {
        var u = usernameEl.value.trim() || 'username';
        var e = emailEl.value.trim() || 'belum diisi';
        var role = 'user';
        roleRadios.forEach(function(r){ if(r.checked) role = r.value; });

        pvAvatar.textContent = u.charAt(0).toUpperCase();
        pvName.textContent = u;
        pvEmail.textContent = e;
        pvRole.textContent = roleMap[role] || 'User';
        pvAccess.textContent = accessMap[role] || 'Miliknya sendiri';

        deptContainer.classList.toggle('visible', role === 'staff');

        roleOptions.forEach(function(opt){
            var radio = opt.querySelector('input[type="radio"]');
            opt.classList.toggle('active', radio.checked);
        });
    }

    usernameEl.addEventListener('input', updatePreview);
    emailEl.addEventListener('input', updatePreview);
    roleRadios.forEach(function(r){ r.addEventListener('change', updatePreview); });
    updatePreview();
})();

function togglePw(){
    var pw = document.getElementById('ufPassword');
    var btn = pw.nextElementSibling;
    if(pw.type === 'password'){ pw.type = 'text'; btn.textContent = '🙈'; }
    else { pw.type = 'password'; btn.textContent = '👁'; }
}