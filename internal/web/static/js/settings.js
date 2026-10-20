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
    id: 'scraper',
    title: 'Scraper',
    fields: [
      FIELD('SCRAPER_USER_AGENTS', 'User-Agents', 'textarea', { mono: true, hint: 'One per line; on 403 an agent is skipped for 1h and the next is tried. Empty value falls back to the built-in default' }),
    ],
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
    id: 'grok',
    title: 'Grok',
    fields: [
      FIELD('GROK_ENABLED', 'Enable @grok commands', 'bool', { hint: 'Enable @grok это правда / перескажи / мнение in replies' }),
      FIELD('GROK_RATE_LIMIT', 'Rate Limit (requests per min)', 'number', { min: 1, max: 1000, unit: 'req/min', hint: 'Maximum allowed Grok calls per minute, shared across all @grok commands' }),
      FIELD('GROK_MAX_CHARS', 'Max Target Text Length (chars)', 'number', { min: 100, max: 50000, unit: 'chars', hint: 'Max input character length passed to Grok' }),
    ],
    extra: 'grok',
  },
  { id: 'translation', title: 'Translation', extra: 'translation' },
  { id: 'advanced', title: 'Advanced', extra: 'advanced' },
  { id: 'system', title: 'System Info', extra: 'system' },
];

const state = {
  entries: new Map(), // key -> {value, source, masked, default}
  edited: new Map(),  // key -> raw editor string, only while unsaved
  system: [],
  advancedRows: [],
  active: 'downloads',
  keyRejected: false,
  models: null, // Gemini catalog, loaded on demand
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
  } else {
    node = el('input', { type: f.type });
    if (f.min !== undefined) node.min = f.min;
    if (f.max !== undefined) node.max = f.max;
    if (f.placeholder) node.placeholder = f.placeholder;
    if (f.mono) node.style.cssText = 'font-family:monospace; font-size:0.8rem;';
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
  node.dataset.key = f.key;
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
  // Scrolls sideways on narrow screens instead of widening the page.
  return el('div', { style: 'overflow-x:auto' }, table);
}

// renderSectionFields puts short fields in a table card and long text fields in full-width editors below it.
function renderSectionFields(section, root) {
  const short = section.fields.filter((f) => f.type !== 'textarea');
  if (short.length) {
    const card = el('div', { class: 'settings-card' });
    card.appendChild(fieldsTable(short.map((f) => fieldRow(f))));
    root.appendChild(card);
  }
  section.fields
    .filter((f) => f.type === 'textarea')
    .forEach((f) => renderTextEditor(root, { key: f.key, title: f.label, desc: f.hint, mono: f.mono }));
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
    case 'rate_limited': return { cls: 'warn', label: 'rate limited' };
    case 'slow': return { cls: 'warn', label: 'slow' };
    case 'dead': return { cls: 'dead', label: 'unavailable' };
    default: return { cls: 'unknown', label: (avail || 'unknown').replace(/_/g, ' ') };
  }
}

function keyStatus() {
  if (!effectiveValue('GEMINI_API_KEY')) return { dot: 'none', badge: el('span', { class: 'badge' }, 'No Key') };
  if (state.keyRejected) return { dot: 'broken', badge: el('span', { class: 'badge expired' }, 'Key Rejected') };
  return { dot: 'ok', badge: el('span', { class: 'badge on' }, 'Configured') };
}

