(function() {
  'use strict';

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

      const isPDF = file.type === 'application/pdf' || file.name.match(/\.pdf$/i);
      if (isPDF) {
        item.classList.add('reply-preview-pdf');
        const pdfBadge = document.createElement('div');
        pdfBadge.className = 'reply-preview-pdf-badge';
        pdfBadge.innerHTML = '<span class="pdf-tag">PDF</span><span class="pdf-name" title="' + file.name + '">' + file.name + '</span>';
        item.appendChild(pdfBadge);
      } else {
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
    const allowedTypes = ['image/jpeg', 'image/png', 'image/gif', 'image/webp', 'application/pdf'];

    replyFileInput.addEventListener('change', function() {
      clearReplyError();
      if (!this.files || this.files.length === 0) return;

      const incoming = Array.from(this.files);
      if (currentReplyFiles.length + incoming.length > maxFiles) {
        showReplyError('Maksimal ' + maxFiles + ' file yang dapat dilampirkan.');
        return;
      }

      for (let i = 0; i < incoming.length; i++) {
        const f = incoming[i];
        const isSupported = allowedTypes.includes(f.type) || f.name.match(/\.(jpg|jpeg|png|gif|webp|pdf)$/i);
        if (!isSupported) {
          showReplyError('Format file "' + f.name + '" tidak didukung. Hanya file gambar (JPG, PNG, GIF, WebP) dan dokumen PDF yang diizinkan.');
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
        showReplyError('Pesan atau lampiran berkas harus diisi');
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

  // ==========================================================================
  // FEATURE 14: QUICK RESOLUTION ESTIMATION LOGIC
  // ==========================================================================
  const estimateSelect = document.getElementById('estimatePresetSelect');
  const customDateGroup = document.getElementById('customDateGroup');
  const customDateInput = document.getElementById('customDateInput');
  const estimateForm = document.getElementById('estimateForm');

  function updateCustomEstimateVisibility() {
    if (!estimateSelect || !customDateGroup) return;
    if (estimateSelect.value === 'custom') {
      customDateGroup.classList.remove('hidden');
      if (customDateInput) {
        customDateInput.required = true;
        // Restrict to future datetimes only
        const now = new Date();
        now.setMinutes(now.getMinutes() - now.getTimezoneOffset());
        customDateInput.min = now.toISOString().slice(0, 16);
      }
    } else {
      customDateGroup.classList.add('hidden');
      if (customDateInput) {
        customDateInput.required = false;
        customDateInput.value = '';
      }
    }
  }

  if (estimateSelect) {
    estimateSelect.addEventListener('change', updateCustomEstimateVisibility);
  }

  if (estimateForm) {
    estimateForm.addEventListener('submit', function(e) {
      if (estimateSelect && estimateSelect.value === 'custom' && customDateInput) {
        const val = customDateInput.value;
        if (!val) {
          e.preventDefault();
          alert('Silakan tentukan tanggal dan waktu estimasi kustom.');
          customDateInput.focus();
          return;
        }
        const selectedDate = new Date(val);
        if (selectedDate <= new Date()) {
          e.preventDefault();
          alert('Waktu target estimasi harus berada di masa depan.');
          customDateInput.focus();
          return;
        }
      }
    });
  }

  // ==========================================================================
  // FEATURE 15: PRIORITY ADJUSTMENT MODAL & AUDIT VALIDATION
  // ==========================================================================
  const openPriorityBtn = document.getElementById('openPriorityModalBtn');
  const priorityModal = document.getElementById('priorityModal');
  const priorityModalClose = document.getElementById('priorityModalClose');
  const priorityModalCancel = document.getElementById('priorityModalCancel');
  const priorityReason = document.getElementById('priorityReason');
  const reasonCounter = document.getElementById('reasonCharCounter');
  const priorityForm = document.getElementById('priorityForm');
  const priorityFormError = document.getElementById('priorityFormError');

  function openPriorityModal() {
    if (!priorityModal) return;
    priorityModal.classList.remove('hidden');
    document.body.style.overflow = 'hidden';
    if (priorityReason) {
      priorityReason.value = '';
      updateReasonCounter();
    }
    if (priorityFormError) {
      priorityFormError.textContent = '';
      priorityFormError.classList.add('hidden');
    }
  }

  function closePriorityModal() {
    if (!priorityModal) return;
    priorityModal.classList.add('hidden');
    document.body.style.overflow = '';
  }

  function updateReasonCounter() {
    if (!priorityReason || !reasonCounter) return;
    const len = priorityReason.value.trim().length;
    reasonCounter.textContent = len + ' / 5 karakter minimum';
    if (len >= 5) {
      reasonCounter.classList.add('valid');
    } else {
      reasonCounter.classList.remove('valid');
    }
  }

  if (openPriorityBtn) {
    openPriorityBtn.addEventListener('click', openPriorityModal);
  }
  if (priorityModalClose) {
    priorityModalClose.addEventListener('click', closePriorityModal);
  }
  if (priorityModalCancel) {
    priorityModalCancel.addEventListener('click', closePriorityModal);
  }
  if (priorityModal) {
    priorityModal.addEventListener('click', function(e) {
      if (e.target === priorityModal) {
        closePriorityModal();
      }
    });
  }
  if (priorityReason) {
    priorityReason.addEventListener('input', updateReasonCounter);
  }
  if (priorityForm) {
    priorityForm.addEventListener('submit', function(e) {
      if (!priorityReason) return;
      const text = priorityReason.value.trim();
      if (text.length < 5) {
        e.preventDefault();
        if (priorityFormError) {
          priorityFormError.textContent = 'Alasan perubahan prioritas harus diisi minimal 5 karakter.';
          priorityFormError.classList.remove('hidden');
        } else {
          alert('Alasan perubahan prioritas harus diisi minimal 5 karakter.');
        }
        priorityReason.focus();
      }
    });
  }

  // Handle escape key to close priority modal
  document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape' && priorityModal && !priorityModal.classList.contains('hidden')) {
      closePriorityModal();
    }
  });

  // ==========================================================================
  // CLEAN ACTION BUTTON HANDLERS (ZERO INLINE ONCLICK)
  // ==========================================================================
  const btnCloseTicket = document.getElementById('btnCloseTicket');
  if (btnCloseTicket) {
    btnCloseTicket.addEventListener('click', function(e) {
      e.preventDefault();
      const url = this.getAttribute('data-close-url') || this.href;
      if (typeof window.showCustomConfirm === 'function') {
        window.showCustomConfirm(
          'Yakin tutup tiket ini? Tiket akan ditandai selesai dan tidak bisa dibalas lagi.',
          'Tutup Tiket',
          function() { window.location.href = url; }
        );
      } else if (confirm('Yakin tutup tiket ini? Tiket akan ditandai selesai dan tidak bisa dibalas lagi.')) {
        window.location.href = url;
      }
    });
  }

  const btnReleaseTicket = document.getElementById('btnReleaseTicket');
  if (btnReleaseTicket) {
    btnReleaseTicket.addEventListener('click', function(e) {
      e.preventDefault();
      const url = this.getAttribute('data-release-url');
      if (typeof window.showCustomConfirm === 'function') {
        window.showCustomConfirm(
          'Kembalikan tiket ini ke pool? Tiket akan bisa diambil lagi oleh staff lain.',
          'Lepas ke Pool',
          function() { window.location.href = url; }
        );
      } else if (confirm('Kembalikan tiket ini ke pool? Tiket akan bisa diambil lagi oleh staff lain.')) {
        window.location.href = url;
      }
    });
  }

  // ==========================================================================
  // REAL-TIME WEBSOCKET CHAT SYNC
  // ==========================================================================
  if (chatThread && chatThread.dataset.ticketId) {
    const ticketId = chatThread.dataset.ticketId;
    const currentUserId = parseInt(chatThread.dataset.currentUserId || '0', 10);
    const isStaffUser = chatThread.dataset.isStaff === 'true';

    let ws = null;
    let reconnectTimer = null;

    function buildMessageGroup(reply) {
      const group = document.createElement('div');
      group.className = 'msg-group';
      group.setAttribute('data-reply-id', reply.id);

      const isMine = isStaffUser
        ? (reply.is_staff && reply.user_id === currentUserId)
        : (reply.user_id === currentUserId);

      if (isMine) {
        group.classList.add('mine');
      }

      // Avatar
      const avatar = document.createElement('div');
      avatar.className = 'avatar' + (reply.is_staff ? ' agent' : '');
      const initials = (reply.username || 'US').slice(0, 2).toUpperCase();
      avatar.textContent = initials;
      group.appendChild(avatar);

      // Stack
      const stack = document.createElement('div');
      stack.className = 'msg-stack';

      // Name
      const nameEl = document.createElement('div');
      nameEl.className = 'msg-name';
      if (isMine) {
        nameEl.textContent = 'Anda';
      } else {
        const displayName = reply.user_display_name || reply.username || 'User';
        nameEl.textContent = displayName + (reply.is_staff ? ' · Staff' : '');
      }
      stack.appendChild(nameEl);

      // Bubble wrap
      const bubbleWrap = document.createElement('div');
      bubbleWrap.className = 'bubble-wrap';

      const bubble = document.createElement('div');
      bubble.className = 'bubble';

      // Safe multi-line text
      const lines = (reply.message || '').split('\n');
      lines.forEach((line, idx) => {
        if (idx > 0) bubble.appendChild(document.createElement('br'));
        bubble.appendChild(document.createTextNode(line));
      });

      // Attachments
      if (reply.attachments && reply.attachments.length > 0) {
        const gallery = document.createElement('div');
        gallery.className = 'attachment-gallery';

        reply.attachments.forEach(att => {
          if (att.is_pdf) {
            const card = document.createElement('div');
            card.className = 'attachment-doc-card attachment-pdf-card';

            const iconDiv = document.createElement('div');
            iconDiv.className = 'attachment-doc-icon pdf-icon';
            iconDiv.innerHTML = '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line><polyline points="10 9 9 9 8 9"></polyline></svg><span class="attachment-doc-badge">PDF</span>';
            card.appendChild(iconDiv);

            const infoDiv = document.createElement('div');
            infoDiv.className = 'attachment-doc-info';
            const nameSpan = document.createElement('span');
            nameSpan.className = 'attachment-doc-name';
            nameSpan.title = att.file_name;
            nameSpan.textContent = att.file_name;
            infoDiv.appendChild(nameSpan);

            const sizeSpan = document.createElement('span');
            sizeSpan.className = 'attachment-doc-size';
            sizeSpan.textContent = att.formatted_size;
            infoDiv.appendChild(sizeSpan);
            card.appendChild(infoDiv);

            const actionsDiv = document.createElement('div');
            actionsDiv.className = 'attachment-doc-actions';
            const openLink = document.createElement('a');
            openLink.href = att.file_path;
            openLink.className = 'attachment-doc-btn';
            openLink.target = '_blank';
            openLink.rel = 'noopener noreferrer';
            openLink.title = 'Buka PDF';
            openLink.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path><polyline points="15 3 21 3 21 9"></polyline><line x1="10" y1="14" x2="21" y2="3"></line></svg><span>Buka PDF</span>';
            actionsDiv.appendChild(openLink);
            card.appendChild(actionsDiv);

            gallery.appendChild(card);
          } else {
            const thumbCard = document.createElement('div');
            thumbCard.className = 'attachment-thumb-card';

            const aLink = document.createElement('a');
            aLink.href = att.file_path;
            aLink.className = 'attachment-link';
            aLink.target = '_blank';
            aLink.dataset.preview = att.file_path;
            aLink.dataset.filename = att.file_name;

            const img = document.createElement('img');
            img.src = att.file_path;
            img.alt = att.file_name;
            img.className = 'attachment-thumb-img';
            img.loading = 'lazy';
            aLink.appendChild(img);
            thumbCard.appendChild(aLink);

            const metaDiv = document.createElement('div');
            metaDiv.className = 'attachment-thumb-meta';
            const nameSpan = document.createElement('span');
            nameSpan.className = 'attachment-thumb-name';
            nameSpan.title = att.file_name;
            nameSpan.textContent = att.file_name;
            metaDiv.appendChild(nameSpan);

            const sizeSpan = document.createElement('span');
            sizeSpan.className = 'attachment-thumb-size';
            sizeSpan.textContent = att.formatted_size;
            metaDiv.appendChild(sizeSpan);
            thumbCard.appendChild(metaDiv);

            gallery.appendChild(thumbCard);
          }
        });

        bubble.appendChild(gallery);
      }

      bubbleWrap.appendChild(bubble);

      // Time & Read Status
      const timeSpan = document.createElement('span');
      timeSpan.className = 'msg-time';
      timeSpan.textContent = (reply.created_at || '') + ' ';

      if (isMine) {
        const statusSpan = document.createElement('span');
        let statusClass = 'sent';
        let statusCheck = '✓';
        let statusTitle = 'Terkirim';

        if (reply.is_read || reply.read_status === 'read') {
          statusClass = 'read';
          statusCheck = '✓✓';
          statusTitle = 'Sudah dibaca';
        } else if (reply.is_delivered || reply.read_status === 'delivered') {
          statusClass = 'delivered';
          statusCheck = '✓✓';
          statusTitle = 'Tersampaikan';
        }

        statusSpan.className = 'msg-status ' + statusClass;
        statusSpan.textContent = statusCheck;
        statusSpan.title = statusTitle;
        timeSpan.appendChild(statusSpan);
      }

      bubbleWrap.appendChild(timeSpan);

      stack.appendChild(bubbleWrap);
      group.appendChild(stack);

      return group;
    }

    // Determine correct base path (/Ticketing or empty)
    let basePath = '';
    const baseEl = document.querySelector('base');
    if (baseEl && baseEl.getAttribute('href')) {
      basePath = baseEl.getAttribute('href').replace(/\/$/, '');
    } else if (window.location.pathname.indexOf('/Ticketing') !== -1) {
      basePath = '/Ticketing';
    }

    // Track highest reply ID to avoid duplicate rendering
    let lastReplyId = 0;
    function scanLastReplyId() {
      chatThread.querySelectorAll('[data-reply-id]').forEach(el => {
        const id = parseInt(el.getAttribute('data-reply-id') || '0', 10);
        if (id > lastReplyId) lastReplyId = id;
      });
    }
    scanLastReplyId();

    // 1. Auto-sync polling every 2.5 seconds (Fail-safe for microservices & reverse proxies)
    let isPolling = false;
    function pollNewMessages() {
      if (isPolling) return;
      isPolling = true;
      const apiUrl = basePath + '/api/ticket/' + ticketId + '/messages?after=' + lastReplyId;
      fetch(apiUrl, { credentials: 'same-origin' })
        .then(res => {
          if (!res.ok) throw new Error('Status ' + res.status);
          return res.json();
        })
        .then(data => {
          isPolling = false;
          if (data && data.replies && data.replies.length > 0) {
            let hasNew = false;
            data.replies.forEach(reply => {
              if (reply.id > lastReplyId && !chatThread.querySelector('[data-reply-id="' + reply.id + '"]')) {
                const msgElement = buildMessageGroup(reply);
                chatThread.appendChild(msgElement);
                if (reply.id > lastReplyId) lastReplyId = reply.id;
                hasNew = true;
              }
            });
            if (hasNew) {
              chatThread.scrollTop = chatThread.scrollHeight;
            }
          }
        })
        .catch(() => {
          isPolling = false;
        });
    }

    const pollInterval = setInterval(pollNewMessages, 2500);

    // 2. Real-Time WebSocket Connection (Instant Push)
    function initWebSocket() {
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = protocol + '//' + window.location.host + basePath + '/ws/ticket/' + ticketId;

      try {
        ws = new WebSocket(wsUrl);

        ws.onopen = function() {
          console.log('[TicketWS] Terhubung ke ruang obrolan tiket #' + ticketId);
        };

        ws.onmessage = function(event) {
          try {
            const data = JSON.parse(event.data);
            if (data && data.type === 'new_reply' && data.reply) {
              const reply = data.reply;
              if (reply.id > lastReplyId && !chatThread.querySelector('[data-reply-id="' + reply.id + '"]')) {
                const msgElement = buildMessageGroup(reply);
                chatThread.appendChild(msgElement);
                if (reply.id > lastReplyId) lastReplyId = reply.id;
                chatThread.scrollTop = chatThread.scrollHeight;
              }
            } else if (data && data.type === 'messages_read') {
              // Counterpart has read messages: turn all outgoing checks into double-blue
              chatThread.querySelectorAll('.msg-group.mine .msg-status').forEach(el => {
                el.className = 'msg-status read';
                el.textContent = '✓✓';
                el.title = 'Sudah dibaca';
              });
            }
          } catch (e) {
            console.error('[TicketWS] Gagal memproses pesan:', e);
          }
        };

        ws.onclose = function(e) {
          reconnectTimer = setTimeout(initWebSocket, 4000);
        };

        ws.onerror = function() {
          if (ws) ws.close();
        };
      } catch (err) {
        reconnectTimer = setTimeout(initWebSocket, 5000);
      }
    }

    initWebSocket();

    window.addEventListener('beforeunload', function() {
      if (pollInterval) clearInterval(pollInterval);
      if (reconnectTimer) clearTimeout(reconnectTimer);
      if (ws) {
        ws.onclose = null;
        ws.close();
      }
    });
  }
})();