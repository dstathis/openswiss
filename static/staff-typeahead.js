// Typeahead for the staff-grant form on the tournament staff page. The
// form carries the search endpoint via `data-search-url` so this script
// stays parameterless and CSP-friendly.

(function () {
    document.addEventListener('DOMContentLoaded', function () {
        var form = document.querySelector('.staff-grant-form');
        if (!form) return;

        var input = form.querySelector('#staff-search-input');
        var results = form.querySelector('#staff-search-results');
        var searchURL = form.dataset.searchUrl;
        if (!input || !results || !searchURL) return;

        var debounceTimer = null;
        var lastQuery = '';

        function hideResults() {
            results.hidden = true;
            results.innerHTML = '';
        }

        function renderResults(items) {
            if (!items || items.length === 0) {
                hideResults();
                return;
            }
            results.innerHTML = '';
            items.forEach(function (u) {
                var li = document.createElement('li');
                li.textContent = u.display_name;
                li.tabIndex = 0;
                li.addEventListener('mousedown', function (e) {
                    // mousedown beats blur — input keeps focus so we can fill it.
                    e.preventDefault();
                    input.value = u.display_name;
                    hideResults();
                });
                results.appendChild(li);
            });
            results.hidden = false;
        }

        function fetchResults(q) {
            if (q === lastQuery) return;
            lastQuery = q;
            fetch(searchURL + '?q=' + encodeURIComponent(q), {
                credentials: 'same-origin',
                headers: { 'Accept': 'application/json' },
            })
                .then(function (r) { return r.ok ? r.json() : []; })
                .then(renderResults)
                .catch(function () { hideResults(); });
        }

        input.addEventListener('input', function () {
            var q = input.value.trim();
            clearTimeout(debounceTimer);
            if (q.length < 1) {
                hideResults();
                return;
            }
            // Debounce so we don't fire on every keystroke.
            debounceTimer = setTimeout(function () { fetchResults(q); }, 200);
        });

        input.addEventListener('blur', function () {
            // Tiny delay so a mousedown on a result can fire first.
            setTimeout(hideResults, 100);
        });
    });
})();
