// Installing the site as an app (Android/desktop Chrome prompt, iOS "Add to Home Screen" hint)
// and web push subscriptions for signed-in users and admins.
(function () {
  'use strict';

  var DISMISS_KEY = 'pwa-install-dismissed';
  var DISMISS_DAYS = 30;

  function storageGet(key) { try { return window.localStorage.getItem(key); } catch (e) { return null; } }
  function storageSet(key, value) { try { window.localStorage.setItem(key, value); } catch (e) { /* private mode */ } }

  var isStandalone = window.matchMedia('(display-mode: standalone)').matches || window.navigator.standalone === true;
  var ua = window.navigator.userAgent;
  var isIOS = /iPad|iPhone|iPod/.test(ua) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
  var deferredPrompt = null;

  /* ---------- service worker ---------- */

  var swReady = null;
  if ('serviceWorker' in navigator) {
    swReady = navigator.serviceWorker.register('/sw.js', { scope: '/' }).then(function () {
      return navigator.serviceWorker.ready;
    }).catch(function (err) {
      console.warn('Service worker registration failed', err);
      return null;
    });
  }

  /* ---------- install ---------- */

  function installButtons() { return document.querySelectorAll('[data-install-app]'); }
  function banner() { return document.getElementById('install-banner'); }

  function dismissedRecently() {
    var at = parseInt(storageGet(DISMISS_KEY) || '0', 10);
    return at && Date.now() - at < DISMISS_DAYS * 86400000;
  }

  function canInstall() {
    return !isStandalone && (deferredPrompt !== null || isIOS);
  }

  function refreshInstallUI() {
    var show = canInstall();
    installButtons().forEach(function (b) { b.classList.toggle('hidden', !show); });
    var b = banner();
    if (b) b.classList.toggle('hidden', !(show && !dismissedRecently()));
  }

  function showIOSHelp() {
    var help = document.getElementById('install-ios-help');
    if (help) help.classList.remove('hidden');
  }

  function install() {
    if (deferredPrompt) {
      var promptEvent = deferredPrompt;
      deferredPrompt = null;
      promptEvent.prompt();
      promptEvent.userChoice.finally(refreshInstallUI);
      return;
    }
    if (isIOS) showIOSHelp();
  }

  window.addEventListener('beforeinstallprompt', function (event) {
    event.preventDefault();
    deferredPrompt = event;
    refreshInstallUI();
  });

  window.addEventListener('appinstalled', function () {
    deferredPrompt = null;
    isStandalone = true;
    refreshInstallUI();
    refreshPushUI();
  });

  document.addEventListener('click', function (event) {
    var target = event.target.closest('[data-install-app]');
    if (target) { event.preventDefault(); install(); return; }
    if (event.target.closest('[data-install-dismiss]')) {
      storageSet(DISMISS_KEY, String(Date.now()));
      refreshInstallUI();
    }
    if (event.target.closest('[data-install-ios-close]')) {
      var help = document.getElementById('install-ios-help');
      if (help) help.classList.add('hidden');
    }
  });

  /* ---------- web push ---------- */

  var pushSupported = 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;

  function urlBase64ToUint8Array(base64) {
    var padding = '='.repeat((4 - (base64.length % 4)) % 4);
    var raw = atob((base64 + padding).replace(/-/g, '+').replace(/_/g, '/'));
    var out = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
    return out;
  }

  function postJSON(url, body) {
    return fetch(url, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
  }

  function currentSubscription() {
    if (!pushSupported || !swReady) return Promise.resolve(null);
    return swReady.then(function (reg) { return reg ? reg.pushManager.getSubscription() : null; });
  }

  // Browsers that cannot reach their push service (offline, blocked network) never settle subscribe().
  function withTimeout(promise, ms, message) {
    return Promise.race([
      promise,
      new Promise(function (_, reject) { setTimeout(function () { reject(new Error(message)); }, ms); }),
    ]);
  }

  function subscribe() {
    return withTimeout(subscribeToPushService(), 20000, 'Nie udało się połączyć z usługą powiadomień. Spróbuj ponownie.');
  }

  function subscribeToPushService() {
    return swReady.then(function (reg) {
      if (!reg) throw new Error('Service worker unavailable');
      return fetch('/api/webpush/vapid-key', { credentials: 'same-origin' })
        .then(function (res) {
          if (!res.ok) throw new Error('Powiadomienia nie są skonfigurowane na serwerze');
          return res.json();
        })
        .then(function (key) {
          return reg.pushManager.subscribe({
            userVisibleOnly: true,
            applicationServerKey: urlBase64ToUint8Array(key.publicKey),
          });
        });
    }).then(function (sub) {
      return postJSON('/api/webpush/subscribe', sub.toJSON()).then(function (res) {
        if (!res.ok) throw new Error('Nie udało się zapisać subskrypcji');
        return sub;
      });
    });
  }

  function unsubscribe() {
    return currentSubscription().then(function (sub) {
      if (!sub) return null;
      return postJSON('/api/webpush/unsubscribe', { endpoint: sub.endpoint })
        .then(function () { return sub.unsubscribe(); });
    });
  }

  function pushBoxes() { return document.querySelectorAll('[data-push-box]'); }

  function renderPush(state, message) {
    pushBoxes().forEach(function (box) {
      var btn = box.querySelector('[data-push-toggle]');
      var status = box.querySelector('[data-push-status]');
      box.classList.remove('hidden');
      box.dataset.pushState = state;
      if (status) status.textContent = message || '';
      if (!btn) return;
      btn.disabled = state === 'unsupported' || state === 'denied' || state === 'busy' || state === 'needs-install';
      btn.classList.toggle('hidden', state === 'unsupported' || state === 'needs-install');
      btn.textContent = state === 'on' ? 'Wyłącz powiadomienia' : 'Włącz powiadomienia';
    });
  }

  function refreshPushUI() {
    // Only signed-in users get pushes; every page re-binds the device to the current account.
    if (document.body.dataset.loggedIn !== '1') return Promise.resolve();
    if (!pushSupported) {
      if (isIOS && !isStandalone) {
        renderPush('needs-install', 'Na iPhonie i iPadzie powiadomienia działają po dodaniu strony do ekranu początkowego (Udostępnij → Do ekranu początkowego), a potem włączeniu ich z poziomu aplikacji.');
      } else {
        renderPush('unsupported', 'Ta przeglądarka nie obsługuje powiadomień push.');
      }
      return Promise.resolve();
    }
    if (Notification.permission === 'denied') {
      renderPush('denied', 'Powiadomienia są zablokowane w ustawieniach przeglądarki dla tej strony.');
      return Promise.resolve();
    }
    return currentSubscription().then(function (sub) {
      if (sub && Notification.permission === 'granted') {
        renderPush('on', 'Powiadomienia są włączone na tym urządzeniu.');
        // Bind the browser subscription to whoever is signed in now (also after switching accounts).
        postJSON('/api/webpush/subscribe', sub.toJSON()).catch(function () {});
      } else {
        renderPush('off', (document.querySelector('[data-push-box]') || {dataset: {}}).dataset.hint || '');
      }
    });
  }

  document.addEventListener('click', function (event) {
    var btn = event.target.closest('[data-push-toggle]');
    if (!btn) return;
    event.preventDefault();
    var box = btn.closest('[data-push-box]');
    var enabling = !box || box.dataset.pushState !== 'on';
    renderPush('busy', enabling ? 'Włączanie…' : 'Wyłączanie…');
    var work = enabling
      ? Notification.requestPermission().then(function (perm) {
          if (perm !== 'granted') throw new Error('permission-' + perm);
          return subscribe();
        })
      : unsubscribe();
    work.then(refreshPushUI).catch(function (err) {
      if (err && /^permission-/.test(err.message)) {
        refreshPushUI();
      } else {
        renderPush('off', (err && err.message) || 'Coś poszło nie tak. Spróbuj ponownie.');
      }
    });
  });

  // Signing out must stop pushes meant for that account from reaching this device.
  document.addEventListener('submit', function (event) {
    var form = event.target;
    if (!form || !form.getAttribute || form.getAttribute('action') !== '/logout' || form.dataset.pushDone) return;
    if (!pushSupported || !swReady) return;
    event.preventDefault();
    var finish = function () { form.dataset.pushDone = '1'; form.submit(); };
    currentSubscription().then(function (sub) {
      if (!sub) return;
      // Only the server-side link to this account is removed; the browser subscription stays,
      // so the next sign-in on this device re-binds it without asking for permission again.
      return postJSON('/api/webpush/unsubscribe', { endpoint: sub.endpoint });
    }).then(finish, finish);
  });

  /* ---------- init ---------- */

  function init() {
    refreshInstallUI();
    refreshPushUI();
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();
})();
