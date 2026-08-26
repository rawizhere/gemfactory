import { $, el, tableWrap, api, toast, confirmDialog } from './api.js';

// ---------------------------------------------------------------------------
// Schema-driven settings page: sub-tabs, field editors, dirty tracking.
// ---------------------------------------------------------------------------

const PRESETS = ['ultrafast', 'superfast', 'veryfast', 'faster', 'fast', 'medium', 'slow'];

const FIELD = (key, label, type, opts = {}) => ({ key, label, type, ...opts });

const SECTIONS = [
  {
    id: 'downloads',
    title: 'Downloads',
    fields: [
      FIELD('DOWNLOAD_CONCURRENCY', 'Max Concurrent Downloads', 'number', { min: 1, max: 20, hint: 'Parallel download & re-encode jobs' }),
      FIELD('DOWNLOAD_RETENTION_HOURS', 'Auto-Cleanup Retention', 'number', { min: 1, max: 8760, unit: 'hours', hint: 'Downloaded files older than this are removed hourly' }),
      FIELD('TG_FILE_LIMIT_MB', 'Telegram File Limit', 'number', { min: 1, max: 2048, unit: 'MB', hint: 'Larger clips are rejected with a "shorter interval" error' }),
      FIELD('CLIP_MAX_DURATION_SECONDS', 'Max Clip Length', 'number', { min: 5, max: 3600, unit: 'sec', hint: 'Standard quality limit; end past video length clamps to the end' }),
      FIELD('CLIP_MAX_DURATION_HQ_SECONDS', 'Max HQ Clip Length', 'number', { min: 5, max: 600, unit: 'sec', hint: 'Limit for hq clips' }),
      FIELD('YTDLP_PROXY', 'yt-dlp Proxy', 'text', { placeholder: 'socks5://host:port (empty = direct)', hint: 'Applies to new yt-dlp processes' }),
      FIELD('CLIP_DELETE_STATUS', 'Status Card After Completion', 'select', { options: [['false', 'Keep status message'], ['true', 'Delete status message']] }),
    ],
    extra: 'storage',
  },
  {
    id: 'encoding',
    title: 'Encoding',
    fields: [
      FIELD('CLIP_CRF', 'Clip Video Quality (CRF)', 'number', { min: 15, max: 35, hint: 'H.264 CRF: lower = better quality' }),
      FIELD('SUBS_CRF', 'Subtitled Video Quality (CRF)', 'number', { min: 15, max: 35, hint: 'CRF for hardsubbed video' }),
      FIELD('CLIP_PRESET', 'x264 Encoding Preset', 'select', { options: PRESETS.map((p) => [p, p]), hint: 'CPU speed vs file compression' }),
      FIELD('CLIP_AUDIO_BITRATE', 'Audio Bitrate', 'select', { options: [['128k', '128k'], ['192k', '192k'], ['256k', '256k'], ['320k', '320k']], hint: 'AAC audio encoding bitrate' }),
    ],
  },
  {
    id: 'translation',
    title: 'Translation',
    fields: [
      FIELD('TRANSLATION_FALLBACK_ORDER', 'Provider Fallback Chain', 'text', { mono: true, hint: 'Comma-separated: gemini, nvidia, groq, opencode, openrouter' }),
      FIELD('SUBS_SOURCE_PREF_RU', 'RU Source Subtitle Priority', 'text', { mono: true, hint: 'Preferred source languages for RU translation' }),
      FIELD('TRANSLATION_TIMEOUT', 'LLM Request Timeout', 'number', { min: 10, max: 600, unit: 'sec', hint: 'Per-provider timeout before falling back' }),
      FIELD('SUBS_GOOGLE_ONLY', 'Google Translate Only', 'bool', { hint: 'Skip all LLM providers, use Google Translate web endpoint' }),
    ],
  },
  { id: 'providers', title: 'Providers', extra: 'providers' },
  { id: 'advanced', title: 'Advanced', extra: 'advanced' },
  { id: 'system', title: 'System Info', extra: 'system' },
];

