import { $, $$, el, tableWrap, api, showMsg, exportToCSV } from './api.js';

let artists = [];
let releaseCounts = {};
let onSelectArtistCallback = null;
let lastCheckedIndex = -1;

export function setArtistSelectCallback(cb) {
  onSelectArtistCallback = cb;
}

export function setReleaseCounts(counts) {
  releaseCounts = counts || {};
  renderArtists();
}

export function getArtistsList() {
  return artists;
}

export function parseArtistNames(raw) {
  if (!raw) return [];
  const items = raw.split(/[\n,;\t]+/);
  const seen = new Set();
  const result = [];
  for (const item of items) {
    const trimmed = item.trim();
    if (trimmed && !seen.has(trimmed.toLowerCase())) {
      seen.add(trimmed.toLowerCase());
      result.push(trimmed);
    }
  }
  return result;
}

export function genderBadge(gender, onClick) {
  const b = el('span', { class: `badge ${gender}`, title: 'Click to change gender', style: 'cursor:pointer' }, gender);
  if (onClick) {
    b.addEventListener('click', (e) => {
      e.stopPropagation();
      onClick();
    });
  }
  return b;
}

function updateBar() {
  const checked = $$('#artists-table input[type=checkbox]:checked').length;
  $('#artist-delete').disabled = checked === 0;
  const toggleBtn = $('#artist-toggle-active');
  if (toggleBtn) {
    toggleBtn.disabled = checked === 0;
  }
}

function updateClearBtn() {
  const searchInput = $('#artist-search');
  const clearBtn = $('#artist-search-clear');
  if (clearBtn) {
    clearBtn.hidden = !searchInput.value;
  }
}

