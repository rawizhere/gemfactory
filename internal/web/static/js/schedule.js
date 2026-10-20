import { $, $$, el, tableWrap } from './api.js';
import { getReleasesList } from './releases.js';
import { getArtistsList } from './artists.js';

let currentYear = new Date().getFullYear();
let currentMonth = new Date().getMonth() + 1; // 1-12

const pad2 = (n) => String(n).padStart(2, '0');

function monthLabel() {
  return new Date(currentYear, currentMonth - 1, 1).toLocaleDateString('en-US', { month: 'long', year: 'numeric' });
}

function getRelativeDateBadge(dateStr) {
  const today = new Date();
  today.setHours(0, 0, 0, 0);

  const [y, m, d] = dateStr.split('-').map(Number);
  const targetDate = new Date(y, m - 1, d);
  targetDate.setHours(0, 0, 0, 0);

  const diffDays = Math.round((targetDate - today) / (1000 * 60 * 60 * 24));

  if (diffDays === 0) return el('span', { class: 'rel-date today' }, 'today');
  if (diffDays === 1) return el('span', { class: 'rel-date soon' }, 'tomorrow');
  if (diffDays === -1) return el('span', { class: 'rel-date' }, 'yesterday');
  if (diffDays > 1) return el('span', { class: 'rel-date' }, `in ${diffDays}d`);
  return el('span', { class: 'rel-date' }, `${Math.abs(diffDays)}d ago`);
}

function getDayOfWeek(dateStr) {
  const [y, m, d] = dateStr.split('-').map(Number);
  return new Date(y, m - 1, d).toLocaleDateString('en-US', { weekday: 'long' });
}

export function renderSchedule() {
  const box = $('#schedule-table');
  const monthTitle = $('#sched-current-month');
  const countEl = $('#sched-count');
  if (!box) return;

  box.textContent = '';
  monthTitle.textContent = monthLabel();

  const monthPrefix = `${currentYear}-${pad2(currentMonth)}`;
  const releases = getReleasesList();
  const artists = getArtistsList();
  const artistMap = new Map(artists.map((a) => [a.name.toLowerCase(), a]));

  const genderFilter = $('#sched-gender')?.value || '';
  const searchFilter = $('#sched-search')?.value.trim().toLowerCase() || '';

  // Filter releases for this month
  const monthReleases = releases.filter((r) => {
    if (!r.date || !r.date.startsWith(monthPrefix)) return false;

    if (genderFilter) {
      const art = artistMap.get((r.artist || '').toLowerCase());
      if (art && art.gender !== genderFilter) return false;
    }

    if (searchFilter) {
      const haystack = `${r.artist} ${r.title} ${r.title_track}`.toLowerCase();
      if (!haystack.includes(searchFilter)) return false;
    }

    return true;
  });

  // Sort chronologically (oldest date first for schedule feed)
  monthReleases.sort((a, b) => a.date.localeCompare(b.date) || a.id - b.id);

  countEl.textContent = `${monthReleases.length} releases`;

  if (!monthReleases.length) {
    box.appendChild(
      el('p', { class: 'empty' }, `No releases scheduled for ${monthLabel()}.`)
    );
    return;
  }

  const todayStr = `${new Date().getFullYear()}-${pad2(new Date().getMonth() + 1)}-${pad2(new Date().getDate())}`;

  const table = el('table');
  const thead = el('thead');
  const headRow = el('tr');
  ['Date', 'Day', 'Artist', 'Title', 'Title Track', 'Links'].forEach((col) => {
    headRow.appendChild(el('th', null, col));
  });
  thead.appendChild(headRow);
  table.appendChild(thead);

  const tbody = el('tbody');

  monthReleases.forEach((r) => {
    const isToday = r.date === todayStr;
    const tr = el('tr', { class: isToday ? 'is-today' : r.date < todayStr ? 'is-past' : '' });

    // 1. Date + Relative tag
    const tdDate = el('td', { class: 'date', style: 'white-space:nowrap' });
    tdDate.appendChild(document.createTextNode(r.date));
    tdDate.appendChild(getRelativeDateBadge(r.date));
    tr.appendChild(tdDate);

    // 2. Day of week
    const tdDay = el('td', { style: 'color:var(--muted);white-space:nowrap' }, getDayOfWeek(r.date));
    tr.appendChild(tdDay);

    // 3. Artist name with gender badge
    const tdArtist = el('td');
    tdArtist.appendChild(el('span', { class: 'artist-name' }, r.artist));

    const artObj = artistMap.get((r.artist || '').toLowerCase());
    if (artObj?.gender) {
      const gBadge = el('span', { class: `badge ${artObj.gender}`, style: 'margin-left:0.45rem' }, artObj.gender);
      tdArtist.appendChild(gBadge);
    }
    tr.appendChild(tdArtist);

    // 4. Title
    const tdTitle = el('td', { class: 'title-cell' }, r.title);
    tr.appendChild(tdTitle);

    // 5. Title Track
    const tdTrack = el('td');
    if (r.title_track && r.title_track !== r.title) {
      tdTrack.appendChild(el('span', { style: 'color:var(--ink)' }, r.title_track));
    } else {
      tdTrack.appendChild(el('span', { style: 'color:var(--muted)' }, '—'));
    }
    tr.appendChild(tdTrack);

    // 6. Links
    const tdLinks = el('td', { style: 'white-space:nowrap' });
    let hasLinks = false;
    if (r.mv) {
      tdLinks.appendChild(el('a', { href: r.mv, target: '_blank', rel: 'noopener' }, 'MV'));
      hasLinks = true;
    }
    if (r.spotify) {
      if (hasLinks) tdLinks.appendChild(document.createTextNode(' · '));
      tdLinks.appendChild(el('a', { href: r.spotify, target: '_blank', rel: 'noopener' }, 'Spotify'));
      hasLinks = true;
    }
    if (!hasLinks) {
      tdLinks.appendChild(el('span', { style: 'color:var(--muted)' }, '—'));
    }
    tr.appendChild(tdLinks);

    tbody.appendChild(tr);
  });

  table.appendChild(tbody);
  const wrap = tableWrap(table);
  box.appendChild(wrap);
  scrollToToday(wrap);
}

