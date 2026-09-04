  const infoBtn = document.getElementById('infoToggle');
  const drawer = document.getElementById('infoDrawer');
  if (infoBtn && drawer) {
    infoBtn.addEventListener('click', () => {
      drawer.classList.toggle('hidden');
      infoBtn.classList.toggle('active');
    });
  }

  // Reply Attachment Handling
  const replyFileInput = document.getElementById('replyAttachmentInput');
  const replyTray = document.getElementById('replyPreviewTray');
  const replyError = document.getElementById('replyError');
  let currentReplyFiles = [];
  let activeReplyUrls = [];

  let canUseDataTransfer = (function() {
    try {
      const dt = new DataTransfer();
      return !!(dt && dt.items && typeof dt.items.add === 'function');
    } catch (e) {
      return false;
    }
  })();

  function showReplyError(msg) {
    if (!replyError) return;
    replyError.textContent = msg;
    replyError.classList.remove('hidden');
  }

  function clearReplyError() {
    if (!replyError) return;
    replyError.textContent = '';
    replyError.classList.add('hidden');
  }

  function revokeReplyUrls() {
    activeReplyUrls.forEach(u => {
      try { URL.revokeObjectURL(u); } catch (e) {}
    });
    activeReplyUrls = [];
  }

  window.addEventListener('beforeunload', revokeReplyUrls);

  function updateReplyFileInput() {
    if (currentReplyFiles.length === 0) {
      if (replyFileInput) replyFileInput.value = '';
      return;
    }
    if (canUseDataTransfer && replyFileInput) {
      try {
        const dt = new DataTransfer();
        currentReplyFiles.forEach(f => dt.items.add(f));
        replyFileInput.files = dt.files;
      } catch (e) {
        canUseDataTransfer = false;
      }
    }
  }

  function renderReplyPreviews() {
    if (!replyTray) return;
    revokeReplyUrls();
    replyTray.innerHTML = '';
    if (currentReplyFiles.length === 0) {
      replyTray.classList.add('hidden');
      return;
    }
    replyTray.classList.remove('hidden');

    currentReplyFiles.forEach((file, idx) => {
      const item = document.createElement('div');
      item.className = 'reply-preview-item';

      const img = document.createElement('img');
      img.className = 'reply-preview-img';
      img.alt = file.name;

      let imgLoaded = false;
      try {
        const objectUrl = URL.createObjectURL(file);
        activeReplyUrls.push(objectUrl);
        img.src = objectUrl;
        imgLoaded = true;
      } catch (err) {
        const errSpan = document.createElement('span');
        errSpan.className = 'reply-preview-error';
        errSpan.textContent = 'IMG';
        item.appendChild(errSpan);
      }

      img.onerror = function() {
        this.style.display = 'none';
        if (!item.querySelector('.reply-preview-error')) {
          const errSpan = document.createElement('span');
          errSpan.className = 'reply-preview-error';
          errSpan.textContent = '!';
          item.appendChild(errSpan);
        }
      };

      if (imgLoaded) {
        item.appendChild(img);
      }

      const removeBtn = document.createElement('button');
      removeBtn.type = 'button';
      removeBtn.className = 'reply-preview-remove';
      removeBtn.title = 'Hapus';
      removeBtn.setAttribute('aria-label', 'Hapus file');
      removeBtn.innerHTML = '&times;';
      removeBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        currentReplyFiles.splice(idx, 1);
        updateReplyFileInput();
        renderReplyPreviews();
        clearReplyError();
      });

      item.appendChild(removeBtn);
      replyTray.appendChild(item);
    });
  }

  if (replyFileInput) {
    const maxFiles = 5;
    const maxSizeBytes = 5 * 1024 * 1024;
    const allowedTypes = ['image/jpeg', 'image/png', 'image/gif', 'image/webp'];

    replyFileInput.addEventListener('change', function() {
      clearReplyError();
      if (!this.files || this.files.length === 0) return;

      const incoming = Array.from(this.files);
      if (currentReplyFiles.length + incoming.length > maxFiles) {
        showReplyError('Maksimal ' + maxFiles + ' file gambar yang dapat dilampirkan.');
        return;
      }

      for (let i = 0; i < incoming.length; i++) {
        const f = incoming[i];
        const isImage = allowedTypes.includes(f.type) || f.name.match(/\.(jpg|jpeg|png|gif|webp)$/i);
        if (!isImage) {
          showReplyError('Format file "' + f.name + '" tidak didukung. Hanya JPG, PNG, GIF, WebP yang diizinkan.');
          return;
        }
        if (f.size > maxSizeBytes) {
          const mb = (f.size / (1024 * 1024)).toFixed(1);
          showReplyError('Ukuran file "' + f.name + '" melebihi 5 MB (' + mb + ' MB).');
          return;
        }
      }

      incoming.forEach(f => currentReplyFiles.push(f));
      updateReplyFileInput();
      renderReplyPreviews();
    });
  }

  const ta = document.getElementById('message');
  if(ta) {
      ta.addEventListener('input', function(){
        this.style.height = 'auto';
        this.style.height = Math.min(this.scrollHeight, 110) + 'px';
      });
      // Submit on enter if message or attachment is present
      ta.addEventListener('keydown', function(e) {
          if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              if (this.value.trim() !== '' || currentReplyFiles.length > 0) {
                  if (typeof this.form.requestSubmit === 'function') {
                      this.form.requestSubmit();
                  } else {
                      this.form.dispatchEvent(new Event('submit', { cancelable: true, bubbles: true }));
                  }
              }
          }
      });
  }

  const replyForm = replyFileInput ? replyFileInput.closest('form') : null;
  if (replyForm) {
    replyForm.addEventListener('submit', function(e) {
      const msg = ta ? ta.value.trim() : '';
      if (msg === '' && currentReplyFiles.length === 0) {
        e.preventDefault();
        showReplyError('Pesan atau lampiran gambar harus diisi');
        return;
      }

      if (currentReplyFiles.length === 0) {
        if (replyFileInput) replyFileInput.value = '';
        return;
      }

      let inSync = false;
      if (replyFileInput && replyFileInput.files && replyFileInput.files.length === currentReplyFiles.length) {
        inSync = true;
        for (let i = 0; i < currentReplyFiles.length; i++) {
          if (replyFileInput.files[i] !== currentReplyFiles[i] && replyFileInput.files[i].name !== currentReplyFiles[i].name) {
            inSync = false;
            break;
          }
        }
      }

      if (inSync) {
        return;
      }

      if (window.FormData && window.fetch) {
        e.preventDefault();
        const sendBtn = replyForm.querySelector('.send-btn');
        if (sendBtn) sendBtn.disabled = true;

        const fd = new FormData(replyForm);
        if (typeof fd.delete === 'function') {
          fd.delete('attachments');
        }
        currentReplyFiles.forEach(f => fd.append('attachments', f, f.name));

        fetch(replyForm.action, {
          method: 'POST',
          body: fd,
          redirect: 'follow'
        }).then(res => {
          if ((res.redirected || (res.url && res.url !== replyForm.action)) && res.url) {
            window.location.href = res.url;
          } else {
            return res.text().then(html => {
              document.open();
              document.write(html);
              document.close();
            });
          }
        }).catch(err => {
          showReplyError('Gagal mengirim balasan: ' + (err.message || 'Kesalahan jaringan'));
          if (sendBtn) sendBtn.disabled = false;
        });
      }
    });
  }

  // scroll chat to bottom
  const chatThread = document.getElementById('chatThread');
  if(chatThread) {
      chatThread.scrollTop = chatThread.scrollHeight;
  }

  // Lightbox Modal Handling
  const lightboxModal = document.getElementById('lightboxModal');
  const lightboxImg = document.getElementById('lightboxImg');
  const lightboxCaption = document.getElementById('lightboxCaption');
  const lightboxClose = document.getElementById('lightboxClose');

  function openLightbox(src, filename) {
    if (!lightboxModal || !lightboxImg) return;
    lightboxImg.onerror = function() {
      if (lightboxCaption) {
        lightboxCaption.textContent = (filename ? filename + ' - ' : '') + 'Gagal memuat gambar (file tidak ditemukan atau rusak)';
      }
    };
    lightboxImg.src = src;
    if (lightboxCaption) {
      lightboxCaption.textContent = filename || '';
    }
    lightboxModal.classList.remove('hidden');
    document.body.style.overflow = 'hidden';
  }

  function closeLightbox() {
    if (!lightboxModal) return;
    lightboxModal.classList.add('hidden');
    if (lightboxImg) lightboxImg.src = '';
    document.body.style.overflow = '';
  }

  if (lightboxClose) {
    lightboxClose.addEventListener('click', closeLightbox);
  }

  if (lightboxModal) {
    lightboxModal.addEventListener('click', function(e) {
      if (e.target === lightboxModal) {
        closeLightbox();
      }
    });
  }

  document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape' && lightboxModal && !lightboxModal.classList.contains('hidden')) {
      closeLightbox();
    }
  });

  document.addEventListener('click', function(e) {
    const link = e.target.closest('.attachment-link');
    if (link && link.dataset.preview) {
      e.preventDefault();
      openLightbox(link.dataset.preview, link.dataset.filename);
    }
  });