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
});
