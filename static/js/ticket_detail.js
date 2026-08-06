const root = document.documentElement;
  
  const savedTheme = localStorage.getItem('ticket-theme') || 'dark';
  root.setAttribute('data-theme', savedTheme);

  const infoBtn = document.getElementById('infoToggle');
  const drawer = document.getElementById('infoDrawer');
  infoBtn.addEventListener('click', () => {
    drawer.classList.toggle('hidden');
    infoBtn.classList.toggle('active');
  });

  const ta = document.getElementById('message');
  if(ta) {
      ta.addEventListener('input', function(){
        this.style.height = 'auto';
        this.style.height = Math.min(this.scrollHeight, 110) + 'px';
      });
      // Submit on enter
      ta.addEventListener('keydown', function(e) {
          if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              if(this.value.trim() !== '') {
                  this.form.submit();
              }
          }
      });
  }

  // scroll chat to bottom
  const chatThread = document.getElementById('chatThread');
  if(chatThread) {
      chatThread.scrollTop = chatThread.scrollHeight;
  }