function renderGeminiCard(root) {
  const card = el('div', { class: 'pipeline-detail-card' });

  const status = keyStatus();
  const head = el('div', { class: 'pipeline-detail-head' });
  const titleWrap = el('div', { class: 'pipeline-detail-title-wrap' });
  const dot = el('span', { class: `status-dot ${status.dot}` });
  let badge = status.badge;
  titleWrap.appendChild(dot);
  titleWrap.appendChild(el('h3', { class: 'pipeline-detail-title' }, 'Google Gemini'));
  titleWrap.appendChild(badge);
  const refreshStatus = () => {
    const next = keyStatus();
    dot.className = `status-dot ${next.dot}`;
    badge.replaceWith(next.badge);
    badge = next.badge;
  };
  head.appendChild(titleWrap);
  card.appendChild(head);

  // API key
  const keySection = el('div', { class: 'pipeline-key-section' });
  keySection.appendChild(el('div', { class: 'pipeline-section-title' }, 'API Key'));
  keySection.appendChild(el('div', { class: 'pipeline-section-desc' }, 'Google AI Studio key. "Check Key" verifies the saved key.'));
  const keyRow = el('div', { class: 'pipeline-key-row' });
  const keyInput = el('input', {
    type: 'password',
    placeholder: 'AIzaSy... or AQ.Ab8...',
    value: effectiveValue('GEMINI_API_KEY'),
    autocomplete: 'off',
  });
  keyInput.addEventListener('input', () => {
    state.edited.set('GEMINI_API_KEY', keyInput.value);
    state.keyRejected = false;
    updateSaveBar();
    refreshStatus();
  });
  const showBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Show');
  showBtn.addEventListener('click', () => {
    const isPass = keyInput.type === 'password';
    keyInput.type = isPass ? 'text' : 'password';
    showBtn.textContent = isPass ? 'Hide' : 'Show';
  });
  const checkKeyBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Check Key');
  checkKeyBtn.addEventListener('click', async () => {
    checkKeyBtn.disabled = true;
    checkKeyBtn.textContent = 'Checking...';
    try {
      const res = await api('GET', '/api/translation');
      state.keyRejected = !!res.key_rejected;
      toast(state.keyRejected ? 'Gemini rejected the saved key' : 'Saved key is accepted', state.keyRejected ? 'error' : 'success');
    } catch (err) {
      toast(`Key check failed: ${err.message}`, 'error');
    } finally {
      renderTranslationSection($('#settings-body'));
    }
  });
  keyRow.appendChild(keyInput);
  keyRow.appendChild(showBtn);
  keyRow.appendChild(checkKeyBtn);
  keySection.appendChild(keyRow);
  card.appendChild(keySection);

  // Model
  const modelSection = el('div', { class: 'pipeline-key-section' });
  const modelHead = el('div', { style: 'display:flex; align-items:center; gap:0.5rem;' });
  modelHead.appendChild(el('div', { class: 'pipeline-section-title' }, 'Model'));
  modelHead.appendChild(srcBadge('GEMINI_MODEL'));
  modelSection.appendChild(modelHead);
  modelSection.appendChild(el('div', { class: 'pipeline-section-desc' }, `Used for subtitles and @grok. Empty = ${entryOf('GEMINI_MODEL').default || 'default'}.`));

  const modelRow = el('div', { class: 'pipeline-key-row' });
  const modelInput = el('input', {
    type: 'text',
    list: 'gemini-models',
    placeholder: entryOf('GEMINI_MODEL').default || '',
    value: effectiveValue('GEMINI_MODEL'),
    autocomplete: 'off',
  });
  const datalist = el('datalist', { id: 'gemini-models' });
  const modelInfo = el('span', { class: 'status-badge status-unknown' }, 'unknown');

  const currentModel = () => modelInput.value.trim() || entryOf('GEMINI_MODEL').default || '';
  const showModelInfo = (status) => {
    const info = availabilityInfo(status);
    modelInfo.className = `status-badge status-${info.cls}`;
    modelInfo.textContent = info.label;
  };
  const fillCatalog = () => {
    datalist.textContent = '';
    (state.models || []).forEach((m) => {
      datalist.appendChild(el('option', { value: m.id }, `${m.name || m.id} · ${fmtContext(m.context_length)}`));
    });
    const m = (state.models || []).find((x) => x.id === currentModel());
    showModelInfo(m && m.status);
  };
  const loadCatalog = async (refresh) => {
    try {
      const data = await api('GET', `/api/translation/models${refresh ? '?refresh=true' : ''}`);
      state.models = data.models || [];
      fillCatalog();
    } catch (err) {
      toast(`Failed to load Gemini models: ${err.message}`, 'error');
    }
  };

  modelInput.addEventListener('input', () => {
    state.edited.set('GEMINI_MODEL', modelInput.value.trim());
    updateSaveBar();
    fillCatalog();
  });
  modelInput.addEventListener('focus', () => { if (!state.models) loadCatalog(false); });

  const checkModelBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Check Model');
  checkModelBtn.addEventListener('click', async () => {
    checkModelBtn.disabled = true;
    checkModelBtn.textContent = 'Checking...';
    try {
      const [res] = await api('POST', '/api/translation/models/check', { models: [currentModel()] });
      showModelInfo(res && res.status);
    } catch (err) {
      toast(`Model check failed: ${err.message}`, 'error');
    } finally {
      checkModelBtn.disabled = false;
      checkModelBtn.textContent = 'Check Model';
    }
  });
  const reloadBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button', title: 'Reload the model list from Gemini' }, 'Reload List');
  reloadBtn.addEventListener('click', () => loadCatalog(true));

  modelRow.appendChild(modelInput);
  modelRow.appendChild(modelInfo);
  modelRow.appendChild(checkModelBtn);
  modelRow.appendChild(reloadBtn);
  modelSection.appendChild(modelRow);
  modelSection.appendChild(datalist);
  card.appendChild(modelSection);
  if (state.models) fillCatalog();

  root.appendChild(card);
}

