/**
 * admin_reports.js
 * Handler untuk interaksi halaman Laporan Kinerja Admin & Fitur Cetak PDF.
 */
document.addEventListener('DOMContentLoaded', function () {
    // Tombol Cetak PDF
    var btnPrint = document.getElementById('btnPrintReport');
    if (btnPrint) {
        btnPrint.addEventListener('click', function () {
            window.print();
        });
    }

    // Auto-submit saat pilihan dropdown periode bulan diubah
    var periodSelect = document.getElementById('periodSelect');
    var reportForm = document.getElementById('reportFilterForm');
    if (periodSelect && reportForm) {
        periodSelect.addEventListener('change', function () {
            reportForm.submit();
        });
    }

    // Auto-submit saat pilihan dropdown perusahaan diubah
    var companySelect = document.getElementById('companySelect');
    if (companySelect && reportForm) {
        companySelect.addEventListener('change', function () {
            reportForm.submit();
        });
    }

    /* [Feature Hidden / Stashed for later release]
    // Accordion Toggle Baris Perusahaan -> Tampilkan / Sembunyikan Departemen
    var companyRows = document.querySelectorAll('.row-company-header');
    companyRows.forEach(function (row) {
        row.addEventListener('click', function (e) {
            // Jangan toggle jika yang diklik adalah link/tombol di dalam baris
            if (e.target.closest('a') || e.target.closest('button')) {
                return;
            }

            var companyId = row.getAttribute('data-company-id');
            if (!companyId) return;

            var chevron = row.querySelector('.company-chevron');
            var isExpanded = row.classList.contains('expanded');
            var deptRows = document.querySelectorAll('.dept-company-' + companyId);

            if (isExpanded) {
                row.classList.remove('expanded');
                if (chevron) chevron.textContent = '▸';
                deptRows.forEach(function (deptRow) {
                    deptRow.style.display = 'none';
                });
            } else {
                row.classList.add('expanded');
                if (chevron) chevron.textContent = '▾';
                deptRows.forEach(function (deptRow) {
                    deptRow.style.display = 'table-row';
                });
            }
        });
    });
    */
});
