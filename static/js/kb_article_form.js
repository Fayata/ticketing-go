(function(){
    var sectionIndex = 0;
    var container = document.getElementById('kbSectionsContainer');
    var addBtn = document.getElementById('kbAddSection');

    function layoutOptions(selected) {
        var opts = [
            { value: 'full', label: 'Gambar penuh (lebar 100%)' },
            { value: 'half', label: 'Setengah lebar (50%)' },
            { value: 'thumb', label: 'Thumbnail (kecil)' }
        ];
        var h = '<select name="section_' + sectionIndex + '_layout" class="form-input kb-section-layout">';
        opts.forEach(function(o) {
            h += '<option value="' + o.value + '"' + (o.value === selected ? ' selected' : '') + '>' + o.label + '</option>';
        });
        h += '</select>';
        return h;
    }

    function addSection(data) {
        data = data || {};
        var idx = sectionIndex++;
        var div = document.createElement('div');
        div.className = 'kb-section-block';
        div.dataset.index = idx;
        div.innerHTML =
            '<div class="kb-section-header">' +
                '<span class="kb-section-title">Sub judul ' + (idx + 1) + '</span>' +
                '<button type="button" class="kb-section-remove btn-outline-teal" aria-label="Hapus section">Hapus</button>' +
            '</div>' +
            '<div class="form-group">' +
                '<label class="form-label">Sub judul</label>' +
                '<input type="text" name="section_' + idx + '_subtitle" class="form-input" placeholder="Judul section" value="' + (data.subtitle || '').replace(/"/g, '&quot;') + '">' +
            '</div>' +
            '<div class="form-group">' +
                '<label class="form-label">Konten</label>' +
                '<textarea name="section_' + idx + '_content" class="form-input" rows="4" placeholder="Teks section...">' + (data.content || '').replace(/</g, '&lt;').replace(/>/g, '&gt;') + '</textarea>' +
            '</div>' +
            '<div class="form-group">' +
                '<label class="form-label">Gambar (opsional)</label>' +
                '<input type="file" name="section_' + idx + '_image" class="form-input kb-section-image" accept="image/jpeg,image/png,image/gif,image/webp">' +
                '<span class="form-hint">Maks. 5 MB. JPG, PNG, GIF, WebP.</span>' +
            '</div>' +
            '<div class="form-group">' +
                '<label class="form-label">Layout gambar</label>' +
                layoutOptions(data.image_layout || 'full') +
            '</div>';
        container.appendChild(div);
        div.querySelector('.kb-section-remove').addEventListener('click', function() {
            div.remove();
        });
    }

    addBtn.addEventListener('click', function() { addSection(); });
    addSection();
})();