export function renderArtists() {
  const q = $('#artist-search').value.trim().toLowerCase();
  const statusFilter = $('#artist-status-filter')?.value || '';
  const genderFilter = $('#artist-gender-filter')?.value || '';
  const box = $('#artists-table');
  box.textContent = '';
  updateClearBtn();

  const rows = artists.filter((a) => {
    if (statusFilter === 'active' && !a.is_active) return false;
    if (statusFilter === 'inactive' && a.is_active) return false;
    if (genderFilter && a.gender !== genderFilter) return false;
    if (!q) return true;
    return `${a.name} ${a.gender}`.toLowerCase().includes(q);
  });

  $('#artist-count').textContent = `${rows.length} of ${artists.length}`;

  if (!rows.length) {
    box.appendChild(el('p', { class: 'empty' }, artists.length ? 'No artists found matching your filter.' : 'No artists yet.'));
    updateBar();
    return;
  }

  const table = el('table');
  const thead = el('thead');
  const headRow = el('tr');
  ['', 'Name', 'Gender', 'Status', ''].forEach((title, idx) => {
    headRow.appendChild(el('th', { class: idx === 0 ? 'num' : '' }, title));
  });
  thead.appendChild(headRow);
  table.appendChild(thead);

  const tbody = el('tbody');
  rows.forEach((a, rowIndex) => {
    const tr = el('tr');

    // Checkbox with Shift-click range support
    const tdChk = el('td', { class: 'num' });
    const chk = el('input', { type: 'checkbox', value: String(a.artist_id), 'aria-label': a.name });
    chk.dataset.index = String(rowIndex);
    chk.addEventListener('click', (e) => {
      if (e.shiftKey && lastCheckedIndex !== -1) {
        const checkboxes = Array.from($$('#artists-table input[type=checkbox]'));
        const start = Math.min(lastCheckedIndex, rowIndex);
        const end = Math.max(lastCheckedIndex, rowIndex);
        checkboxes.slice(start, end + 1).forEach((c) => {
          c.checked = chk.checked;
        });
      }
      lastCheckedIndex = rowIndex;
      updateBar();
    });
    tdChk.appendChild(chk);
    tr.appendChild(tdChk);

    // Name + Release Count + Inline Rename
    const tdName = el('td', { class: 'title-cell' });
    const nameSpan = el('span', null, a.name);
    const link = el('a', { class: 'clickable-link', title: 'Filter releases by this artist' }, [nameSpan]);
    link.addEventListener('click', (e) => {
      e.preventDefault();
      if (onSelectArtistCallback) onSelectArtistCallback(a.name);
    });
    tdName.appendChild(link);

    // Releases count pill
    const count = releaseCounts[a.name] || 0;
    if (count > 0) {
      const countPill = el('span', { class: 'count-pill', title: `${count} releases. Click to view.` }, `${count} releases`);
      countPill.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        if (onSelectArtistCallback) onSelectArtistCallback(a.name);
      });
      tdName.appendChild(countPill);
    }

    // Double-click inline rename
    tdName.addEventListener('dblclick', () => {
      const input = el('input', { type: 'text', value: a.name, style: 'width:12rem;padding:0.15rem 0.4rem' });
      tdName.textContent = '';
      tdName.appendChild(input);
      input.focus();
      input.select();

      const save = async () => {
        const newName = input.value.trim();
        if (newName && newName !== a.name) {
          try {
            await api('PATCH', `/api/artists/${a.artist_id}`, { name: newName });
            a.name = newName;
          } catch (err) {
            alert(`Failed to rename: ${err.message}`);
          }
        }
        renderArtists();
      };

      input.addEventListener('blur', save);
      input.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
          input.blur();
        } else if (e.key === 'Escape') {
          renderArtists();
        }
      });
    });

    tr.appendChild(tdName);

    // Gender toggle
    const tdG = el('td');
    const nextGender = { female: 'male', male: 'mixed', mixed: 'female' };
    tdG.appendChild(
      genderBadge(a.gender, async () => {
        const target = nextGender[a.gender] || 'female';
        try {
          await api('PATCH', `/api/artists/${a.artist_id}`, { gender: target });
          a.gender = target;
          renderArtists();
        } catch (err) {
          alert(`Failed to change gender: ${err.message}`);
        }
      })
    );
    tr.appendChild(tdG);

    // Status toggle
    const tdStatus = el('td');
    const statusBtn = el(
      'button',
      {
        type: 'button',
        class: `badge badge-toggle ${a.is_active ? 'on' : ''}`,
        title: a.is_active ? 'Click to deactivate' : 'Click to activate',
      },
      a.is_active ? 'active' : 'inactive'
    );
    statusBtn.addEventListener('click', async () => {
      try {
        statusBtn.disabled = true;
        const updated = await api('PATCH', `/api/artists/${a.artist_id}`, { is_active: !a.is_active });
        a.is_active = updated.is_active;
        statusBtn.className = `badge badge-toggle ${a.is_active ? 'on' : ''}`;
        statusBtn.textContent = a.is_active ? 'active' : 'inactive';
        statusBtn.title = a.is_active ? 'Click to deactivate' : 'Click to activate';
        statusBtn.disabled = false;
      } catch (err) {
        statusBtn.disabled = false;
        alert(`Failed to update status: ${err.message}`);
      }
    });
    tdStatus.appendChild(statusBtn);
    tr.appendChild(tdStatus);

    // Delete
    const tdDel = el('td', { style: 'text-align:right' });
    const delBtn = el('button', { class: 'btn btn-danger btn-sm', type: 'button' }, 'Delete');
    delBtn.addEventListener('click', async () => {
      if (!confirm(`Delete artist "${a.name}"? Their releases will also be deleted.`)) return;
      try {
        delBtn.disabled = true;
        await api('DELETE', `/api/artists/${a.artist_id}`);
        artists = artists.filter((x) => x.artist_id !== a.artist_id);
        renderArtists();
        await loadArtists();
      } catch (err) {
        alert(`Delete failed: ${err.message}`);
        await loadArtists();
      }
    });
    tdDel.appendChild(delBtn);
    tr.appendChild(tdDel);

    tbody.appendChild(tr);
  });

  table.appendChild(tbody);
  box.appendChild(tableWrap(table));
  updateBar();
}

export async function loadArtists() {
  try {
    artists = await api('GET', '/api/artists');
    renderArtists();
  } catch (err) {
    console.error('Failed to load artists:', err);
  }
}

