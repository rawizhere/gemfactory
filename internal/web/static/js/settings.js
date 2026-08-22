import { $, el, tableWrap, api } from './api.js';

function renderConfigTable(rows, editable) {
  const box = editable ? $('#config-db-table') : $('#config-env-table');
  box.textContent = '';

  if (!rows.length) {
    box.appendChild(el('p', { class: 'empty' }, 'No entries found.'));
    return;
  }

  const table = el('table');
  const thead = el('thead');
  const headRow = el('tr');
  ['Key', 'Value', 'Description', ''].forEach((title) => {
    headRow.appendChild(el('th', null, title));
  });
  thead.appendChild(headRow);
  table.appendChild(thead);

  const tbody = el('tbody');
  rows.forEach((c) => {
    const tr = el('tr');

    const tdKey = el('td');
    tdKey.style.whiteSpace = 'nowrap';
    tdKey.appendChild(el('code', { class: 'mono' }, c.key));
    tr.appendChild(tdKey);

    const tdVal = el('td');
    let inputEl = null;
    if (editable && c.editable) {
      inputEl = el('input', { type: 'text', value: c.value, style: 'width:100%' });
      tdVal.appendChild(inputEl);
    } else {
      tdVal.textContent = c.value;
      if (!c.value) tdVal.style.color = 'var(--muted)';
    }
    tr.appendChild(tdVal);

    const tdDesc = el('td', { style: 'color:var(--muted)' }, c.description || '');
    tr.appendChild(tdDesc);

    const tdAct = el('td', { style: 'text-align:right; white-space:nowrap;' });
    if (editable && c.editable) {
      const saveBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Save');
      const doSave = async () => {
        const newVal = inputEl.value;
        try {
          saveBtn.disabled = true;
          await api('PUT', `/api/config/${encodeURIComponent(c.key)}`, { value: newVal });
          saveBtn.textContent = 'Saved';
          setTimeout(() => {
            saveBtn.textContent = 'Save';
            saveBtn.disabled = false;
          }, 1500);
        } catch (err) {
          saveBtn.disabled = false;
          alert(`Failed to save: ${err.message}`);
        }
      };

      saveBtn.addEventListener('click', doSave);
      inputEl.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          doSave();
        }
      });

      tdAct.appendChild(saveBtn);
    } else {
      tdAct.appendChild(el('span', { style: 'color:var(--muted);font-size:0.78rem' }, 'read-only'));
    }
    tr.appendChild(tdAct);

    tbody.appendChild(tr);
  });

  table.appendChild(tbody);
  box.appendChild(tableWrap(table));
}

export async function loadStorageUsage() {
  const sizeEl = $('#storage-size');
  const filesEl = $('#storage-files');
  if (!sizeEl || !filesEl) return;

  try {
    const res = await api('GET', '/api/downloads/storage');
    if (res) {
      sizeEl.textContent = res.formatted || '0 B';
      filesEl.textContent = `(${res.files || 0} files)`;
    }
  } catch (err) {
    console.error('Failed to load storage usage:', err);
  }
}

export async function cleanStorage() {
  const btn = $('#clean-storage-btn');
  if (!confirm('Are you sure you want to clean all downloaded clips and cache?')) return;

  try {
    if (btn) {
      btn.disabled = true;
      btn.textContent = 'Cleaning...';
    }
    const res = await api('POST', '/api/downloads/storage/clean');
    alert(`Storage cleaned. Freed ${res.formatted} across ${res.files} files.`);
    await loadStorageUsage();
  } catch (err) {
    alert(`Failed to clean storage: ${err.message}`);
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = 'Clean Downloads';
    }
  }
}

export async function loadConfig() {
  try {
    const rows = await api('GET', '/api/config');
    renderConfigTable(rows.filter((c) => c.source === 'db'), true);
    renderConfigTable(rows.filter((c) => c.source === 'env'), false);
    await loadStorageUsage();
  } catch (err) {
    console.error('Failed to load config:', err);
  }
}

export function initSettings() {
  $('#clean-storage-btn')?.addEventListener('click', cleanStorage);
}
