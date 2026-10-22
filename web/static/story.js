// Polish for long-form articles. Everything works without it:
// scripts only reveal elements as they scroll in and keep link previews on screen.
(function () {
    'use strict';

    // Reveal on scroll.
    var targets = document.querySelectorAll('.scene__card, .stats li, .rent-embed, .story > h2');
    if ('IntersectionObserver' in window && targets.length) {
        var io = new IntersectionObserver(function (entries) {
            entries.forEach(function (entry) {
                if (entry.isIntersecting) {
                    entry.target.classList.add('is-in');
                    io.unobserve(entry.target);
                }
            });
        }, {rootMargin: '0px 0px -12% 0px', threshold: 0.12});
        targets.forEach(function (el) { io.observe(el); });
    } else {
        targets.forEach(function (el) { el.classList.add('is-in'); });
    }

    // Link previews: open upwards near the bottom of the screen and never leave it sideways.
    function place(hc) {
        var link = hc.querySelector('a');
        var card = hc.querySelector('.hc-card');
        if (!link || !card) return;
        var vw = document.documentElement.clientWidth;
        var vh = window.innerHeight;
        var width = Math.min(360, vw * 0.86);
        var rect = link.getBoundingClientRect();
        var center = rect.left + rect.width / 2;
        var left = Math.max(8, Math.min(center - width / 2, vw - width - 8));
        hc.style.setProperty('--hc-shift', (left - (center - width / 2)) + 'px');
        var room = vh - rect.bottom;
        hc.classList.toggle('hc--up', room < 190 && rect.top > room);
    }
    document.addEventListener('mouseover', function (e) {
        var hc = e.target.closest && e.target.closest('.hc');
        if (hc) place(hc);
    });
    document.addEventListener('focusin', function (e) {
        var hc = e.target.closest && e.target.closest('.hc');
        if (hc) place(hc);
    });
    document.addEventListener('keydown', function (e) {
        if (e.key === 'Escape' && document.activeElement && document.activeElement.closest('.hc')) {
            document.activeElement.blur();
        }
    });

    // The flash after "add to cart" must not come back on reload.
    var toast = document.getElementById('cart-toast');
    if (toast && window.history && history.replaceState) {
        var url = new URL(window.location.href);
        if (url.searchParams.has('cart_added')) {
            url.searchParams.delete('cart_added');
            history.replaceState({}, '', url.pathname + url.search + url.hash);
        }
        setTimeout(function () { if (toast.parentNode) toast.remove(); }, 9000);
    }
})();
