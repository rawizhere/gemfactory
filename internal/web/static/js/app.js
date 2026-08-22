import { $, $$, api } from './api.js';
import { initArtists, loadArtists, setArtistSelectCallback, setReleaseCounts } from './artists.js';
import { initReleases, loadReleases, filterByArtist, setReleasesLoadedCallback } from './releases.js';
import { initSchedule, renderSchedule } from './schedule.js';
import { initCookies, loadCookies } from './cookies.js';
import { initSettings, loadConfig } from './settings.js';
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
  if (tabName === 'releases') loadReleases();
  updateStats();
}

async function updateStats() {
  const statsEl = $('#stats-summary');
  if (!statsEl) return;
  try {
    const stats = await api('GET', '/api/stats');
    if (stats) {
      statsEl.textContent = `Online · ${stats.artists} artists (${stats.active_artists} active) · ${stats.releases} releases · Storage: ${stats.storage_formatted || '0 B'}`;
    }
  } catch (err) {
    statsEl.textContent = 'Server Online';
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

  await loadReleases();
  renderSchedule();
  loadArtists();
  updateStats();
});