async function runTranslationTest(text, resultEl, btn) {
  btn.disabled = true;
  btn.textContent = 'Testing...';
  resultEl.textContent = '';

  const body = { text, target_lang: 'ru' };
  if (state.edited.has('GEMINI_API_KEY')) body.api_key = state.edited.get('GEMINI_API_KEY');
  if (state.edited.has('GEMINI_MODEL')) body.model = state.edited.get('GEMINI_MODEL');
  if (state.edited.has('TRANSLATION_PROMPT')) body.prompt = state.edited.get('TRANSLATION_PROMPT');

  let res;
  try {
    res = await api('POST', '/api/translation/test', body);
  } catch (err) {
    res = { ok: false, error: err.message };
  }
  btn.disabled = false;
  btn.textContent = 'Test';

  resultEl.appendChild(el('div', { class: `timeline-step ${res.ok ? 'ok' : 'err'}` }, [
    el('div', { class: 'timeline-step-head' }, [
      el('span', { class: 'timeline-step-title' }, res.model || 'Gemini'),
      el('span', { class: 'timeline-step-meta' }, [
        el('span', { class: 'timeline-latency' }, res.latency_ms !== undefined ? `${res.latency_ms}ms` : ''),
        el('span', { class: `timeline-status-badge ${res.ok ? 'ok' : 'err'}` }, res.ok ? 'OK' : 'FAIL'),
      ]),
    ]),
    el('div', { class: 'timeline-step-body' }, res.ok ? res.result : (res.error || 'Unknown error')),
  ]));
}

function renderTranslationGeneralCard(root) {
  const card = el('div', { class: 'trans-general-card' });
  const grid = el('div', { class: 'trans-general-grid' });

  const fRu = FIELD('SUBS_SOURCE_PREF_RU', 'RU Source Priority', 'text', { mono: true, placeholder: 'ko, ja, en', hint: 'Preferred source languages' });
  const colRu = el('div', { class: 'trans-col' });
  const headRu = el('div', { class: 'trans-col-head' });
  headRu.appendChild(el('span', null, fRu.label));
  headRu.appendChild(srcBadge(fRu.key));
  const editRu = buildEditor(fRu);
  colRu.appendChild(headRu);
  colRu.appendChild(editRu);
  if (fRu.hint) colRu.appendChild(el('span', { class: 'fld-hint' }, fRu.hint));
  grid.appendChild(colRu);

  const fTimeout = FIELD('TRANSLATION_TIMEOUT', 'Request Timeout', 'number', { min: 10, max: 600, unit: 'sec', hint: 'Per Gemini request' });
  const colTimeout = el('div', { class: 'trans-col' });
  const headTimeout = el('div', { class: 'trans-col-head' });
  headTimeout.appendChild(el('span', null, fTimeout.label));
  headTimeout.appendChild(srcBadge(fTimeout.key));
  const editTimeout = buildEditor(fTimeout);
  colTimeout.appendChild(headTimeout);
  colTimeout.appendChild(editTimeout);
  if (fTimeout.hint) colTimeout.appendChild(el('span', { class: 'fld-hint' }, fTimeout.hint));
  grid.appendChild(colTimeout);

  const fYouTube = FIELD('SUBS_YOUTUBE_ONLY', 'YouTube Auto-translate Only', 'bool', { hint: 'Skip Gemini' });
  const colYouTube = el('div', { class: 'trans-col' });
  const headYouTube = el('label', { class: 'trans-col-head trans-toggle' });
  const editYouTube = buildEditor(fYouTube);
  headYouTube.appendChild(editYouTube);
  headYouTube.appendChild(el('span', null, fYouTube.label));
  headYouTube.appendChild(srcBadge(fYouTube.key));
  colYouTube.appendChild(headYouTube);
  if (fYouTube.hint) colYouTube.appendChild(el('span', { class: 'fld-hint' }, fYouTube.hint));
  grid.appendChild(colYouTube);

  card.appendChild(grid);
  root.appendChild(card);
}


function renderTranslationTestCard(root) {
  const card = el('div', { class: 'pipeline-test-card' });
  const head = el('div', { class: 'pipeline-card-header' });
  head.appendChild(el('h3', null, 'Test Translation'));
  card.appendChild(head);

  const form = el('div', { class: 'pipeline-test-form' });
  const row = el('div', { class: 'pipeline-test-row' });
  const input = el('input', {
    type: 'text',
    class: 'pipeline-test-input',
    placeholder: '리브 미모 난리도 아니야',
    value: '리브 미모 난리도 아니야',
  });
  const btn = el('button', { class: 'btn btn-primary btn-sm', type: 'button' }, 'Test');
  row.appendChild(input);
  row.appendChild(btn);
  form.appendChild(row);

  const result = el('div');
  form.appendChild(result);
  card.appendChild(form);
  btn.addEventListener('click', () => runTranslationTest(input.value, result, btn));

  root.appendChild(card);
}