const PROVIDERS = [
  { id: 'gemini', label: 'Google Gemini', keyPlaceholder: 'AIzaSy... or AQ.Ab8...' },
  { id: 'nvidia', label: 'NVIDIA NIM', keyPlaceholder: 'nvapi-...' },
  { id: 'groq', label: 'Groq', keyPlaceholder: 'gsk_...' },
  { id: 'opencode', label: 'OpenCode Zen', keyPlaceholder: 'sk-...' },
  { id: 'openrouter', label: 'OpenRouter', keyPlaceholder: 'sk-or-v1-...' },
];

const state = {
  entries: new Map(), // key -> {value, source, masked, default}
  edited: new Map(),  // key -> raw editor string, only while unsaved
  system: [],
  advancedRows: [],
  active: 'downloads',
};

function entryOf(key) {
  return state.entries.get(key) || { value: '', source: 'default', masked: false, default: '' };
}

function effectiveValue(key) {
  return state.edited.has(key) ? state.edited.get(key) : entryOf(key).value;
}

function isDirty() {
  return state.edited.size > 0;
}

function updateSaveBar() {
  const bar = $('#settings-savebar');
  const count = $('#dirty-count');
  if (!bar || !count) return;
  bar.classList.toggle('visible', isDirty());
  count.textContent = `${state.edited.size} unsaved change${state.edited.size === 1 ? '' : 's'}`;
}

// ---------------------------------------------------------------------------
// Field rendering
// ---------------------------------------------------------------------------

function srcBadge(key) {
  const e = entryOf(key);
  if (state.edited.has(key)) return el('span', { class: 'src-badge' }, 'modified');
  return el('span', { class: `src-badge src-${e.source}` }, e.source);
}

function buildEditor(f) {
  const e = entryOf(f.key);
  const current = effectiveValue(f.key);

  if (f.type === 'bool') {
    const cb = el('input', { type: 'checkbox' });
    cb.checked = String(current) === 'true';
    cb.addEventListener('change', () => {
      state.edited.set(f.key, cb.checked ? 'true' : 'false');
      updateSaveBar();
    });
    return cb;
  }

  let node;
  if (f.type === 'select') {
    node = el('select');
    f.options.forEach(([val, label]) => {
      node.appendChild(el('option', { value: val }, label));
    });
    node.value = current;
  } else if (f.type === 'textarea') {
    node = el('textarea', { rows: f.rows || 5, style: 'width:100%; font-family:monospace; font-size:0.8rem; line-height:1.4; resize:vertical; padding:0.5rem;' });
    node.value = current;
  } else {
    node = el('input', { type: f.type });
    if (f.min !== undefined) node.min = f.min;
    if (f.max !== undefined) node.max = f.max;
    if (f.placeholder) node.placeholder = f.placeholder;
    if (f.mono) node.style.cssText = 'font-family:monospace; font-size:0.8rem;';
    if (f.unit) node.style.width = '9rem';
    node.value = current;
  }

  node.addEventListener('input', () => {
    state.edited.set(f.key, node.value);
    updateSaveBar();
  });
  node.addEventListener('change', () => {
    state.edited.set(f.key, node.value);
    updateSaveBar();
  });
  return node;
}

// One settings row: Setting | Value | Description (same layout as the Advanced table).
function fieldRow(f, opts = {}) {
  const tr = el('tr');

  const tdLabel = el('td', { class: 'fld-label' });
  const labelWrap = el('div', { style: 'display:flex; align-items:center; gap:0.5rem;' });
  labelWrap.appendChild(el('span', null, f.label));
  labelWrap.appendChild(srcBadge(f.key));
  tdLabel.appendChild(labelWrap);
  tr.appendChild(tdLabel);

  const editor = buildEditor(f);
  if (opts.placeholder) editor.placeholder = opts.placeholder;

  const tdEdit = el('td', { class: 'fld-edit' });
  if (opts.toggle) {
    const holder = el('div', { style: 'display:flex; gap:0.5rem;' });
    holder.appendChild(editor);
    const btn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Show');
    btn.addEventListener('click', () => {
      editor.type = editor.type === 'password' ? 'text' : 'password';
      btn.textContent = editor.type === 'password' ? 'Show' : 'Hide';
    });
    holder.appendChild(btn);
    tdEdit.appendChild(holder);
  } else {
    tdEdit.appendChild(editor);
  }
  tr.appendChild(tdEdit);

  tr.appendChild(el('td', { class: 'fld-hint' }, f.hint || ''));
  return tr;
}

