import { $, $$, el, api } from './api.js';
import { initArtists, loadArtists, setArtistSelectCallback, setReleaseCounts } from './artists.js';
import { initReleases, loadReleases, filterByArtist, setReleasesLoadedCallback } from './releases.js';
import { initSchedule, renderSchedule } from './schedule.js';
import { initCookies, loadCookies } from './cookies.js';
import { initSettings, loadConfig, openSection } from './settings.js';
import { initPalette, setPaletteTabCallback } from './palette.js';

export function switchTab(tabName) {
  $$('nav[role=tablist] button').forEach((b) => {
    b.setAttribute('aria-selected', b.dataset.tab === tabName ? 'true' : 'false');
  });

  $$('main section[role=tabpanel]').forEach((s) => {
    s.classList.toggle('active', s.id === `tab-${tabName}`);
  });

  if (tabName === 'schedule') renderSchedule();
  if (tabName === 'cookies') loadCookies();
  if (tabName === 'settings') loadConfig();
  if (tabName === 'artists') loadArtists();
  // loadReleases refreshes stats through its loaded callback.
  if (tabName === 'releases') loadReleases();
  else updateStats();
}

// Gemini rejecting the key takes a network round trip, so it is checked once per page load.
let keyRejected = false;

async function checkGeminiKey() {
  try {
    keyRejected = !!(await api('GET', '/api/translation')).key_rejected;
  } catch (_) {
    keyRejected = false;
  }
}

function renderWarnings(warnings) {
  const box = $('#stats-warnings');
  const dot = $('#system-stats .status-dot');
  if (!box) return;
  box.textContent = '';
  const all = keyRejected ? [...warnings, { text: 'Gemini rejects the API key', tab: 'settings/translation' }] : warnings;
  all.forEach((w) => {
    const [tab, section] = w.tab.split('/');
    const b = el('button', { type: 'button', class: 'warn-pill', title: `Open ${w.tab}` }, w.text);
    b.addEventListener('click', () => {
      if (section) openSection(section);
      switchTab(tab);
    });
    box.appendChild(b);
  });
  dot?.classList.toggle('warn', all.length > 0);
}

async function updateStats() {
  const statsEl = $('#stats-summary');
  if (!statsEl) return;
  try {
    const stats = await api('GET', '/api/stats');
    if (stats) {
      statsEl.textContent = `${stats.artists} artists (${stats.active_artists} active) · ${stats.releases} releases · ${stats.storage_formatted || '0 B'} stored`;
      renderWarnings(stats.warnings || []);
    }
  } catch (err) {
    statsEl.textContent = 'Server unreachable';
    $('#system-stats .status-dot')?.classList.add('warn');
  }
}

function initNavigation() {
  $$('nav[role=tablist] button').forEach((btn) => {
    btn.addEventListener('click', () => switchTab(btn.dataset.tab));
  });

  // Cross-tab navigation from Artists -> Releases
  setArtistSelectCallback((artistName) => {
    switchTab('releases');
    filterByArtist(artistName);
  });

  // Share release counts with artists tab and schedule
  setReleasesLoadedCallback((counts) => {
    setReleaseCounts(counts);
    renderSchedule();
    updateStats();
  });

  // Palette tab switch
  setPaletteTabCallback((tabName) => {
    switchTab(tabName);
  });
}

function initHotkeys() {
  window.addEventListener('keydown', (e) => {
    const activeEl = document.activeElement;
    const isEditing =
      activeEl &&
      (activeEl.tagName === 'INPUT' || activeEl.tagName === 'TEXTAREA' || activeEl.isContentEditable);

    if (e.key === 'Escape') {
      if (isEditing) {
        activeEl.blur();
      }
      return;
    }

    if (isEditing) return;

    // Focus search on `/`
    if (e.key === '/') {
      const activePanel = $('section[role=tabpanel].active');
      const searchInput = activePanel?.querySelector('input[type=text]');
      if (searchInput) {
        e.preventDefault();
        searchInput.focus();
        searchInput.select();
      }
      return;
    }

    // Number keys 1-5 to switch tabs
    const tabs = ['schedule', 'releases', 'artists', 'cookies', 'settings'];
    const num = parseInt(e.key, 10);
    if (num >= 1 && num <= tabs.length) {
      e.preventDefault();
      switchTab(tabs[num - 1]);
    }
  });
}

document.addEventListener('DOMContentLoaded', async () => {
  initNavigation();
  initHotkeys();
  initPalette();
  initArtists();
  initReleases();
  initSchedule();
  initCookies();
  initSettings();

  // Schedule needs artists for gender badges and the gender filter.
  await Promise.all([loadReleases(), loadArtists()]);
  renderSchedule();
  await checkGeminiKey();
  updateStats();
});
