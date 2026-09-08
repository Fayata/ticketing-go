(function() {
    'use strict';

    function initSLAPolicyForm() {
        initCompanyDeptSync();
        initDefaultScopeSync();
        initDurationCalculators();
        initFormSubmitValidation();
    }

    // Synchronize PT & Department selectors
    function initCompanyDeptSync() {
        var companySelect = document.getElementById('company_id');
        var deptSelect = document.getElementById('department_id');
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
            filterDepartments();
        });

        filterDepartments();
    }

    // Synchronize "Default Sistem" checkbox
    function initDefaultScopeSync() {
        var defaultCheckbox = document.getElementById('is_default');
        var companySelect = document.getElementById('company_id');
        var deptSelect = document.getElementById('department_id');
        if (!defaultCheckbox || !companySelect || !deptSelect) return;

        function updateFieldsState() {
            if (defaultCheckbox.checked) {
                companySelect.value = '';
                deptSelect.value = '';
                companySelect.disabled = true;
                deptSelect.disabled = true;
            } else {
                companySelect.disabled = false;
                deptSelect.disabled = false;
            }
        }

        defaultCheckbox.addEventListener('change', updateFieldsState);
        updateFieldsState();
    }

    // Live calculations and previews for response & resolution times
    function initDurationCalculators() {
        var priorities = ['high', 'med', 'low'];

        function updatePreviews() {
            priorities.forEach(function(p) {
                // Response time
                var respH = parseInt(document.getElementById('resp_' + p + '_hours').value || '0', 10);
                var respM = parseInt(document.getElementById('resp_' + p + '_minutes').value || '0', 10);
                var totalRespMinutes = respH * 60 + respM;
                var previewResp = document.getElementById('preview_resp_' + p);
                if (previewResp) {
                    if (totalRespMinutes <= 0) {
                        previewResp.textContent = '⚠️ Waktu respon minimal 1 menit';
                        previewResp.style.color = '#dc2626';
                    } else {
                        var parts = [];
                        if (respH > 0) parts.push(respH + ' Jam');
                        if (respM > 0) parts.push(respM + ' Menit');
                        previewResp.textContent = 'Total: ' + parts.join(' ') + ' (' + totalRespMinutes + ' Menit)';
                        previewResp.style.color = '#059669';
                    }
                }

                // Resolution time
                var resolD = parseInt(document.getElementById('resol_' + p + '_days').value || '0', 10);
                var resolH = parseInt(document.getElementById('resol_' + p + '_hours').value || '0', 10);
                var totalResolHours = resolD * 24 + resolH;
                var previewResol = document.getElementById('preview_resol_' + p);
                if (previewResol) {
                    if (totalResolHours <= 0) {
                        previewResol.textContent = '⚠️ Waktu resolusi minimal 1 jam';
                        previewResol.style.color = '#dc2626';
                    } else {
                        var rParts = [];
                        if (resolD > 0) rParts.push(resolD + ' Hari');
                        if (resolH > 0) rParts.push(resolH + ' Jam');
                        previewResol.textContent = 'Total: ' + rParts.join(' ') + ' (' + totalResolHours + ' Jam)';
                        previewResol.style.color = '#059669';
                    }
                }
            });
        }

        var inputs = document.querySelectorAll('.sla-num-input');
        inputs.forEach(function(input) {
            input.addEventListener('input', updatePreviews);
        });

        updatePreviews();
    }

    // Pre-submit validation
    function initFormSubmitValidation() {
        var form = document.getElementById('slaPolicyForm');
        if (!form) return;

        form.addEventListener('submit', function(e) {
            var priorities = ['high', 'med', 'low'];
            for (var i = 0; i < priorities.length; i++) {
                var p = priorities[i];
                var respH = parseInt(document.getElementById('resp_' + p + '_hours').value || '0', 10);
                var respM = parseInt(document.getElementById('resp_' + p + '_minutes').value || '0', 10);
                var resolD = parseInt(document.getElementById('resol_' + p + '_days').value || '0', 10);
                var resolH = parseInt(document.getElementById('resol_' + p + '_hours').value || '0', 10);

                if (respH * 60 + respM <= 0) {
                    e.preventDefault();
                    alert('Target First Response Time untuk prioritas ' + p.toUpperCase() + ' minimal 1 menit.');
                    document.getElementById('resp_' + p + '_minutes').focus();
                    return;
                }

                if (resolD * 24 + resolH <= 0) {
                    e.preventDefault();
                    alert('Target Resolusi untuk prioritas ' + p.toUpperCase() + ' minimal 1 jam.');
                    document.getElementById('resol_' + p + '_hours').focus();
                    return;
                }
            }
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initSLAPolicyForm);
    } else {
        initSLAPolicyForm();
    }
})();