function fieldsTable(rows) {
  const table = el('table');
  const headRow = el('tr');
  ['Setting', 'Value', 'Description'].forEach((t) => headRow.appendChild(el('th', null, t)));
  const thead = el('thead');
  thead.appendChild(headRow);
  table.appendChild(thead);
  const tbody = el('tbody');
  rows.forEach((r) => tbody.appendChild(r));
  table.appendChild(tbody);
  return table;
}

function renderSectionFields(section, root) {
  root.appendChild(fieldsTable(section.fields.map((f) => fieldRow(f))));
}

// ---------------------------------------------------------------------------
// Extra widgets per section
// ---------------------------------------------------------------------------

async function loadStorageUsage() {
  const sizeEl = $('#storage-size');
  const filesEl = $('#storage-files');
  if (!sizeEl || !filesEl) return;
  try {
    const res = await api('GET', '/api/downloads/storage');
    sizeEl.textContent = res.formatted || '0 B';
    filesEl.textContent = `(${res.files || 0} files)`;
  } catch (err) {
    console.error('Failed to load storage usage:', err);
  }
}

async function cleanStorage() {
  const ok = await confirmDialog('Delete all downloaded clips and cache? Active downloads are kept.');
  if (!ok) return;
  try {
    await api('POST', '/api/downloads/storage/clean');
    toast('Storage cleaned', 'success');
    await loadStorageUsage();
  } catch (err) {
    toast(`Failed to clean storage: ${err.message}`, 'error');
  }
}

function renderStorageCard(root) {
  const card = el('div', { class: 'settings-card' });
  const row = el('div', { class: 'settings-row' });
  const info = el('div', { class: 'storage-info' });
  info.appendChild(el('span', { class: 'storage-label' }, 'Downloads Folder: '));
  const size = el('strong', { id: 'storage-size' }, '…');
  const files = el('span', { class: 'hint', style: 'margin:0', id: 'storage-files' }, '');
  info.appendChild(size);
  info.appendChild(files);
  const btn = el('button', { class: 'btn btn-danger btn-sm', type: 'button', id: 'clean-storage-btn' }, 'Clean Downloads');
  btn.addEventListener('click', cleanStorage);
  row.appendChild(info);
  row.appendChild(btn);
  card.appendChild(row);
  root.appendChild(card);
  loadStorageUsage();
}

let testResultEl = null;

function collectProviderOverrides() {
  const overrides = {};
  PROVIDERS.forEach((p) => {
    const k = p.id.toUpperCase();
    if (state.edited.has(k + '_API_KEY')) overrides[`${p.id}_api_key`] = state.edited.get(k + '_API_KEY');
    if (state.edited.has(k + '_MODELS')) overrides[`${p.id}_models`] = state.edited.get(k + '_MODELS');
  });
  if (state.edited.has('TRANSLATION_PROMPT')) overrides.prompt = state.edited.get('TRANSLATION_PROMPT');
  return overrides;
}

async function testTranslation() {
  const btn = $('#translation-test-btn');
  const input = $('#translation-test-input');
  const text = input && input.value.trim() ? input.value.trim() : '리브 미모 난리도 아니야';

  if (btn) {
    btn.disabled = true;
    btn.textContent = 'Testing...';
  }
  if (testResultEl) {
    testResultEl.textContent = 'Probing fallback chain...';
    testResultEl.style.color = '';
  }

  const appendRow = (e) => {
    if (!testResultEl) return;
    const row = document.createElement('div');
    row.style.marginBottom = '0.35rem';
    row.style.wordBreak = 'break-word';
    const name = e.provider === 'google' ? 'Google Translate' : `${e.provider}/${e.model || '?'}`;
    if (e.ok) {
      row.style.color = '#4caf50';
      row.textContent = `${name}: ${e.result ?? ''}`;
    } else {
      row.style.color = '#f44336';
      row.textContent = `${name}: FAIL — ${e.error ?? 'unknown error'}${e.result ? ` | Raw: ${e.result}` : ''}`;
    }
    testResultEl.appendChild(row);
  };

  try {
    const res = await fetch('/api/translation/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text, target_lang: 'ru', ...collectProviderOverrides() }),
    });
    if (!res.ok) throw new Error((await res.text()) || res.statusText);
    if (testResultEl) testResultEl.textContent = '';

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let idx;
      while ((idx = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, idx).trim();
        buffer = buffer.slice(idx + 1);
        if (!line) continue;
        const event = JSON.parse(line);
        if (event.type === 'result') appendRow(event);
        if (event.type === 'done' && !event.success && testResultEl && !testResultEl.hasChildNodes()) {
          testResultEl.textContent = 'No provider succeeded';
          testResultEl.style.color = '#f44336';
        }
      }
    }
  } catch (err) {
    if (testResultEl) {
      testResultEl.textContent = `Error: ${err.message}`;
      testResultEl.style.color = '#f44336';
    }
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = 'Test Translation';
    }
  }
}

