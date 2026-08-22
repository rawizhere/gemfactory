import { $, $$, el } from './api.js';
import { getArtistsList } from './artists.js';
import { getReleasesList, filterByArtist } from './releases.js';

let selectedIndex = 0;
let items = [];
let onSwitchTab = null;

export function setPaletteTabCallback(cb) {
  onSwitchTab = cb;
}

export function openPalette() {
  const dialog = $('#cmd-palette');
  const input = $('#palette-input');
  if (!dialog || !input) return;

  dialog.showModal();
  input.value = '';
  renderPaletteResults('');
  input.focus();
}

export function closePalette() {
  const dialog = $('#cmd-palette');
  if (dialog?.open) {
    dialog.close();
  }
}

function getAvailableActions() {
  return [
    { title: 'Go to Schedule', type: 'Navigation', action: () => onSwitchTab?.('schedule') },
    { title: 'Go to Releases', type: 'Navigation', action: () => onSwitchTab?.('releases') },
    { title: 'Go to Artists', type: 'Navigation', action: () => onSwitchTab?.('artists') },
    { title: 'Go to Cookies', type: 'Navigation', action: () => onSwitchTab?.('cookies') },
    { title: 'Go to Settings', type: 'Navigation', action: () => onSwitchTab?.('settings') },
    {
      title: 'Run Web Scraper',
      type: 'Action',
      action: () => {
        onSwitchTab?.('releases');
        $('#parse-month')?.focus();
      },
    },
    {
      title: 'Export Artists (CSV)',
      type: 'Export',
      action: () => $('#artist-export')?.click(),
    },
    {
      title: 'Export Releases (CSV)',
      type: 'Export',
      action: () => $('#rel-export')?.click(),
    },
    {
      title: 'Clean Downloads Storage',
      type: 'Storage',
      action: () => {
        onSwitchTab?.('settings');
        $('#clean-storage-btn')?.click();
      },
    },
  ];
}

function renderPaletteResults(query) {
  const container = $('#palette-results');
  if (!container) return;
  container.textContent = '';
  const q = query.trim().toLowerCase();

  const actions = getAvailableActions();
  const artists = getArtistsList();
  const releases = getReleasesList();

  items = [];

  // Match system actions
  actions.forEach((act) => {
    if (!q || act.title.toLowerCase().includes(q)) {
      items.push(act);
    }
  });

  // Match artists
  if (q) {
    artists.forEach((a) => {
      if (a.name.toLowerCase().includes(q)) {
        items.push({
          title: a.name,
          type: 'Artist',
          action: () => {
            onSwitchTab?.('releases');
            filterByArtist(a.name);
          },
        });
      }
    });

    // Match releases
    releases.slice(0, 100).forEach((r) => {
      if (`${r.artist} - ${r.title}`.toLowerCase().includes(q)) {
        items.push({
          title: `${r.artist} — ${r.title} (${r.date})`,
          type: 'Release',
          action: () => {
            onSwitchTab?.('releases');
            filterByArtist(r.artist);
          },
        });
      }
    });
  }

  // Cap results to 15 items for speed & responsiveness
  items = items.slice(0, 15);

  if (!items.length) {
    container.appendChild(el('div', { class: 'empty', style: 'padding:1.5rem 0' }, 'No matching commands or items.'));
    return;
  }

  if (selectedIndex >= items.length) selectedIndex = 0;

  items.forEach((item, idx) => {
    const itemEl = el('div', { class: `palette-item ${idx === selectedIndex ? 'selected' : ''}` });

    const left = el('span', null, item.title);
    const right = el('span', { class: 'item-type' }, item.type);

    itemEl.appendChild(left);
    itemEl.appendChild(right);

    itemEl.addEventListener('click', () => {
      closePalette();
      item.action();
    });

    itemEl.addEventListener('mouseenter', () => {
      selectedIndex = idx;
      updateSelection();
    });

    container.appendChild(itemEl);
  });
}

function updateSelection() {
  const resultEls = $$('#palette-results .palette-item');
  resultEls.forEach((el, idx) => {
    el.classList.toggle('selected', idx === selectedIndex);
    if (idx === selectedIndex) {
      el.scrollIntoView({ block: 'nearest' });
    }
  });
}

export function initPalette() {
  const input = $('#palette-input');
  const dialog = $('#cmd-palette');
  const openBtn = $('#open-palette');
  const closeBtn = $('#palette-close');

  openBtn?.addEventListener('click', openPalette);
  closeBtn?.addEventListener('click', closePalette);

  dialog?.addEventListener('click', (e) => {
    if (e.target === dialog) closePalette();
  });

  input?.addEventListener('input', () => {
    selectedIndex = 0;
    renderPaletteResults(input.value);
  });

  input?.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (items.length) {
        selectedIndex = (selectedIndex + 1) % items.length;
        updateSelection();
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (items.length) {
        selectedIndex = (selectedIndex - 1 + items.length) % items.length;
        updateSelection();
      }
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (items[selectedIndex]) {
        closePalette();
        items[selectedIndex].action();
      }
    } else if (e.key === 'Escape') {
      closePalette();
    }
  });

  // Global hotkey Cmd+K / Ctrl+K
  window.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      if (dialog?.open) {
        closePalette();
      } else {
        openPalette();
      }
    }
  });
}
