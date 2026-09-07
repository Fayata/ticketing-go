(function () {
    'use strict';

    document.addEventListener('DOMContentLoaded', function () {
        var articleContainer = document.querySelector('[data-article-id]');
        var articleId = articleContainer ? articleContainer.getAttribute('data-article-id') : null;

        // 1. Scroll-based view counter
        if (articleId) {
            var viewRecorded = false;
            var bodyEl = document.getElementById('kbArticleBody');

            function checkViewScroll() {
                if (viewRecorded || !bodyEl) return;
                var rect = bodyEl.getBoundingClientRect();
                if (rect.bottom <= (window.innerHeight || document.documentElement.clientHeight) + 100) {
                    viewRecorded = true;
                    fetch('api/kb/article/view', {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json',
                            'X-Requested-With': 'XMLHttpRequest'
                        },
                        body: JSON.stringify({ article_id: parseInt(articleId, 10) })
                    })
                    .then(function (res) { return res.json(); })
                    .then(function (data) {
                        if (data && data.ok) {
                            var viewCountEl = document.getElementById('articleViewsCount');
                            if (viewCountEl) {
                                var cur = parseInt(viewCountEl.textContent, 10) || 0;
                                viewCountEl.textContent = cur + 1;
                            }
                        }
                    })
                    .catch(function () {});
                }
            }

            window.addEventListener('scroll', checkViewScroll, { passive: true });
            window.addEventListener('resize', checkViewScroll, { passive: true });
            checkViewScroll();
        }

        // 2. Sidebar Tab Switching
        var tabButtons = document.querySelectorAll('.kb-tab-btn');
        var tabPanes = {
            related: document.getElementById('kbTabRelated'),
            popular: document.getElementById('kbTabPopular')
        };

        tabButtons.forEach(function (btn) {
            btn.addEventListener('click', function () {
                var targetTab = this.getAttribute('data-tab');
                tabButtons.forEach(function (b) {
                    b.classList.remove('active');
                    b.setAttribute('aria-pressed', 'false');
                });
                this.classList.add('active');
                this.setAttribute('aria-pressed', 'true');

                if (tabPanes.related) tabPanes.related.hidden = (targetTab !== 'related');
                if (tabPanes.popular) tabPanes.popular.hidden = (targetTab !== 'popular');
            });
        });

        // 3. Smooth scroll for Table of Contents
        var tocLinks = document.querySelectorAll('.kb-toc-link');
        tocLinks.forEach(function (link) {
            link.addEventListener('click', function (e) {
                var href = this.getAttribute('href');
                if (href && href.startsWith('#')) {
                    e.preventDefault();
                    var targetId = href.substring(1);
                    var targetEl = document.getElementById(targetId);
                    if (targetEl) {
                        targetEl.scrollIntoView({ behavior: 'smooth', block: 'start' });
                        history.pushState(null, '', href);

                        tocLinks.forEach(function (l) { l.classList.remove('active'); });
                        link.classList.add('active');
                    }
                }
            });
        });

        // 4. Highlight active section in Table of Contents on scroll
        var sections = document.querySelectorAll('.kb-article-section[id]');
        if (sections.length > 0 && tocLinks.length > 0) {
            function updateActiveToc() {
                var scrollPos = window.scrollY + 120;
                var currentId = '';

                sections.forEach(function (sec) {
                    var top = sec.offsetTop;
                    var height = sec.offsetHeight;
                    if (scrollPos >= top && scrollPos < top + height) {
                        currentId = sec.getAttribute('id');
                    }
                });

                if (currentId) {
                    tocLinks.forEach(function (l) {
                        if (l.getAttribute('href') === '#' + currentId) {
                            l.classList.add('active');
                        } else {
                            l.classList.remove('active');
                        }
                    });
                }
            }

            window.addEventListener('scroll', updateActiveToc, { passive: true });
        }

        // 5. Copy Link to Clipboard with Toast
        var shareBtn = document.getElementById('kbShareBtn');
        var toastEl = document.getElementById('kbToast');

        function showToast(msg) {
            if (!toastEl) {
                toastEl = document.createElement('div');
                toastEl.id = 'kbToast';
                toastEl.className = 'kb-toast';
                document.body.appendChild(toastEl);
            }
            toastEl.textContent = msg;
            toastEl.classList.add('show');
            setTimeout(function () {
                toastEl.classList.remove('show');
            }, 3000);
        }

        if (shareBtn) {
            shareBtn.addEventListener('click', function () {
                var url = window.location.href;
                if (navigator.clipboard && navigator.clipboard.writeText) {
                    navigator.clipboard.writeText(url).then(function () {
                        showToast('Tautan artikel berhasil disalin ke clipboard!');
                    }).catch(function () {
                        showToast('Gagal menyalin tautan.');
                    });
                } else {
                    // Fallback
                    var tempInput = document.createElement('input');
                    tempInput.value = url;
                    document.body.appendChild(tempInput);
                    tempInput.select();
                    document.execCommand('copy');
                    document.body.removeChild(tempInput);
                    showToast('Tautan artikel berhasil disalin!');
                }
            });
        }

        // 6. Interactive Helpful Feedback Buttons
        var feedbackActions = document.getElementById('kbFeedbackActions');
        var feedbackResponse = document.getElementById('kbFeedbackResponse');
        var feedbackBtns = document.querySelectorAll('.kb-feedback-btn');

        feedbackBtns.forEach(function (btn) {
            btn.addEventListener('click', function () {
                var val = this.getAttribute('data-value');
                if (feedbackActions && feedbackResponse) {
                    feedbackActions.style.display = 'none';
                    feedbackResponse.hidden = false;
                    feedbackResponse.textContent = val === 'yes'
                        ? 'Terima kasih atas tanggapan positif Anda! 🎉'
                        : 'Terima kasih! Kami akan terus menyempurnakan panduan ini. 🙏';
                }
            });
        });
    });
})();