// ---------------------------------------------------------------------------
// Provider model catalog and fallback-chain drag & drop
// ---------------------------------------------------------------------------

function fmtContext(n) {
  const v = typeof n === 'string' ? parseInt(n, 10) : n;
  if (typeof v !== 'number' || !isFinite(v) || v <= 0) return '—';
  if (v >= 1e6) return (v / 1e6).toFixed(v % 1e6 === 0 ? 0 : 1).replace(/\.0$/, '') + 'M';
  if (v >= 1e3) return Math.round(v / 1e3) + 'K';
  return String(v);
}

function availabilityInfo(avail) {
  switch ((avail || '').toLowerCase()) {
    case 'ok': return { cls: 'ok', label: 'ok' };
    case 'rate_limited':
    case 'quota': return { cls: 'warn', label: (avail || 'limited').replace(/_/g, ' ') };
    case 'slow': return { cls: 'warn', label: 'slow' };
    case 'dead':
    case 'not_chat': return { cls: 'dead', label: (avail || 'down').replace(/_/g, ' ') };
    default: return { cls: 'unknown', label: (avail || 'unknown').replace(/_/g, ' ') };
  }
}

function getDragAfter(container, selector, y) {
  const els = [...container.querySelectorAll(selector + ':not(.dragging)')];
  let closest = { offset: -Infinity, element: null };
  for (const child of els) {
    const box = child.getBoundingClientRect();
    const offset = y - box.top - box.height / 2;
    if (offset < 0 && offset > closest.offset) closest = { offset, element: child };
  }
  return closest.element;
}

function findModelsInput(providerId) {
  return document.getElementById('model-input-' + providerId);
}

function renderModelChain(providerId) {
  const ul = document.getElementById('model-chain-' + providerId);
  if (!ul) return;
  const key = providerId.toUpperCase() + '_MODELS';
  const ids = (effectiveValue(key) || '').split(',').map((s) => s.trim()).filter(Boolean);
  ul.textContent = '';
  if (!ids.length) {
    ul.appendChild(el('li', { class: 'model-chain-empty' }, 'No models yet — use + in the catalog below.'));
    return;
  }
  ids.forEach((id) => ul.appendChild(modelChainItem(providerId, id)));
}

function modelChainItem(providerId, id) {
  const li = el('li', { class: 'model-chain-item', draggable: 'true', 'data-model': id });
  li.appendChild(el('span', { class: 'drag-handle' }, '\u280F'));
  li.appendChild(el('span', { class: 'model-id' }, id));
  const rm = el('button', { class: 'model-remove', type: 'button', title: 'Remove from chain' }, '\u00D7');
  rm.addEventListener('click', () => removeModelFromChain(providerId, id));
  li.appendChild(rm);
  li.addEventListener('dragstart', (e) => {
    if (e.target.closest('.model-remove')) { e.preventDefault(); return; }
    li.classList.add('dragging');
    e.dataTransfer.effectAllowed = 'move';
    try { e.dataTransfer.setData('text/plain', id); } catch (_) {}
  });
  li.addEventListener('dragend', () => {
    li.classList.remove('dragging');
    commitModelOrder(providerId);
  });
  return li;
}

function commitModelOrder(providerId) {
  const ul = document.getElementById('model-chain-' + providerId);
  if (!ul) return;
  const ids = [...ul.querySelectorAll('.model-chain-item')].map((li) => li.dataset.model);
  state.edited.set(providerId.toUpperCase() + '_MODELS', ids.join(','));
  updateSaveBar();
}

