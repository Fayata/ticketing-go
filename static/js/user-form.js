/* Tampilkan sembunyikan field departemen sesuai role (form user admin) */
function toggleDepartment() {
    var role = document.getElementById('roleSelect').value;
    var deptDiv = document.getElementById('deptContainer');
    if (deptDiv) {
        deptDiv.style.display = role === 'staff' ? 'block' : 'none';
    }
}
