document.addEventListener('DOMContentLoaded', function() {
    var companySelect = document.getElementById('company_id');
    var deptSelect = document.getElementById('department');
    if (!companySelect || !deptSelect) return;

    var allDeptOptions = Array.from(deptSelect.querySelectorAll('option[data-company]'));

    function filterDepartments() {
        var selectedCompany = companySelect.value;
        var currentDeptVal = deptSelect.value;
        var hasMatchingCurrent = false;

        allDeptOptions.forEach(function(opt) {
            var compId = opt.getAttribute('data-company');
            if (!selectedCompany || compId === selectedCompany || compId === '0') {
                opt.style.display = '';
                opt.disabled = false;
                if (opt.value === currentDeptVal) {
                    hasMatchingCurrent = true;
                }
            } else {
                opt.style.display = 'none';
                opt.disabled = true;
            }
        });

        if (!hasMatchingCurrent && currentDeptVal !== '') {
            deptSelect.value = '';
        }
    }

    companySelect.addEventListener('change', function() {
        deptSelect.value = '';
        filterDepartments();
    });

    filterDepartments();
});