function removeModelFromChain(providerId, id) {
  const key = providerId.toUpperCase() + '_MODELS';
  const ids = (effectiveValue(key) || '').split(',').map((s) => s.trim()).filter(Boolean).filter((x) => x !== id);
  state.edited.set(key, ids.join(','));
  const input = findModelsInput(providerId);
  if (input) input.value = ids.join(',');
  updateSaveBar();
  renderModelChain(providerId);
}

function addModelToChain(providerId, id) {
  const key = providerId.toUpperCase() + '_MODELS';
  const ids = (effectiveValue(key) || '').split(',').map((s) => s.trim()).filter(Boolean);
  if (ids.includes(id)) { toast(`${id} is already in the ${providerId} chain`, 'info'); return; }
  ids.push(id);
  state.edited.set(key, ids.join(','));
  const input = findModelsInput(providerId);
  if (input) input.value = ids.join(',');
  updateSaveBar();
  renderModelChain(providerId);
  toast(`Added ${id} to ${providerId} chain`, 'success');
}

function catalogRow(providerId, m) {
  const tr = el('tr');
  const tdModel = el('td');
  tdModel.appendChild(el('code', { class: 'mono' }, m.id || '—'));
  if (m.reasoning) tdModel.appendChild(el('span', { class: 'tag tag-reason' }, 'reasoning'));
  tr.appendChild(tdModel);
  tr.appendChild(el('td', null, m.free ? el('span', { class: 'free-badge' }, 'Free') : ''));
  tr.appendChild(el('td', null, fmtContext(m.context_length)));
  const info = availabilityInfo(m.status);
  tr.appendChild(el('td', null, el('span', { class: `status-badge status-${info.cls}` }, info.label)));
  const tdAct = el('td', { style: 'text-align:right; white-space:nowrap;' });
  const add = el('button', { class: 'btn btn-ghost btn-sm', type: 'button', title: 'Add to chain' }, '+');
  add.addEventListener('click', () => addModelToChain(providerId, m.id));
  tdAct.appendChild(add);
  tr.appendChild(tdAct);
  return tr;
}

const STATUS_RANK = { ok: 0, rate_limited: 1, quota: 1, slow: 1, dead: 2, not_chat: 2, unknown: 3, error: 4 };

const catalogExpanded = new Set();

function sortModelsByStatus(models) {
  return [...models].sort((a, b) => {
    const ra = STATUS_RANK[(a.status || '').toLowerCase()] ?? 3;
    const rb = STATUS_RANK[(b.status || '').toLowerCase()] ?? 3;
    return ra - rb;
  });
}

function renderCatalogTable(providerId, box, models) {
  box.textContent = '';
  if (!models.length) {
    box.appendChild(el('p', { class: 'empty' }, 'No models returned by the provider catalog.'));
    return;
  }
  const sorted = sortModelsByStatus(models);
  const free = sorted.filter((m) => m.free);
  const paid = sorted.filter((m) => !m.free);
  const expanded = catalogExpanded.has(providerId);
  const visible = expanded ? sorted : free;

  if (!visible.length) {
    box.appendChild(el('p', { class: 'empty' }, 'No free models in this provider catalog.'));
  } else {
    const table = el('table', { class: 'catalog-table' });
    const thead = el('thead');
    const hr = el('tr');
    ['Model', 'Free', 'Context', 'Status', ''].forEach((h) => hr.appendChild(el('th', null, h)));
    thead.appendChild(hr);
    table.appendChild(thead);
    const tbody = el('tbody');
    visible.forEach((m) => tbody.appendChild(catalogRow(providerId, m)));
    table.appendChild(tbody);
    box.appendChild(tableWrap(table));
  }

  if (paid.length) {
    const label = expanded ? 'Show less' : `Show more (${paid.length})`;
    const more = el('button', { class: 'btn btn-ghost btn-sm catalog-more', type: 'button' }, label);
    more.addEventListener('click', () => {
      if (expanded) catalogExpanded.delete(providerId);
      else catalogExpanded.add(providerId);
      renderCatalogTable(providerId, box, models);
    });
    box.appendChild(more);
  }
}

