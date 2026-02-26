/* Picker emoji & warna untuk form kategori KB (admin) */
(function(){
    var emojis = ['📄','📁','🖥️','⚙️','🔧','📋','📌','📎','📂','💡','🔔','📢','📖','📚','🎯','✅','❓','❗','🔒','🔓','📧','📞','🌐','💬','👤','👥','🏢','📊','📈','📉','🛠️','🔨','⚡','🔥','⭐','❤️','💚','💙','📝','✏️','🗂️','📑','🔖','📌','🎨','🖼️','📷','📹','🔍','📌','🏷️','📦','📤','📥','💾','🗃️','📂','📅','⏰','🔔','✔️','➡️','📌','🔗'];
    var colorMap = { green:'#d1fae5', cyan:'#cffafe', indigo:'#e0e7ff', amber:'#fef3c7', rose:'#ffe4e6', violet:'#ede9fe' };
    var iconInput = document.getElementById('kb_icon');
    var emojiPreview = document.getElementById('emojiPreview');
    var emojiTrigger = document.getElementById('emojiTrigger');
    var emojiDropdown = document.getElementById('emojiDropdown');
    var colorInput = document.getElementById('kb_color_class');
    var colorPreview = document.getElementById('colorPreview');
    var colorLabel = document.getElementById('colorLabel');
    var colorSample = document.getElementById('colorSample');
    var colorTrigger = document.getElementById('colorTrigger');
    var colorDropdown = document.getElementById('colorDropdown');
    var grid = document.getElementById('emojiGrid');
    if (!grid || !iconInput) return;

    emojis.forEach(function(e){
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'kb-emoji-cell';
        btn.textContent = e;
        btn.addEventListener('click', function(){ iconInput.value = e; emojiPreview.textContent = e; if (emojiDropdown) emojiDropdown.style.display = 'none'; });
        grid.appendChild(btn);
    });

    if (emojiTrigger && emojiDropdown) emojiTrigger.addEventListener('click', function(ev){ ev.preventDefault(); if (colorDropdown) colorDropdown.style.display = 'none'; emojiDropdown.style.display = emojiDropdown.style.display === 'none' ? 'block' : 'none'; });
    if (colorTrigger && colorDropdown) colorTrigger.addEventListener('click', function(ev){ ev.preventDefault(); if (emojiDropdown) emojiDropdown.style.display = 'none'; colorDropdown.style.display = colorDropdown.style.display === 'none' ? 'block' : 'none'; });

    function setColor(cls) {
        colorInput.value = cls;
        if (colorPreview) colorPreview.style.background = colorMap[cls] || colorMap.green;
        if (colorLabel) colorLabel.textContent = cls;
        if (colorSample) colorSample.style.background = colorMap[cls] || colorMap.green;
        document.querySelectorAll('.kb-color-cell').forEach(function(el){ el.classList.toggle('selected', el.getAttribute('data-class') === cls); });
    }
    setColor('green');
    document.querySelectorAll('.kb-color-cell').forEach(function(el){
        el.addEventListener('click', function(){ setColor(el.getAttribute('data-class')); if (colorDropdown) colorDropdown.style.display = 'none'; });
    });

    document.addEventListener('click', function(ev){
        if (!ev.target.closest('.kb-picker-wrap')) { if (emojiDropdown) emojiDropdown.style.display = 'none'; if (colorDropdown) colorDropdown.style.display = 'none'; }
    });
})();
