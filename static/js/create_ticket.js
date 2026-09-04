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

    // Attachment Handling
    var dropzone = document.getElementById('ticketDropzone');
    var fileInput = document.getElementById('attachmentsInput');
    var previewList = document.getElementById('previewList');
    var errorBox = document.getElementById('attachmentError');

    if (dropzone && fileInput) {
        var maxFiles = 5;
        var maxSizeBytes = 5 * 1024 * 1024;
        var allowedTypes = ['image/jpeg', 'image/png', 'image/gif', 'image/webp'];
        var currentFiles = [];
        var activePreviewUrls = [];

        var canUseDataTransfer = (function() {
            try {
                var dt = new DataTransfer();
                return !!(dt && dt.items && typeof dt.items.add === 'function');
            } catch (e) {
                return false;
            }
        })();

        function showError(msg) {
            if (!errorBox) return;
            errorBox.textContent = msg;
            errorBox.classList.remove('hidden');
        }

        function clearError() {
            if (!errorBox) return;
            errorBox.textContent = '';
            errorBox.classList.add('hidden');
        }

        function formatBytes(bytes) {
            if (bytes < 1024) return bytes + ' B';
            if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
            return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
        }

        function revokePreviewUrls() {
            activePreviewUrls.forEach(function(u) {
                try {
                    URL.revokeObjectURL(u);
                } catch (e) {}
            });
            activePreviewUrls = [];
        }

        window.addEventListener('beforeunload', revokePreviewUrls);

        function updateFileInput() {
            if (currentFiles.length === 0) {
                fileInput.value = '';
                return;
            }
            if (canUseDataTransfer) {
                try {
                    var dt = new DataTransfer();
                    currentFiles.forEach(function(f) {
                        dt.items.add(f);
                    });
                    fileInput.files = dt.files;
                } catch (e) {
                    canUseDataTransfer = false;
                }
            }
        }

        function renderPreviews() {
            if (!previewList) return;
            revokePreviewUrls();
            previewList.innerHTML = '';
            currentFiles.forEach(function(file, idx) {
                var card = document.createElement('div');
                card.className = 'preview-card';

                var thumbWrap = document.createElement('div');
                thumbWrap.className = 'preview-thumb-wrap';

                var img = document.createElement('img');
                img.className = 'preview-thumb';
                img.alt = file.name;

                var imgLoaded = false;
                try {
                    var objectUrl = URL.createObjectURL(file);
                    activePreviewUrls.push(objectUrl);
                    img.src = objectUrl;
                    imgLoaded = true;
                } catch (err) {
                    var errIcon = document.createElement('span');
                    errIcon.className = 'preview-fallback-icon';
                    errIcon.textContent = 'IMG';
                    thumbWrap.appendChild(errIcon);
                }

                img.onerror = function() {
                    this.style.display = 'none';
                    if (!thumbWrap.querySelector('.preview-error-badge')) {
                        var errBadge = document.createElement('div');
                        errBadge.className = 'preview-error-badge';
                        errBadge.textContent = 'Preview tidak tersedia';
                        thumbWrap.appendChild(errBadge);
                    }
                };

                if (imgLoaded) {
                    thumbWrap.appendChild(img);
                }

                var removeBtn = document.createElement('button');
                removeBtn.type = 'button';
                removeBtn.className = 'preview-remove-btn';
                removeBtn.title = 'Hapus file';
                removeBtn.setAttribute('aria-label', 'Hapus file');
                removeBtn.innerHTML = '&times;';
                removeBtn.setAttribute('data-index', idx);
                removeBtn.addEventListener('click', function(e) {
                    e.stopPropagation();
                    currentFiles.splice(idx, 1);
                    updateFileInput();
                    renderPreviews();
                    clearError();
                });

                var info = document.createElement('div');
                info.className = 'preview-info';

                var name = document.createElement('span');
                name.className = 'preview-name';
                name.textContent = file.name;
                name.title = file.name;

                var size = document.createElement('span');
                size.className = 'preview-size';
                size.textContent = formatBytes(file.size);

                info.appendChild(name);
                info.appendChild(size);

                card.appendChild(thumbWrap);
                card.appendChild(removeBtn);
                card.appendChild(info);
                previewList.appendChild(card);
            });
        }

        function handleNewFiles(fileList) {
            clearError();
            var incoming = Array.from(fileList);
            if (currentFiles.length + incoming.length > maxFiles) {
                showError('Maksimal ' + maxFiles + ' file gambar yang dapat dilampirkan.');
                return;
            }

            for (var i = 0; i < incoming.length; i++) {
                var f = incoming[i];
                var isImage = allowedTypes.indexOf(f.type) !== -1 || f.name.match(/\.(jpg|jpeg|png|gif|webp)$/i);
                if (!isImage) {
                    showError('Format file "' + f.name + '" tidak didukung. Hanya JPG, PNG, GIF, WebP yang diizinkan.');
                    return;
                }
                if (f.size > maxSizeBytes) {
                    showError('Ukuran file "' + f.name + '" melebihi batas 5 MB (' + formatBytes(f.size) + ').');
                    return;
                }
            }

            incoming.forEach(function(f) {
                currentFiles.push(f);
            });

            updateFileInput();
            renderPreviews();
        }

        dropzone.addEventListener('click', function() {
            fileInput.click();
        });

        fileInput.addEventListener('change', function() {
            if (this.files && this.files.length > 0) {
                handleNewFiles(this.files);
            }
        });

        ['dragenter', 'dragover'].forEach(function(eventName) {
            dropzone.addEventListener(eventName, function(e) {
                e.preventDefault();
                e.stopPropagation();
                dropzone.classList.add('dragover');
            });
        });

        ['dragleave', 'drop'].forEach(function(eventName) {
            dropzone.addEventListener(eventName, function(e) {
                e.preventDefault();
                e.stopPropagation();
                dropzone.classList.remove('dragover');
            });
        });

        dropzone.addEventListener('drop', function(e) {
            if (e.dataTransfer && e.dataTransfer.files) {
                handleNewFiles(e.dataTransfer.files);
            }
        });

        var ticketForm = dropzone.closest('form');
        if (ticketForm) {
            ticketForm.addEventListener('submit', function(e) {
                if (currentFiles.length === 0) {
                    fileInput.value = '';
                    return;
                }

                var filesInSync = false;
                if (fileInput.files && fileInput.files.length === currentFiles.length) {
                    filesInSync = true;
                    for (var i = 0; i < currentFiles.length; i++) {
                        if (fileInput.files[i] !== currentFiles[i] && fileInput.files[i].name !== currentFiles[i].name) {
                            filesInSync = false;
                            break;
                        }
                    }
                }

                if (filesInSync) {
                    return;
                }

                if (window.FormData && window.fetch) {
                    e.preventDefault();
                    var submitBtn = ticketForm.querySelector('.btn-submit');
                    if (submitBtn) {
                        submitBtn.classList.add('loading');
                        submitBtn.disabled = true;
                    }

                    var fd = new FormData(ticketForm);
                    if (typeof fd.delete === 'function') {
                        fd.delete('attachments');
                    }
                    currentFiles.forEach(function(f) {
                        fd.append('attachments', f, f.name);
                    });

                    fetch(ticketForm.action, {
                        method: 'POST',
                        body: fd,
                        redirect: 'follow'
                    }).then(function(res) {
                        if ((res.redirected || (res.url && res.url !== ticketForm.action)) && res.url) {
                            window.location.href = res.url;
                        } else {
                            return res.text().then(function(html) {
                                document.open();
                                document.write(html);
                                document.close();
                            });
                        }
                    }).catch(function(err) {
                        showError('Gagal mengirim tiket: ' + (err.message || 'Terjadi kesalahan jaringan'));
                        if (submitBtn) {
                            submitBtn.classList.remove('loading');
                            submitBtn.disabled = false;
                        }
                    });
                }
            });
        }
    }
});
