import { $, $$, el, tableWrap, api, showMsg, exportToCSV } from './api.js';

let releases = [];
let currentPage = 1;
let pageSize = 50;
let lastCheckedIndex = -1;
let onReleasesLoadedCallback = null;

export function setReleasesLoadedCallback(cb) {
  onReleasesLoadedCallback = cb;
}

export function getReleasesList() {
  return releases;
}

function releaseMatches(r, q) {
  return `${r.artist} ${r.title} ${r.date} ${r.title_track} ${r.mv} ${r.spotify}`
    .toLowerCase()
    .includes(q);
}

function getVisibleReleases() {
  const q = $('#rel-search').value.trim().toLowerCase();
  const artist = $('#rel-artist-input').value.trim().toLowerCase();
  const year = $('#rel-year').value;
  const month = $('#rel-month')?.value || '';
  const sort = $('#rel-sort').value;

  const rows = releases.filter((r) => {
    if (artist && (r.artist || '').toLowerCase().indexOf(artist) === -1) return false;
    if (year && (r.date || '').slice(0, 4) !== year) return false;
    if (month && (r.date || '').slice(5, 7) !== month) return false;
    if (q && !releaseMatches(r, q)) return false;
    return true;
  });

  if (sort === 'new') {
    rows.sort((a, b) => b.date.localeCompare(a.date) || b.id - a.id);
  } else if (sort === 'old') {
    rows.sort((a, b) => a.date.localeCompare(b.date) || a.id - b.id);
  }
  return rows;
}

function updateFilterDatalist() {
  const datalist = $('#rel-artist-list');
  if (!datalist) return;
  datalist.textContent = '';
  const allArtists = Array.from(new Set(releases.map((r) => r.artist).filter(Boolean))).sort();
  allArtists.forEach((name) => {
    datalist.appendChild(el('option', { value: name }));
  });

  const yearSelect = $('#rel-year');
  if (yearSelect) {
    const currentYear = new Date().getFullYear();
    const currentSelected = yearSelect.value;
    const yearSet = new Set(releases.map((r) => (r.date || '').slice(0, 4)).filter((y) => y && y.length === 4));
    yearSet.add(String(currentYear));
    yearSet.add(String(currentYear + 1));
    const allYears = Array.from(yearSet).sort((a, b) => b.localeCompare(a));

    yearSelect.textContent = '';
    yearSelect.appendChild(el('option', { value: '' }, 'All years'));
    allYears.forEach((y) => {
      yearSelect.appendChild(el('option', { value: y }, y));
    });
    if (allYears.includes(currentSelected)) {
      yearSelect.value = currentSelected;
    }
  }

  // Populate parser year select in clean descending order
  const parseYearSelect = $('#parse-year');
  if (parseYearSelect && parseYearSelect.children.length === 0) {
    const currentYear = new Date().getFullYear();
    const parserYears = [currentYear + 1, currentYear, currentYear - 1, currentYear - 2];
    parserYears.forEach((y) => {
      parseYearSelect.appendChild(el('option', { value: String(y) }, String(y)));
    });
    parseYearSelect.value = String(currentYear);
  }
}

function renderPager(totalRows) {
  const pager = $('#releases-pager');
  if (!pager) return;
  pager.textContent = '';

  if (pageSize === 0 || totalRows <= pageSize) {
    return;
  }

  const totalPages = Math.ceil(totalRows / pageSize);
  if (currentPage > totalPages) currentPage = totalPages;
  if (currentPage < 1) currentPage = 1;

  const info = el('span', null, `Page ${currentPage} of ${totalPages} (${totalRows} items)`);

  const controls = el('div', { class: 'pager-controls' });

  const prevBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, '← Prev');
  prevBtn.disabled = currentPage <= 1;
  prevBtn.addEventListener('click', () => {
    if (currentPage > 1) {
      currentPage--;
      renderReleases();
    }
  });
  controls.appendChild(prevBtn);

  const nextBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Next →');
  nextBtn.disabled = currentPage >= totalPages;
  nextBtn.addEventListener('click', () => {
    if (currentPage < totalPages) {
      currentPage++;
      renderReleases();
    }
  });
  controls.appendChild(nextBtn);

  const sizeSelect = el('select', { style: 'padding:0.18rem 0.4rem;font-size:0.78rem' });
  [50, 100, 200, 0].forEach((sz) => {
    sizeSelect.appendChild(el('option', { value: String(sz) }, sz === 0 ? 'All' : `${sz} / page`));
  });
  sizeSelect.value = String(pageSize);
  sizeSelect.addEventListener('change', () => {
    pageSize = Number(sizeSelect.value);
    currentPage = 1;
    renderReleases();
  });
  controls.appendChild(sizeSelect);

  pager.appendChild(info);
  pager.appendChild(controls);
}