async function loadModelCatalog(providerId, box, status, refresh) {
  try {
    const url = `/api/translation/models?provider=${encodeURIComponent(providerId)}${refresh ? '&refresh=true' : ''}`;
    const data = await api('GET', url);
    const models = Array.isArray(data) ? data : (data && data.models) ? data.models : [];
    renderCatalogTable(providerId, box, models);
    if (status) { status.textContent = `${models.length} models`; status.className = 'catalog-status ok'; }
  } catch (err) {
    box.textContent = '';
    box.appendChild(el('p', { class: 'empty' }, `Model catalog unavailable: ${err.message}`));
    if (status) { status.textContent = 'unavailable'; status.className = 'catalog-status err'; }
  }
}

async function checkModels(providerId, box, status) {
  if (status) { status.textContent = 'Checking\u2026'; status.className = 'catalog-status'; }
  try {
    const data = await api('GET', `/api/translation/models?provider=${encodeURIComponent(providerId)}&refresh=true`);
    const models = Array.isArray(data) ? data : (data && data.models) ? data.models : [];
    let ids = models.filter((m) => m.free).map((m) => m.id);
    if (!ids.length) {
      const key = providerId.toUpperCase() + '_MODELS';
      ids = (effectiveValue(key) || '').split(',').map((s) => s.trim()).filter(Boolean);
    }
    if (!ids.length) {
      toast('No models to check for this provider', 'info');
      if (status) { status.textContent = 'no models'; status.className = 'catalog-status'; }
      return;
    }
    await api('POST', '/api/translation/models/check', { provider: providerId, models: ids });
    await loadModelCatalog(providerId, box, status, true);
  } catch (err) {
    toast(`Check failed: ${err.message}`, 'error');
    if (status) { status.textContent = 'check failed'; status.className = 'catalog-status err'; }
  }
}

function setupProviderDrag(card, handle) {
  handle.addEventListener('mousedown', () => { card.draggable = true; });
  handle.addEventListener('mouseup', () => { card.draggable = false; });
  card.addEventListener('dragstart', (e) => {
    card.classList.add('dragging');
    e.dataTransfer.effectAllowed = 'move';
    try { e.dataTransfer.setData('text/plain', card.dataset.provider); } catch (_) {}
  });
  card.addEventListener('dragend', () => {
    card.classList.remove('dragging');
    card.draggable = false;
    commitProviderOrder();
  });
}

function commitProviderOrder() {
  const host = document.getElementById('providers-chain');
  if (!host) return;
  const ids = [...host.querySelectorAll('.provider-card')].map((c) => c.dataset.provider);
  state.edited.set('TRANSLATION_FALLBACK_ORDER', ids.join(','));
  updateSaveBar();
}

function buildProviderCard(p) {
  const card = el('div', { class: 'provider-card', 'data-provider': p.id });

  const head = el('div', { class: 'provider-card-head' });
  const handle = el('span', { class: 'drag-handle', title: 'Drag to reorder providers' }, '\u280F');
  head.appendChild(handle);
  head.appendChild(el('span', { class: 'provider-name' }, p.label));
  card.appendChild(head);

  const apiRow = fieldRow(
    FIELD(p.id.toUpperCase() + '_API_KEY', 'API Key', 'password'),
    { placeholder: p.keyPlaceholder, toggle: true });
  const modelsRow = fieldRow(FIELD(p.id.toUpperCase() + '_MODELS', 'Models Fallback Chain', 'text', { mono: true }));
  const modelsInput = modelsRow.querySelector('input');
  modelsInput.id = 'model-input-' + p.id;
  modelsInput.addEventListener('input', () => renderModelChain(p.id));
  card.appendChild(fieldsTable([apiRow, modelsRow]));

  const chainWrap = el('div', { class: 'model-chain-wrap' });
  chainWrap.appendChild(el('div', { class: 'subhead' }, 'Chain order — drag to reorder'));
  const ul = el('ul', { class: 'model-chain', id: 'model-chain-' + p.id, 'data-provider': p.id });
  ul.addEventListener('dragover', (e) => {
    e.preventDefault();
    const dragging = ul.querySelector('.model-chain-item.dragging');
    if (!dragging) return;
    const after = getDragAfter(ul, '.model-chain-item', e.clientY);
    if (after == null) ul.appendChild(dragging);
    else ul.insertBefore(dragging, after);
  });
  chainWrap.appendChild(ul);
  card.appendChild(chainWrap);
  renderModelChain(p.id);

  const catWrap = el('div', { class: 'model-catalog-wrap' });
  const toolbar = el('div', { class: 'catalog-toolbar' });
  const refreshBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Refresh');
  const checkBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Check availability');
  const status = el('span', { class: 'catalog-status' });
  toolbar.appendChild(refreshBtn);
  toolbar.appendChild(checkBtn);
  toolbar.appendChild(status);
  catWrap.appendChild(toolbar);
  const catBox = el('div', { class: 'catalog-box', id: 'catalog-' + p.id });
  catWrap.appendChild(catBox);
  card.appendChild(catWrap);

  refreshBtn.addEventListener('click', () => loadModelCatalog(p.id, catBox, status, true));
  checkBtn.addEventListener('click', () => checkModels(p.id, catBox, status));
  loadModelCatalog(p.id, catBox, status, false);

  setupProviderDrag(card, handle);
  return card;
}

