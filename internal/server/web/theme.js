(() => {
  'use strict';

  // Read old preferences once without overwriting an explicit Kino choice.
  for (const suffix of ['ui-theme', 'ui-locale']) {
    try {
      const key = `kino-${suffix}`, previous = localStorage.getItem(`varkiv-${suffix}`);
      if (localStorage.getItem(key) === null && previous !== null) localStorage.setItem(key, previous);
    } catch (_) { /* Storage can be unavailable in private/restricted contexts. */ }
  }
  const storageKey = 'kino-ui-theme';
  const modes = new Set(['system', 'light', 'dark']);
  const media = window.matchMedia('(prefers-color-scheme: dark)');
  const root = document.documentElement;
  let mode = modes.has(localStorage.getItem(storageKey)) ? localStorage.getItem(storageKey) : 'system';

  function resolvedTheme() {
    return mode === 'system' ? (media.matches ? 'dark' : 'light') : mode;
  }

  function updateControls() {
    document.querySelectorAll('.theme-switch [data-theme-mode]').forEach(button => {
      const selected = button.dataset.themeMode === mode;
      button.classList.toggle('active', selected);
      button.setAttribute('aria-pressed', String(selected));
    });
  }

  function apply(nextMode = mode, persist = false) {
    mode = modes.has(nextMode) ? nextMode : 'system';
    const resolved = resolvedTheme();
    root.dataset.themeMode = mode;
    root.dataset.theme = resolved;
    root.style.colorScheme = resolved;
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', resolved === 'dark' ? '#0b0c10' : '#f5f6f8');
    updateControls();
    if (persist) localStorage.setItem(storageKey, mode);
    document.dispatchEvent(new CustomEvent('kino:theme-change', { detail: { mode, resolved } }));
  }

  function bindControls() {
    document.querySelectorAll('.theme-switch [data-theme-mode]').forEach(button => {
      button.addEventListener('click', () => apply(button.dataset.themeMode, true));
    });
    updateControls();
  }

  media.addEventListener?.('change', () => {
    if (mode === 'system') apply('system');
  });
  apply(mode);
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', bindControls, { once: true });
  else bindControls();

  window.kinoTheme = { apply, mode: () => mode, resolved: resolvedTheme };
})();