function updateRelBar() {
  const checked = $$('#releases-table input[type=checkbox]:checked').length;
  $('#rel-delete').disabled = checked === 0;

  const searchVal = $('#rel-search').value;
  const artistVal = $('#rel-artist-input').value;
  const yearVal = $('#rel-year').value;
  const monthVal = $('#rel-month')?.value || '';
  const sortVal = $('#rel-sort').value;

  $('#rel-search-clear').hidden = !searchVal;
  $('#rel-artist-clear').hidden = !artistVal;

  const hasActiveFilters = Boolean(searchVal || artistVal || yearVal || monthVal || sortVal);
  const resetBtn = $('#rel-reset');
  if (resetBtn) {
    resetBtn.classList.toggle('active-filter', hasActiveFilters);
  }
}

export function renderReleases() {
  const box = $('#releases-table');
  box.textContent = '';

  const allVisible = getVisibleReleases();
  $('#rel-count').textContent = `${allVisible.length} of ${releases.length}`;

  if (!allVisible.length) {
    box.appendChild(el('p', { class: 'empty' }, releases.length ? 'No releases match your filter.' : 'No releases yet.'));
    renderPager(0);
    updateRelBar();
    return;
  }

  // Slice for pagination
  const pagedRows = pageSize === 0 ? allVisible : allVisible.slice((currentPage - 1) * pageSize, currentPage * pageSize);

  const table = el('table');
  const thead = el('thead');
  const headRow = el('tr');
  ['', 'Artist', 'Title', 'Date', 'Title Track', 'Links', ''].forEach((title, idx) => {
    headRow.appendChild(el('th', { class: idx === 0 ? 'num' : '' }, title));
  });
  thead.appendChild(headRow);
  table.appendChild(thead);

  const tbody = el('tbody');
  pagedRows.forEach((r, rowIndex) => {
    const tr = el('tr');

    // Checkbox with Shift-click
    const tdChk = el('td', { class: 'num' });
    const chk = el('input', { type: 'checkbox', value: String(r.id), 'aria-label': r.title });
    chk.dataset.index = String(rowIndex);
    chk.addEventListener('click', (e) => {
      if (e.shiftKey && lastCheckedIndex !== -1) {
        const checkboxes = Array.from($$('#releases-table input[type=checkbox]'));
        const start = Math.min(lastCheckedIndex, rowIndex);
        const end = Math.max(lastCheckedIndex, rowIndex);
        checkboxes.slice(start, end + 1).forEach((c) => {
          c.checked = chk.checked;
        });
      }
      lastCheckedIndex = rowIndex;
      updateRelBar();
    });
    tdChk.appendChild(chk);
    tr.appendChild(tdChk);

    // Artist name (clickable)
    const tdArtist = el('td');
    const artistLink = el('a', { class: 'clickable-link', title: 'Filter by this artist' }, r.artist);
    artistLink.addEventListener('click', (e) => {
      e.preventDefault();
      filterByArtist(r.artist);
    });
    tdArtist.appendChild(artistLink);
    tr.appendChild(tdArtist);

    // Title
    tr.appendChild(el('td', { class: 'title-cell' }, r.title));

    // Date
    tr.appendChild(el('td', { class: 'date' }, r.date));

    // Title Track
    const tdTrack = el('td');
    if (r.title_track) {
      tdTrack.appendChild(el('span', { style: 'color:var(--muted)' }, r.title_track));
    } else {
      tdTrack.appendChild(el('span', { style: 'color:var(--muted)' }, '—'));
    }
    tr.appendChild(tdTrack);

    // Links
    const tdLinks = el('td');
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

    // Actions
    const tdDel = el('td', { style: 'text-align:right' });
    const delBtn = el('button', { class: 'btn btn-danger btn-sm', type: 'button' }, 'Delete');
    delBtn.addEventListener('click', async () => {
      if (!confirm(`Delete release "${r.title}" (${r.artist})?`)) return;
      try {
        delBtn.disabled = true;
        await api('DELETE', `/api/releases/${r.id}`);
        releases = releases.filter((x) => x.id !== r.id);
        renderReleases();
        await loadReleases();
      } catch (err) {
        alert(`Delete failed: ${err.message}`);
        await loadReleases();
      }
    });
    tdDel.appendChild(delBtn);
    tr.appendChild(tdDel);

    tbody.appendChild(tr);
  });

  table.appendChild(tbody);
  box.appendChild(tableWrap(table));
  renderPager(allVisible.length);
  updateRelBar();
}