function renderProvidersSection(root) {
  // Sort providers by the effective fallback chain, unknown ones go last.
  const chain = (effectiveValue('TRANSLATION_FALLBACK_ORDER') || '')
    .split(',').map((s) => s.trim().toLowerCase()).filter(Boolean);
  const ordered = [...PROVIDERS].sort((a, b) => {
    const ia = chain.indexOf(a.id); const ib = chain.indexOf(b.id);
    return (ia === -1 ? 99 : ia) - (ib === -1 ? 99 : ib);
  });

  const chainHost = el('div', { class: 'prov-chain', id: 'providers-chain' });
  chainHost.addEventListener('dragover', (e) => {
    e.preventDefault();
    const dragging = chainHost.querySelector('.provider-card.dragging');
    if (!dragging) return;
    const after = getDragAfter(chainHost, '.provider-card', e.clientY);
    if (after == null) chainHost.appendChild(dragging);
    else chainHost.insertBefore(dragging, after);
  });
  ordered.forEach((p) => chainHost.appendChild(buildProviderCard(p)));
  root.appendChild(chainHost);

  // Prompt shapes LLM output for every provider.
  const promptRows = [
    el('tr', { class: 'prov-head' }, el('td', { colspan: '3' }, 'Prompt')),
    fieldRow(FIELD('TRANSLATION_PROMPT', 'AI Translation System Prompt', 'textarea', { rows: 6 })),
  ];
  root.appendChild(fieldsTable(promptRows));

  const row = el('div', { class: 'settings-row' });
  const input = el('input', { type: 'text', id: 'translation-test-input', placeholder: 'Test text...', style: 'flex:1; font-size:0.85rem;' });
  const tbtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button', id: 'translation-test-btn' }, 'Test Translation');
  tbtn.addEventListener('click', testTranslation);
  row.appendChild(input);
  row.appendChild(tbtn);
  root.appendChild(row);

  testResultEl = el('div', { style: 'font-size:0.85rem; color:var(--muted); word-break:break-word;' });
  root.appendChild(testResultEl);
}

// ---------------------------------------------------------------------------
// Advanced (generic DB config) and System sections
// ---------------------------------------------------------------------------