function renderTranslationSection(root) {
  root.textContent = '';
  renderTranslationGeneralCard(root);
  renderGeminiCard(root);
  renderTextEditor(root, {
    key: 'TRANSLATION_PROMPT',
    title: 'Translation Prompt',
    desc: 'System prompt sent to Gemini with every subtitle translation.',
    collapsed: true,
  });
  renderTranslationTestCard(root);
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
          c.value = inputEl.value; // keep state in sync for re-renders of this section
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

// renderTextEditor edits a long setting (prompt, user agent list) at full width. The field grows with its
// content up to 70% of the viewport; mono keeps one item per line without wrapping.
function renderTextEditor(root, { key, title, desc, mono = false, collapsed = false }) {
  const details = el('details', { class: 'pipeline-prompt-details' });
  details.open = !collapsed;
  const summary = el('summary', { class: 'pipeline-prompt-summary' });
  summary.appendChild(el('span', null, title));
  summary.appendChild(srcBadge(key));

  const area = el('textarea', {
    class: `text-editor${mono ? ' mono' : ''}`,
    spellcheck: 'false',
    placeholder: 'Empty = built-in default',
  });
  if (mono) area.wrap = 'off';
  area.value = effectiveValue(key);
  const fit = () => {
    if (!area.offsetParent) return;
    area.style.height = 'auto';
    area.style.height = `${Math.min(area.scrollHeight + 2, window.innerHeight * 0.7)}px`;
  };
  area.addEventListener('input', () => {
    state.edited.set(key, area.value);
    updateSaveBar();
    fit();
  });
  details.addEventListener('toggle', fit);

  const resetBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Reset to Default');
  resetBtn.addEventListener('click', () => {
    area.value = entryOf(key).default || '';
    state.edited.set(key, area.value);
    updateSaveBar();
    fit();
  });

  const body = el('div', { class: 'pipeline-prompt-body' });
  if (desc) body.appendChild(el('div', { class: 'pipeline-section-desc' }, desc));
  body.appendChild(area);
  body.appendChild(el('div', { class: 'text-editor-actions' }, resetBtn));
  details.appendChild(summary);
  details.appendChild(body);
  root.appendChild(details);
  requestAnimationFrame(fit);
}

function renderSectionBody(sectionId) {
  const body = $('#settings-body');
  if (!body) return;
  body.textContent = '';

  const section = SECTIONS.find((s) => s.id === sectionId) || SECTIONS[0];
  const card = el('div', { class: 'settings-card' });

  switch (section.extra) {
    case 'storage':
      renderStorageCard(body);
      renderSectionFields(section, body);
      break;
    case 'grok':
      renderSectionFields(section, body);
      renderTextEditor(body, { key: 'GROK_PROMPT', title: 'Factcheck Prompt', desc: 'Instructions for "@grok это правда".', collapsed: true });
      renderTextEditor(body, { key: 'GROK_RETELL_PROMPT', title: 'Retell Prompt', desc: 'Instructions for "@grok перескажи".', collapsed: true });
      renderTextEditor(body, { key: 'GROK_OPINION_PROMPT', title: 'Opinion Prompt', desc: 'Instructions for "@grok мнение".', collapsed: true });
      break;
    case 'translation':
      renderTranslationSection(body);
      break;
    case 'advanced':
      renderAdvancedSection(body);
      break;
    case 'system':
      renderSystemSection(card);
      body.appendChild(card);
      break;
    default:
      renderSectionFields(section, body);
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
    const [data, configRows, transConfig] = await Promise.all([
      api('GET', '/api/settings'),
      api('GET', '/api/config').catch(() => []),
      api('GET', '/api/translation').catch(() => null),
    ]);
    state.entries = new Map(data.settings.map((e) => [e.key, e]));
    state.system = data.system || [];
    state.advancedRows = configRows.filter((c) => c.source === 'db');
    state.keyRejected = !!(transConfig && transConfig.key_rejected);
    if (state.active === 'providers' || !SECTIONS.some((s) => s.id === state.active)) {
      state.active = 'translation';
    }
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

// openSection makes the next settings render show the given subtab.
export function openSection(id) {
  if (SECTIONS.some((s) => s.id === id)) state.active = id;
}

export function initSettings() {
  $('#settings-save-btn')?.addEventListener('click', saveChanges);
  $('#settings-revert-btn')?.addEventListener('click', () => {
    state.edited.clear();
    updateSaveBar();
    renderSectionBody(state.active);
  });
}
