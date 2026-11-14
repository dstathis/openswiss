// Round timer countdown for the tournament detail page.
//
// The clock ticks locally once a second and polls the JSON endpoint every
// `data-poll-seconds` -- and immediately whenever the tab regains focus -- to
// pick up a judge starting, pausing, resuming or clearing it, so a page left
// open mid-round stays correct without a reload.
// This is the one place the app does live partial updates (see SPEC §11).
//
// Config comes from data-* attributes on the section, so this file stays
// parameterless and the strict CSP (script-src 'self', no unsafe-inline) holds:
// no inline script, no inline handlers.
//
// The device clock is not trusted. The endpoint returns `server_now` alongside
// `ends_at`, and the difference is kept as an offset -- a phone several minutes
// out of sync would otherwise show a countdown several minutes wrong.

(function () {
    var TICK_MS = 1000;
    var DEFAULT_POLL_SECONDS = 5;

    function pad(n) {
        return n < 10 ? '0' + n : String(n);
    }

    // format renders whole seconds as a clock. Negative input is overtime and
    // is rendered with a leading '+' -- a round past time keeps counting up,
    // because "how far over are we" is what staff need at that point.
    function format(seconds) {
        var overtime = seconds < 0;
        var total = Math.abs(seconds);
        var mins = Math.floor(total / 60);
        var secs = total % 60;
        return (overtime ? '+' : '') + mins + ':' + pad(secs);
    }

    document.addEventListener('DOMContentLoaded', function () {
        var section = document.querySelector('.round-timer');
        if (!section) return;

        var clock = section.querySelector('[data-timer-clock]');
        var label = section.querySelector('[data-timer-label]');
        var url = section.dataset.timerUrl;
        if (!clock || !url) return;

        var pollMs = (parseInt(section.dataset.pollSeconds, 10) || DEFAULT_POLL_SECONDS) * 1000;

        // state mirrors the last response we trust. skewMs is deviceNow -
        // serverNow, subtracted from the device clock to get server time.
        var state = {
            running: false,
            paused: false,
            endsAtMs: null,
            pausedAtMs: null,
            skewMs: 0
        };

        // Seed from what the server rendered, so the clock shows a real value
        // immediately rather than '--:--' until the first poll lands. Skew is
        // still zero here; the first poll corrects it.
        if (section.dataset.endsAt) {
            state.endsAtMs = Date.parse(section.dataset.endsAt);
            state.running = true;
            state.paused = section.dataset.state === 'paused';
            if (section.dataset.pausedAt) {
                state.pausedAtMs = Date.parse(section.dataset.pausedAt);
            }
        }

        function serverNow() {
            return Date.now() - state.skewMs;
        }

        function render() {
            if (!state.running || state.endsAtMs === null) {
                section.hidden = true;
                return;
            }
            section.hidden = false;

            // A paused clock measures to paused_at, so it holds its remainder.
            var reference = state.paused && state.pausedAtMs !== null
                ? state.pausedAtMs
                : serverNow();
            var remaining = Math.round((state.endsAtMs - reference) / 1000);

            clock.textContent = format(remaining);
            section.classList.toggle('is-overtime', remaining < 0);
            section.classList.toggle('is-paused', state.paused);
        }

        function apply(data) {
            // Trust the server's clock over the device's.
            var serverMs = Date.parse(data.server_now);
            if (!isNaN(serverMs)) state.skewMs = Date.now() - serverMs;

            if (data.state === 'running' || data.state === 'paused') {
                state.running = true;
                state.paused = data.state === 'paused';
                state.endsAtMs = Date.parse(data.ends_at);
                state.pausedAtMs = data.paused_at ? Date.parse(data.paused_at) : null;
                if (label && data.round_label) label.textContent = data.round_label;
            } else {
                // 'none': either cleared, or the round advanced and nobody has
                // started a clock for the new one. Either way, show nothing.
                state.running = false;
                state.endsAtMs = null;
                state.pausedAtMs = null;
            }
            render();
        }

        function poll() {
            // cache: 'no-store' is essential, not belt-and-braces. Every poll
            // hits the same URL, and with the default cache mode browsers
            // answer the repeats from their memory cache -- the clock then
            // freezes at whatever it was when the page loaded, so a started
            // timer never appears and a cleared one never goes away. The
            // endpoint also sends Cache-Control: no-store.
            fetch(url, {
                credentials: 'same-origin',
                cache: 'no-store',
                headers: { 'Accept': 'application/json' }
            })
                .then(function (res) { return res.ok ? res.json() : null; })
                .then(function (data) { if (data) apply(data); })
                .catch(function () { /* keep ticking on the last known state */ });
        }

        // Polling a hidden tab is wasted work, and browsers throttle timers in
        // background tabs anyway. Instead, poll the moment the tab comes back:
        // someone returning to a page they left mid-round sees the current
        // clock immediately rather than after the next interval.
        function pollIfVisible() {
            if (document.visibilityState !== 'hidden') poll();
        }

        document.addEventListener('visibilitychange', function () {
            if (document.visibilityState === 'visible') poll();
        });
        window.addEventListener('focus', poll);
        window.addEventListener('pageshow', poll);

        render();
        poll();
        setInterval(render, TICK_MS);
        setInterval(pollIfVisible, pollMs);
    });
})();