function renderConfigTable(rows, editable) {
  const box = editable ? $('#config-db-table') : $('#config-env-table');
  box.textContent = '';
  if (!rows.length) {
    box.appendChild(el('p', { class: 'empty' }, 'No extra settings. Everything configurable lives in the sections above.'));
    return;
  }

  const table = el('table');
  const headRow = el('tr');
  ['Key', 'Value', 'Description', ''].forEach((t) => headRow.appendChild(el('th', null, t)));
  const thead = el('thead');
  thead.appendChild(headRow);
  table.appendChild(thead);

  const tbody = el('tbody');
  rows.forEach((c) => {
    const tr = el('tr');

    const tdKey = el('td', { style: 'white-space:nowrap' });
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
    tr.appendChild(el('td', { style: 'color:var(--muted)' }, c.description || ''));

    const tdAct = el('td', { style: 'text-align:right; white-space:nowrap;' });
    if (editable && c.editable) {
      const saveBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Save');
      saveBtn.addEventListener('click', async () => {
        saveBtn.disabled = true;
        try {
          await api('PUT', `/api/config/${encodeURIComponent(c.key)}`, { value: inputEl.value });
          saveBtn.textContent = 'Saved';
          setTimeout(() => { saveBtn.textContent = 'Save'; saveBtn.disabled = false; }, 1500);
        } catch (err) {
          saveBtn.disabled = false;
          toast(`Failed to save: ${err.message}`, 'error');
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

async function renderAdvancedSection(root) {
  const holder = el('div', { id: 'config-db-table' });
  root.appendChild(holder);
  renderConfigTable(state.advancedRows, true);
}

function renderSystemSection(root) {
  const table = el('table');
  const headRow = el('tr');
  ['Key', 'Value'].forEach((t) => headRow.appendChild(el('th', null, t)));
  const thead = el('thead');
  thead.appendChild(headRow);
  table.appendChild(thead);

  const tbody = el('tbody');
  state.system.forEach((row) => {
    const tr = el('tr');
    const k = el('td');
    k.appendChild(el('code', { class: 'mono' }, row.key));
    tr.appendChild(k);
    tr.appendChild(el('td', null, row.value || '—'));
    tbody.appendChild(tr);
  });
  table.appendChild(tbody);
  root.appendChild(tableWrap(table));
}

// ---------------------------------------------------------------------------
// Page assembly, load and save
// ---------------------------------------------------------------------------

function renderSectionBody(sectionId) {
  const body = $('#settings-body');
  if (!body) return;
  body.textContent = '';

  const section = SECTIONS.find((s) => s.id === sectionId) || SECTIONS[0];
  const card = el('div', { class: 'settings-card' });

  switch (section.extra) {
    case 'storage':
      renderStorageCard(body);
      renderSectionFields(section, card);
      body.appendChild(card);
      break;
    case 'providers':
      renderProvidersSection(card);
      body.appendChild(card);
      break;
    case 'advanced':
      renderAdvancedSection(body);
      break;
    case 'system':
      renderSystemSection(card);
      body.appendChild(card);
      break;
    default:
      renderSectionFields(section, card);
      body.appendChild(card);
  }
}

function renderSubtabs() {
  const host = $('#settings-subtabs');
  if (!host) return;
  host.textContent = '';
  SECTIONS.forEach((s) => {
    const b = el('button', { class: `subtab${s.id === state.active ? ' active' : ''}`, type: 'button' }, s.title);
    b.id = `subtab-${s.id}`;
    // Advanced is an escape hatch for custom DB keys; hide it while there are none.
    if (s.id === 'advanced' && !state.advancedRows.length) b.style.display = 'none';
    b.addEventListener('click', () => {
      state.active = s.id;
      renderSubtabs();
      renderSectionBody(s.id);
    });
    host.appendChild(b);
  });
}

export async function loadConfig() {
  try {
    const [data, configRows] = await Promise.all([
      api('GET', '/api/settings'),
      api('GET', '/api/config').catch(() => []),
    ]);
    state.entries = new Map(data.settings.map((e) => [e.key, e]));
    state.system = data.system || [];
    state.advancedRows = configRows.filter((c) => c.source === 'db');
    if (state.active === 'advanced' && !state.advancedRows.length) {
      state.active = SECTIONS[0].id;
    }
    state.edited.clear();
    updateSaveBar();
    renderSubtabs();
    renderSectionBody(state.active);
  } catch (err) {
    toast(`Failed to load settings: ${err.message}`, 'error');
  }
}

async function saveChanges() {
  if (!isDirty()) return;
  const payload = {};
  state.edited.forEach((v, k) => { payload[k] = v; });
  try {
    await api('POST', '/api/settings', payload);
    toast('Settings saved', 'success');
    state.edited.clear();
    await loadConfig();
  } catch (err) {
    toast(`Failed to save: ${err.message}`, 'error');
  }
}

export function initSettings() {
  $('#settings-save-btn')?.addEventListener('click', saveChanges);
  $('#settings-revert-btn')?.addEventListener('click', () => {
    state.edited.clear();
    updateSaveBar();
    renderSectionBody(state.active);
  });
}
