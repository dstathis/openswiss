// Front-end JS for OpenSwiss. Kept as a single small file so it can be
// served from /static and locked down by a strict Content-Security-Policy
// (no inline scripts, no inline event handlers).

// Theme: apply the saved preference before the body paints to avoid a flash
// of the wrong theme. Runs at parse time, before DOMContentLoaded.
(function () {
    var t = localStorage.getItem('theme');
    if (t) document.documentElement.setAttribute('data-theme', t);
})();

function toggleTheme() {
    var root = document.documentElement;
    var current = root.getAttribute('data-theme');
    var next = current === 'light' ? 'dark' : 'light';
    root.setAttribute('data-theme', next);
    localStorage.setItem('theme', next);
    updateIcon(next);
}

function updateIcon(t) {
    var el = document.querySelector('.theme-icon');
    if (el) el.textContent = t === 'light' ? '🌙' : '☀️';
}

document.addEventListener('DOMContentLoaded', function () {
    // Theme toggle button.
    var themeBtn = document.querySelector('.theme-toggle');
    if (themeBtn) themeBtn.addEventListener('click', toggleTheme);
    updateIcon(localStorage.getItem('theme') || 'dark');

    // Mobile nav hamburger.
    var navBtn = document.querySelector('.nav-toggle');
    var navLinks = document.querySelector('.nav-links');
    if (navBtn && navLinks) {
        navBtn.addEventListener('click', function () {
            navLinks.classList.toggle('open');
        });
    }

    // Auto-inject the CSRF token (read from cookie) into every POST form so
    // we don't have to add a hidden input to each template by hand.
    var match = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]+)/);
    if (match) {
        var tok = match[1];
        document.querySelectorAll('form[method="POST"],form[method="post"]').forEach(function (f) {
            if (f.querySelector('input[name="csrf_token"]')) return;
            var input = document.createElement('input');
            input.type = 'hidden';
            input.name = 'csrf_token';
            input.value = tok;
            f.prepend(input);
        });
    }

    // Generic confirm-on-submit. Replaces inline `onsubmit="return confirm(...)"`
    // so a strict CSP can ban inline event handlers entirely. Mark a form
    // with `data-confirm="Are you sure?"` to gate submission on a confirm().
    document.addEventListener('submit', function (e) {
        var form = e.target;
        if (form && form.dataset && form.dataset.confirm) {
            if (!window.confirm(form.dataset.confirm)) {
                e.preventDefault();
            }
        }
    }, true);
});
