// Live Markdown preview for the tournament description editor on the create
// and manage pages. The markup carries the endpoint via `data-preview-url`
// and the textarea id via `data-preview-for`, so this script stays
// parameterless and CSP-friendly (no inline script, no inline handlers).

(function () {
    document.addEventListener('DOMContentLoaded', function () {
        var preview = document.querySelector('[data-preview-for]');
        if (!preview) return;

        var textarea = document.getElementById(preview.dataset.previewFor);
        var previewURL = preview.dataset.previewUrl;
        if (!textarea || !previewURL) return;

        var match = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]+)/);
        if (!match) return;
        var csrf = match[1];

        var debounceTimer = null;
        var lastSource = null;

        function render(src) {
            if (src === lastSource) return;
            lastSource = src;
            if (src === '') {
                preview.innerHTML = '';
                return;
            }
            fetch(previewURL, {
                method: 'POST',
                credentials: 'same-origin',
                headers: {
                    'Content-Type': 'application/x-www-form-urlencoded',
                    'X-CSRF-Token': csrf,
                },
                body: 'description=' + encodeURIComponent(src),
            })
                .then(function (r) { return r.ok ? r.text() : null; })
                .then(function (html) {
                    if (html === null) return;
                    // The only innerHTML of server-supplied markup in the
                    // app. Safe because the server renders with goldmark in
                    // its default (non-Unsafe) mode: raw HTML is omitted and
                    // dangerous URL schemes are dropped. Never point this at
                    // an endpoint that echoes unsanitized input.
                    preview.innerHTML = html;
                })
                // Leave the last good preview up rather than blanking it.
                .catch(function () { });
        }

        textarea.addEventListener('input', function () {
            clearTimeout(debounceTimer);
            // Debounce so we don't render on every keystroke.
            debounceTimer = setTimeout(function () {
                render(textarea.value);
            }, 300);
        });

        // Seed the pane with whatever is already in the box (edit form).
        render(textarea.value);
    });
})();