export async function loadReleases() {
  try {
    releases = await api('GET', '/api/releases');
    updateFilterDatalist();

    // Compute release counts per artist
    const counts = {};
    releases.forEach((r) => {
      if (r.artist) counts[r.artist] = (counts[r.artist] || 0) + 1;
    });
    if (onReleasesLoadedCallback) onReleasesLoadedCallback(counts);

    renderReleases();
  } catch (err) {
    console.error('Failed to load releases:', err);
  }
}

export function filterByArtist(artistName) {
  const artistInput = $('#rel-artist-input');
  if (artistInput) {
    artistInput.value = artistName;
    $('#rel-search').value = '';
    currentPage = 1;
    renderReleases();
  }
}

export function resetFilters() {
  $('#rel-search').value = '';
  $('#rel-artist-input').value = '';
  $('#rel-year').value = '';
  if ($('#rel-month')) $('#rel-month').value = '';
  $('#rel-sort').value = '';
  currentPage = 1;
  renderReleases();
}

export function initReleases() {
  $('#rel-search').addEventListener('input', () => {
    currentPage = 1;
    renderReleases();
  });
  $('#rel-artist-input').addEventListener('input', () => {
    currentPage = 1;
    renderReleases();
  });
  $('#rel-year').addEventListener('change', () => {
    currentPage = 1;
    renderReleases();
  });
  $('#rel-month')?.addEventListener('change', () => {
    currentPage = 1;
    renderReleases();
  });
  $('#rel-sort').addEventListener('change', () => {
    currentPage = 1;
    renderReleases();
  });

  $('#rel-search-clear').addEventListener('click', () => {
    $('#rel-search').value = '';
    currentPage = 1;
    renderReleases();
    $('#rel-search').focus();
  });

  $('#rel-artist-clear').addEventListener('click', () => {
    $('#rel-artist-input').value = '';
    currentPage = 1;
    renderReleases();
    $('#rel-artist-input').focus();
  });

  $('#rel-reset').addEventListener('click', resetFilters);

  $('#rel-check-all').addEventListener('change', function () {
    const isChecked = this.checked;
    $$('#releases-table input[type=checkbox]').forEach((cb) => {
      cb.checked = isChecked;
    });
    updateRelBar();
  });

  $('#releases-table').addEventListener('change', (ev) => {
    if (ev.target.type === 'checkbox') updateRelBar();
  });

  // Parser form submission
  $('#parser-form')?.addEventListener('submit', async (e) => {
    e.preventDefault();
    const btn = $('#parse-btn');
    const msg = $('#parse-msg');
    const month = $('#parse-month').value;
    const year = Number($('#parse-year').value);

    try {
      btn.disabled = true;
      btn.textContent = 'Parsing...';
      const label = month === 'all' ? `all 12 months of ${year}` : `${month} ${year}`;
      showMsg(msg, `Scraping releases for ${label}...`, true, 0);

      const res = await api('POST', '/api/parse', { month, year });
      showMsg(msg, `Scraping complete. Found ${res.found} releases for ${label}.`, true, 6000);
      await loadReleases();
    } catch (err) {
      showMsg(msg, `Scraper error: ${err.message}`, false, 6000);
    } finally {
      btn.disabled = false;
      btn.textContent = 'Run Scraper';
    }
  });

  // Export CSV
  $('#rel-export')?.addEventListener('click', () => {
    const visible = getVisibleReleases();
    exportToCSV('releases.csv', visible, {
      artist: 'Artist',
      title: 'Title',
      date: 'Date',
      title_track: 'Title Track',
      mv: 'MV Link',
      spotify: 'Spotify Link',
    });
  });

  $('#rel-delete').addEventListener('click', async () => {
    const ids = Array.from($$('#releases-table input[type=checkbox]:checked')).map((cb) => Number(cb.value));
    if (!ids.length) return;
    if (!confirm(`Delete ${ids.length} selected releases?`)) return;

    try {
      const idsToDelete = new Set(ids);
      await api('POST', '/api/releases/delete', { ids });
      releases = releases.filter((x) => !idsToDelete.has(x.id));
      $('#rel-check-all').checked = false;
      renderReleases();
      await loadReleases();
    } catch (err) {
      alert(`Bulk delete failed: ${err.message}`);
      await loadReleases();
    }
  });
}