export function initArtists() {
  const searchInput = $('#artist-search');
  const clearBtn = $('#artist-search-clear');
  const statusFilter = $('#artist-status-filter');
  const genderFilter = $('#artist-gender-filter');
  const addTextarea = $('#artist-add textarea[name=names]');
  const countBadge = $('#artist-input-count');

  searchInput.addEventListener('input', renderArtists);
  if (statusFilter) statusFilter.addEventListener('change', renderArtists);
  if (genderFilter) genderFilter.addEventListener('change', renderArtists);

  if (clearBtn) {
    clearBtn.addEventListener('click', () => {
      searchInput.value = '';
      renderArtists();
      searchInput.focus();
    });
  }

  // Live count preview
  if (addTextarea && countBadge) {
    addTextarea.addEventListener('input', () => {
      const parsed = parseArtistNames(addTextarea.value);
      countBadge.textContent = parsed.length > 0 ? `(${parsed.length} detected)` : '';
    });
  }

  $('#artist-add').addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const form = ev.target;
    const msg = form.querySelector('.msg');
    const names = parseArtistNames(form.names.value);

    if (!names.length) return;

    try {
      const res = await api('POST', '/api/artists/batch', {
        names,
        gender: form.gender.value,
      });
      form.reset();
      if (countBadge) countBadge.textContent = '';
      const statusText = `Added: ${res.added}${res.skipped.length ? ` · already existed: ${res.skipped.join(', ')}` : ''}`;
      showMsg(msg, statusText, true);
      await loadArtists();
    } catch (err) {
      showMsg(msg, err.message, false);
    }
  });

  $('#artist-check-all').addEventListener('change', function () {
    const isChecked = this.checked;
    $$('#artists-table input[type=checkbox]').forEach((cb) => {
      cb.checked = isChecked;
    });
    updateBar();
  });

  $('#artists-table').addEventListener('change', (ev) => {
    if (ev.target.type === 'checkbox') updateBar();
  });

  // Bulk status toggle
  const toggleActiveBtn = $('#artist-toggle-active');
  if (toggleActiveBtn) {
    toggleActiveBtn.addEventListener('click', async () => {
      const checkedBoxes = Array.from($$('#artists-table input[type=checkbox]:checked'));
      if (!checkedBoxes.length) return;

      const byId = Object.fromEntries(artists.map((a) => [String(a.artist_id), a]));
      const selected = checkedBoxes.map((cb) => byId[cb.value]).filter(Boolean);
      if (!selected.length) return;

      const anyInactive = selected.some((a) => !a.is_active);
      const targetState = anyInactive;

      try {
        toggleActiveBtn.disabled = true;
        await Promise.all(
          selected.map((a) => api('PATCH', `/api/artists/${a.artist_id}`, { is_active: targetState }))
        );
        $('#artist-check-all').checked = false;
        await loadArtists();
      } catch (err) {
        alert(`Failed to update status: ${err.message}`);
      } finally {
        toggleActiveBtn.disabled = false;
      }
    });
  }

  // Export CSV
  $('#artist-export')?.addEventListener('click', () => {
    exportToCSV('artists.csv', artists, {
      name: 'Name',
      gender: 'Gender',
      is_active: 'Active',
    });
  });

  // Export JSON (grouped into female, male, mixed blocks)
  $('#artist-export-json')?.addEventListener('click', () => {
    const grouped = {
      female: [],
      male: [],
      mixed: [],
    };
    for (const a of artists) {
      if (grouped[a.gender]) {
        grouped[a.gender].push(a.name);
      } else {
        grouped.mixed.push(a.name);
      }
    }
    grouped.female.sort((a, b) => a.localeCompare(b));
    grouped.male.sort((a, b) => a.localeCompare(b));
    grouped.mixed.sort((a, b) => a.localeCompare(b));

    const jsonStr = JSON.stringify(grouped, null, 2);
    const blob = new Blob([jsonStr], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'artists.json';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  });

  // Import JSON
  const importBtn = $('#artist-import-json');
  const importFile = $('#artist-import-json-file');
  if (importBtn && importFile) {
    importBtn.addEventListener('click', () => {
      importFile.value = '';
      importFile.click();
    });

    importFile.addEventListener('change', async (e) => {
      const file = e.target.files?.[0];
      if (!file) return;

      try {
        importBtn.disabled = true;
        importBtn.textContent = 'Importing...';
        const text = await file.text();
        const parsed = JSON.parse(text);

        if (typeof parsed !== 'object' || parsed === null) {
          throw new Error('Invalid JSON format: root must be an object with female, male, or mixed blocks');
        }

        const res = await api('POST', '/api/artists/import-json', parsed);
        alert(`Imported ${res.added} new artists (${res.skipped?.length || 0} already existed).`);
        await loadArtists();
      } catch (err) {
        alert(`Import failed: ${err.message}`);
      } finally {
        importBtn.disabled = false;
        importBtn.textContent = 'Import JSON';
      }
    });
  }

  $('#artist-delete').addEventListener('click', async () => {
    const checkedBoxes = Array.from($$('#artists-table input[type=checkbox]:checked'));
    if (!checkedBoxes.length) return;

    const byId = Object.fromEntries(artists.map((a) => [String(a.artist_id), a]));
    const names = checkedBoxes.map((cb) => byId[cb.value]?.name).filter(Boolean);

    if (!confirm(`Delete selected artists (${names.length})? Their releases will also be deleted:\n${names.join(', ')}`)) {
      return;
    }

    try {
      const idsToDelete = new Set(checkedBoxes.map((cb) => Number(cb.value)));
      await Promise.all(checkedBoxes.map((cb) => api('DELETE', `/api/artists/${cb.value}`)));
      artists = artists.filter((x) => !idsToDelete.has(x.artist_id));
      $('#artist-check-all').checked = false;
      renderArtists();
      await loadArtists();
    } catch (err) {
      alert(`Bulk delete failed: ${err.message}`);
      await loadArtists();
    }
  });
}