// scrollToToday scrolls the table so today's (or the next upcoming) release sits near the top,
// keeping one earlier row in view for context.
function scrollToToday(wrap) {
  const rows = [...wrap.querySelectorAll('tbody tr')];
  const idx = rows.findIndex((tr) => !tr.classList.contains('is-past'));
  if (idx <= 0) return;
  const anchor = rows[idx - 1];
  const headHeight = wrap.querySelector('thead')?.offsetHeight || 0;
  wrap.scrollTop += anchor.getBoundingClientRect().top - wrap.getBoundingClientRect().top - headHeight;
}

export function initSchedule() {
  $('#sched-prev-month')?.addEventListener('click', () => {
    currentMonth--;
    if (currentMonth < 1) {
      currentMonth = 12;
      currentYear--;
    }
    renderSchedule();
  });

  $('#sched-next-month')?.addEventListener('click', () => {
    currentMonth++;
    if (currentMonth > 12) {
      currentMonth = 1;
      currentYear++;
    }
    renderSchedule();
  });

  $('#sched-today-btn')?.addEventListener('click', () => {
    currentYear = new Date().getFullYear();
    currentMonth = new Date().getMonth() + 1;
    renderSchedule();
  });

  $('#sched-gender')?.addEventListener('change', renderSchedule);

  const searchInput = $('#sched-search');
  const clearBtn = $('#sched-search-clear');

  searchInput?.addEventListener('input', () => {
    if (clearBtn) clearBtn.hidden = !searchInput.value;
    renderSchedule();
  });

  clearBtn?.addEventListener('click', () => {
    searchInput.value = '';
    clearBtn.hidden = true;
    renderSchedule();
    searchInput.focus();
  });
}
