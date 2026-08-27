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
  { id: 'translation', title: 'Translation', extra: 'translation' },
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
  selectedProvider: 'gemini',
  brokenKeys: {},
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

function collectProviderOverrides() {
  const overrides = {
    fallback_order: effectiveValue('TRANSLATION_FALLBACK_ORDER'),
  };
  PROVIDERS.forEach((p) => {
    const k = p.id.toUpperCase();
    if (state.edited.has(k + '_API_KEY')) overrides[`${p.id}_api_key`] = state.edited.get(k + '_API_KEY');
    if (state.edited.has(k + '_MODELS')) overrides[`${p.id}_models`] = state.edited.get(k + '_MODELS');
  });
  if (state.edited.has('TRANSLATION_PROMPT')) overrides.prompt = state.edited.get('TRANSLATION_PROMPT');
  return overrides;
}

function getProvider(id) {
  return PROVIDERS.find((p) => p.id === id) || { id, label: id, keyPlaceholder: 'API key' };
}

function getEffectiveFallbackChain() {
  const raw = effectiveValue('TRANSLATION_FALLBACK_ORDER');
  const items = (raw || '')
    .split(',')
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean);
  if (items.length > 0) return items;
  return ['gemini', 'nvidia', 'groq', 'opencode', 'openrouter'];
}

function setEffectiveFallbackChain(chain) {
  const val = chain.join(',');
  state.edited.set('TRANSLATION_FALLBACK_ORDER', val);
  updateSaveBar();
}

function getProviderModelIds(providerId) {
  const key = providerId.toUpperCase() + '_MODELS';
  const val = effectiveValue(key);
  if (val !== undefined && val !== null && String(val).trim() !== '') {
    return String(val).split(',').map((s) => s.trim()).filter(Boolean);
  }
  const def = entryOf(key).default;
  if (def) {
    return String(def).split(',').map((s) => s.trim()).filter(Boolean);
  }
  return [];
}

function setProviderModelIds(providerId, models) {
  const key = providerId.toUpperCase() + '_MODELS';
  state.edited.set(key, models.join(','));
  updateSaveBar();
}

function getProviderStatus(providerId) {
  const key = effectiveValue(providerId.toUpperCase() + '_API_KEY');
  if (!key) return 'none';
  if (state.brokenKeys && state.brokenKeys[providerId]) return 'broken';
  return 'ok';
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

function showAddProviderDialog(onAdded) {
  const chain = getEffectiveFallbackChain();
  const available = PROVIDERS.filter((p) => !chain.includes(p.id));
  const options = available.length ? available : PROVIDERS;

  const dlg = el('dialog', { class: 'add-provider-dialog' });
  const title = el('h3', { style: 'margin:0 0 0.85rem; font-size:1rem; font-weight:600;' }, 'Add Provider to Chain');
  const form = el('form', { class: 'add-provider-form', method: 'dialog' });

  const provLabel = el('label', null, 'Provider');
  const provSelect = el('select', { style: 'width:100%' });
  options.forEach((p) => {
    provSelect.appendChild(el('option', { value: p.id }, p.label));
  });
  provLabel.appendChild(provSelect);

  const keyLabel = el('label', null, 'API Key (optional)');
  const keyInput = el('input', {
    type: 'text',
    placeholder: getProvider(provSelect.value).keyPlaceholder,
    style: 'width:100%; font-family:monospace;',
  });
  keyLabel.appendChild(keyInput);

  provSelect.addEventListener('change', () => {
    keyInput.placeholder = getProvider(provSelect.value).keyPlaceholder;
  });

  const actions = el('div', { class: 'add-provider-actions' });
  const cancelBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Cancel');
  const addBtn = el('button', { class: 'btn btn-primary btn-sm', type: 'submit' }, 'Add Provider');

  cancelBtn.addEventListener('click', () => dlg.close());
  form.addEventListener('submit', (e) => {
    e.preventDefault();
    const pid = provSelect.value;
    const keyVal = keyInput.value.trim();
    if (keyVal) {
      state.edited.set(pid.toUpperCase() + '_API_KEY', keyVal);
    }
    const curChain = getEffectiveFallbackChain();
    if (!curChain.includes(pid)) {
      curChain.push(pid);
      setEffectiveFallbackChain(curChain);
    }
    state.selectedProvider = pid;
    dlg.close();
    toast(`Added ${getProvider(pid).label} to fallback chain`, 'success');
    if (onAdded) onAdded();
  });

  actions.appendChild(cancelBtn);
  actions.appendChild(addBtn);
  form.appendChild(provLabel);
  form.appendChild(keyLabel);
  form.appendChild(actions);

  dlg.appendChild(title);
  dlg.appendChild(form);
  dlg.addEventListener('close', () => dlg.remove());
  document.body.appendChild(dlg);
  dlg.showModal();
}

async function runChainTest(text, timeline, btn) {
  if (btn) {
    btn.disabled = true;
    btn.textContent = 'Testing...';
  }
  timeline.textContent = '';

  const sample = text.trim() || '리브 미모 난리도 아니야';
  const startTime = performance.now();

  try {
    const res = await fetch('/api/translation/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        text: sample,
        target_lang: 'ru',
        ...collectProviderOverrides(),
      }),
    });
    if (!res.ok) throw new Error((await res.text()) || res.statusText);

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let stepIndex = 0;

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
        if (event.type === 'result') {
          stepIndex++;
          const latency = Math.round(performance.now() - startTime);
          const stepEl = el('div', { class: `timeline-step ${event.ok ? 'ok' : 'err'}` });

          const head = el('div', { class: 'timeline-step-head' });
          const provLabel = PROVIDERS.find((p) => p.id === event.provider)?.label || event.provider;
          const modelLabel = event.model ? `${provLabel} / ${event.model}` : provLabel;

          const title = el('span', { class: 'timeline-step-title' }, [
            el('strong', null, `#${stepIndex}`),
            el('span', null, modelLabel),
          ]);

          const meta = el('span', { class: 'timeline-step-meta' }, [
            el('span', { class: 'timeline-latency' }, `${latency}ms`),
            el('span', { class: `timeline-status-badge ${event.ok ? 'ok' : 'err'}` }, event.ok ? 'OK' : 'FAIL'),
          ]);

          head.appendChild(title);
          head.appendChild(meta);
          stepEl.appendChild(head);

          const body = el('div', { class: 'timeline-step-body' },
            event.ok
              ? (event.result || 'Empty result')
              : (event.error || 'Unknown error') + (event.result ? ` (Raw: ${event.result})` : '')
          );
          stepEl.appendChild(body);
          timeline.appendChild(stepEl);
        } else if (event.type === 'done' && !event.success && !timeline.hasChildNodes()) {
          const emptyStep = el('div', { class: 'timeline-step err' }, [
            el('div', { class: 'timeline-step-head' }, [
              el('span', { class: 'timeline-step-title' }, 'Pipeline execution completed'),
              el('span', { class: 'timeline-status-badge err' }, 'FAIL'),
            ]),
            el('div', { class: 'timeline-step-body' }, 'No provider in the fallback chain succeeded.'),
          ]);
          timeline.appendChild(emptyStep);
        }
      }
    }
  } catch (err) {
    const errStep = el('div', { class: 'timeline-step err' }, [
      el('div', { class: 'timeline-step-head' }, [
        el('span', { class: 'timeline-step-title' }, 'Test Error'),
        el('span', { class: 'timeline-status-badge err' }, 'FAIL'),
      ]),
      el('div', { class: 'timeline-step-body' }, err.message),
    ]);
    timeline.appendChild(errStep);
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = 'Test Chain';
    }
  }
}

function renderPipelineMaster(masterContainer, refresh) {
  masterContainer.textContent = '';

  const chain = getEffectiveFallbackChain();

  const card = el('div', { class: 'pipeline-card' });
  const head = el('div', { class: 'pipeline-card-header' });
  head.appendChild(el('h3', null, 'Provider Fallback Chain'));

  const addBtn = el('button', { class: 'btn btn-primary btn-sm', type: 'button' }, '+ Add Provider');
  addBtn.addEventListener('click', () => showAddProviderDialog(refresh));
  head.appendChild(addBtn);
  card.appendChild(head);

  const list = el('div', { class: 'pipeline-prov-list' });

  list.addEventListener('dragover', (e) => {
    e.preventDefault();
    const dragging = list.querySelector('.pipeline-prov-item.dragging');
    if (!dragging) return;
    const after = getDragAfter(list, '.pipeline-prov-item', e.clientY);
    if (after == null) list.appendChild(dragging);
    else list.insertBefore(dragging, after);
  });

  list.addEventListener('drop', (e) => {
    e.preventDefault();
    const ids = [...list.querySelectorAll('.pipeline-prov-item')].map((node) => node.dataset.provider);
    setEffectiveFallbackChain(ids);
    refresh();
  });

  chain.forEach((pId, i) => {
    const p = getProvider(pId);
    const status = getProviderStatus(pId);
    const modelCount = getProviderModelIds(pId).length;

    const item = el('div', {
      class: `pipeline-prov-item${pId === state.selectedProvider ? ' active' : ''}`,
      'data-provider': pId,
      draggable: 'true',
    });

    const handle = el('span', { class: 'drag-handle', title: 'Drag to reorder' }, '⠿');
    const dot = el('span', { class: `status-dot ${status}`, title: status === 'ok' ? 'Configured' : (status === 'broken' ? 'Key rejected' : 'No API key') });
    const name = el('span', { class: 'pipeline-prov-name' }, p.label);
    const badge = el('span', { class: 'pipeline-prov-badge' }, `${modelCount} model${modelCount === 1 ? '' : 's'}`);

    const orderBtns = el('div', { class: 'pipeline-order-btns' });
    const btnUp = el('button', { class: 'btn-icon', type: 'button', title: 'Move up' }, '↑');
    const btnDown = el('button', { class: 'btn-icon', type: 'button', title: 'Move down' }, '↓');

    if (i === 0) btnUp.disabled = true;
    if (i === chain.length - 1) btnDown.disabled = true;

    btnUp.addEventListener('click', (e) => {
      e.stopPropagation();
      if (i > 0) {
        const next = [...chain];
        [next[i - 1], next[i]] = [next[i], next[i - 1]];
        setEffectiveFallbackChain(next);
        refresh();
      }
    });

    btnDown.addEventListener('click', (e) => {
      e.stopPropagation();
      if (i < chain.length - 1) {
        const next = [...chain];
        [next[i], next[i + 1]] = [next[i + 1], next[i]];
        setEffectiveFallbackChain(next);
        refresh();
      }
    });

    item.addEventListener('dragstart', (e) => {
      if (e.target.closest('.btn-icon')) { e.preventDefault(); return; }
      item.classList.add('dragging');
      e.dataTransfer.effectAllowed = 'move';
      try { e.dataTransfer.setData('text/plain', pId); } catch (_) {}
    });

    item.addEventListener('dragend', () => {
      item.classList.remove('dragging');
      const ids = [...list.querySelectorAll('.pipeline-prov-item')].map((node) => node.dataset.provider);
      setEffectiveFallbackChain(ids);
      refresh();
    });

    item.addEventListener('click', () => {
      state.selectedProvider = pId;
      refresh();
    });

    orderBtns.appendChild(btnUp);
    orderBtns.appendChild(btnDown);

    item.appendChild(handle);
    item.appendChild(dot);
    item.appendChild(name);
    item.appendChild(badge);
    item.appendChild(orderBtns);

    list.appendChild(item);
  });

  card.appendChild(list);
  masterContainer.appendChild(card);
}

const catalogCache = new Map();

function renderPipelineDetail(detailContainer, refresh) {
  detailContainer.textContent = '';

  const chain = getEffectiveFallbackChain();
  const pid = state.selectedProvider;
  if (!pid || !chain.includes(pid)) {
    if (chain.length > 0) {
      state.selectedProvider = chain[0];
      return renderPipelineDetail(detailContainer, refresh);
    }
    const emptyCard = el('div', { class: 'pipeline-detail-card' }, [
      el('p', { class: 'empty' }, 'No providers in the fallback chain. Click "+ Add Provider" on the left to add one.')
    ]);
    detailContainer.appendChild(emptyCard);
    return;
  }

  const p = getProvider(pid);
  const status = getProviderStatus(pid);

  const card = el('div', { class: 'pipeline-detail-card' });

  const head = el('div', { class: 'pipeline-detail-head' });
  const titleWrap = el('div', { class: 'pipeline-detail-title-wrap' });
  titleWrap.appendChild(el('h3', { class: 'pipeline-detail-title' }, p.label));

  let statusBadge;
  if (status === 'ok') {
    statusBadge = el('span', { class: 'badge on' }, 'Configured');
  } else if (status === 'broken') {
    statusBadge = el('span', { class: 'badge expired' }, 'Key Broken');
  } else {
    statusBadge = el('span', { class: 'badge' }, 'No Key');
  }
  titleWrap.appendChild(statusBadge);
  head.appendChild(titleWrap);

  const removeBtn = el('button', { class: 'btn btn-danger btn-sm', type: 'button' }, 'Remove from Chain');
  removeBtn.addEventListener('click', () => {
    const next = chain.filter((x) => x !== pid);
    setEffectiveFallbackChain(next);
    state.selectedProvider = next[0] || null;
    toast(`Removed ${p.label} from fallback chain`, 'info');
    refresh();
  });
  head.appendChild(removeBtn);
  card.appendChild(head);

  const keySection = el('div', { class: 'pipeline-key-section' });
  keySection.appendChild(el('div', { class: 'pipeline-section-title' }, 'API Key'));
  keySection.appendChild(el('div', { class: 'pipeline-section-desc' }, `Secret authentication key for ${p.label}`));

  const keyRow = el('div', { class: 'pipeline-key-row' });
  const keyInput = el('input', {
    type: 'password',
    placeholder: p.keyPlaceholder,
    value: effectiveValue(pid.toUpperCase() + '_API_KEY'),
    autocomplete: 'off',
  });

  const showBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Show');
  showBtn.addEventListener('click', () => {
    const isPass = keyInput.type === 'password';
    keyInput.type = isPass ? 'text' : 'password';
    showBtn.textContent = isPass ? 'Hide' : 'Show';
  });

  const checkBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Check Key');
  checkBtn.addEventListener('click', async () => {
    checkBtn.disabled = true;
    checkBtn.textContent = 'Checking...';
    try {
      await api('GET', `/api/translation/models?provider=${encodeURIComponent(pid)}&refresh=true`);
      state.brokenKeys[pid] = false;
      toast(`API key for ${p.label} is valid`, 'success');
    } catch (err) {
      state.brokenKeys[pid] = true;
      toast(`Key check failed: ${err.message}`, 'error');
    } finally {
      checkBtn.disabled = false;
      checkBtn.textContent = 'Check Key';
      refresh();
    }
  });

  keyInput.addEventListener('input', () => {
    state.edited.set(pid.toUpperCase() + '_API_KEY', keyInput.value);
    delete state.brokenKeys[pid];
    updateSaveBar();
  });

  keyRow.appendChild(keyInput);
  keyRow.appendChild(showBtn);
  keyRow.appendChild(checkBtn);
  keySection.appendChild(keyRow);
  card.appendChild(keySection);

  const modelsSection = el('div', { class: 'pipeline-models-section' });
  const modelsHead = el('div', { class: 'model-chain-header' });
  const modelsTitleWrap = el('div');
  modelsTitleWrap.appendChild(el('div', { class: 'pipeline-section-title' }, 'Active Models Chain'));
  modelsTitleWrap.appendChild(el('div', { class: 'pipeline-section-desc', style: 'margin:0' }, 'Models are attempted sequentially until one succeeds'));
  modelsHead.appendChild(modelsTitleWrap);

  const popoverWrap = el('div', { class: 'model-chain-popover-wrap' });
  const addModelBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, '+ Add Model');
  popoverWrap.appendChild(addModelBtn);

  const popover = el('div', { class: 'model-search-popover', hidden: true });
  const searchInput = el('input', {
    type: 'text',
    class: 'model-search-input',
    placeholder: 'Search models...',
    autocomplete: 'off',
  });
  const filterRow = el('div', { class: 'model-filter-row' });
  const freeLabel = el('label', { style: 'display:flex; align-items:center; gap:0.35rem; cursor:pointer;' });
  const freeCb = el('input', { type: 'checkbox' });
  freeLabel.appendChild(freeCb);
  freeLabel.appendChild(document.createTextNode('Free only'));
  const statusLabel = el('span', null, 'Loading...');
  filterRow.appendChild(freeLabel);
  filterRow.appendChild(statusLabel);

  const catalogList = el('ul', { class: 'model-catalog-list' });
  popover.appendChild(searchInput);
  popover.appendChild(filterRow);
  popover.appendChild(catalogList);
  popoverWrap.appendChild(popover);

  const updatePopoverList = (catalog) => {
    catalogList.textContent = '';
    const q = searchInput.value.trim().toLowerCase();
    const freeOnly = freeCb.checked;
    const activeIds = getProviderModelIds(pid);

    const filtered = catalog.filter((m) => {
      if (freeOnly && !m.free) return false;
      if (q && !m.id.toLowerCase().includes(q)) return false;
      return true;
    });

    statusLabel.textContent = `${filtered.length} model${filtered.length === 1 ? '' : 's'}`;

    if (!filtered.length) {
      catalogList.appendChild(el('li', { class: 'empty', style: 'padding:1rem 0; font-size:0.8rem;' }, 'No models match filter'));
      return;
    }

    filtered.forEach((m) => {
      const inChain = activeIds.includes(m.id);
      const row = el('li', { class: 'model-catalog-item' });
      const main = el('div', { class: 'model-catalog-item-main' });
      main.appendChild(el('span', { class: 'model-catalog-item-id' }, m.id));

      const meta = el('div', { class: 'model-catalog-item-meta' });
      if (m.free) meta.appendChild(el('span', { class: 'free-badge' }, 'Free'));
      if (m.context_length) meta.appendChild(el('span', { style: 'color:var(--muted)' }, fmtContext(m.context_length)));
      const info = availabilityInfo(m.status);
      meta.appendChild(el('span', { class: `status-badge status-${info.cls}` }, info.label));
      main.appendChild(meta);
      row.appendChild(main);

      if (inChain) {
        row.appendChild(el('span', { class: 'badge on', style: 'font-size:0.68rem;' }, 'Added'));
      } else {
        const add = el('button', { class: 'btn btn-ghost btn-sm', type: 'button', style: 'padding:0.1rem 0.4rem; font-size:0.75rem;' }, '+ Add');
        add.addEventListener('click', (e) => {
          e.stopPropagation();
          const current = getProviderModelIds(pid);
          if (!current.includes(m.id)) {
            current.push(m.id);
            setProviderModelIds(pid, current);
            toast(`Added ${m.id} to ${p.label} chain`, 'success');
            refresh();
          }
        });
        row.appendChild(add);
      }

      row.addEventListener('click', () => {
        const current = getProviderModelIds(pid);
        if (!current.includes(m.id)) {
          current.push(m.id);
          setProviderModelIds(pid, current);
          toast(`Added ${m.id} to ${p.label} chain`, 'success');
          refresh();
        }
      });

      catalogList.appendChild(row);
    });
  };

  const loadCatalog = async () => {
    statusLabel.textContent = 'Loading models...';
    try {
      let catalog = catalogCache.get(pid);
      if (!catalog) {
        const data = await api('GET', `/api/translation/models?provider=${encodeURIComponent(pid)}`);
        catalog = Array.isArray(data) ? data : (data && data.models) ? data.models : [];
        catalogCache.set(pid, catalog);
      }
      updatePopoverList(catalog);
    } catch (err) {
      catalogList.textContent = '';
      catalogList.appendChild(el('li', { class: 'empty', style: 'padding:1rem 0; font-size:0.8rem;' }, `Error loading models: ${err.message}`));
      statusLabel.textContent = 'Error';
    }
  };

  addModelBtn.addEventListener('click', (e) => {
    e.stopPropagation();
    const open = popover.hidden;
    popover.hidden = !open;
    if (open) {
      searchInput.value = '';
      freeCb.checked = false;
      searchInput.focus();
      loadCatalog();
    }
  });

  searchInput.addEventListener('input', () => {
    const catalog = catalogCache.get(pid) || [];
    updatePopoverList(catalog);
  });

  freeCb.addEventListener('change', () => {
    const catalog = catalogCache.get(pid) || [];
    updatePopoverList(catalog);
  });

  document.addEventListener('click', (e) => {
    if (!popover.hidden && !popoverWrap.contains(e.target)) {
      popover.hidden = true;
    }
  });

  modelsHead.appendChild(popoverWrap);
  modelsSection.appendChild(modelsHead);

  const ul = el('ul', { class: 'model-chain' });
  const modelIds = getProviderModelIds(pid);

  if (!modelIds.length) {
    ul.appendChild(el('li', { class: 'model-chain-empty' }, 'No active models in chain. Click "+ Add Model" above.'));
  } else {
    modelIds.forEach((mId, idx) => {
      const li = el('li', { class: 'model-chain-item', draggable: 'true', 'data-model': mId });
      li.appendChild(el('span', { class: 'drag-handle', title: 'Drag to reorder' }, '⠿'));
      li.appendChild(el('span', { class: 'model-id' }, mId));

      const btns = el('div', { class: 'pipeline-order-btns' });
      const btnUp = el('button', { class: 'btn-icon', type: 'button', title: 'Move up' }, '↑');
      const btnDown = el('button', { class: 'btn-icon', type: 'button', title: 'Move down' }, '↓');
      const btnRm = el('button', { class: 'btn-icon btn-remove', type: 'button', title: 'Remove model' }, '×');

      if (idx === 0) btnUp.disabled = true;
      if (idx === modelIds.length - 1) btnDown.disabled = true;

      btnUp.addEventListener('click', () => {
        if (idx > 0) {
          const next = [...modelIds];
          [next[idx - 1], next[idx]] = [next[idx], next[idx - 1]];
          setProviderModelIds(pid, next);
          refresh();
        }
      });

      btnDown.addEventListener('click', () => {
        if (idx < modelIds.length - 1) {
          const next = [...modelIds];
          [next[idx], next[idx + 1]] = [next[idx + 1], next[idx]];
          setProviderModelIds(pid, next);
          refresh();
        }
      });

      btnRm.addEventListener('click', () => {
        const next = modelIds.filter((x) => x !== mId);
        setProviderModelIds(pid, next);
        toast(`Removed ${mId}`, 'info');
        refresh();
      });

      btns.appendChild(btnUp);
      btns.appendChild(btnDown);
      btns.appendChild(btnRm);
      li.appendChild(btns);

      li.addEventListener('dragstart', (e) => {
        if (e.target.closest('.btn-icon')) { e.preventDefault(); return; }
        li.classList.add('dragging');
        e.dataTransfer.effectAllowed = 'move';
        try { e.dataTransfer.setData('text/plain', mId); } catch (_) {}
      });

      li.addEventListener('dragend', () => {
        li.classList.remove('dragging');
        const ids = [...ul.querySelectorAll('.model-chain-item')].map((node) => node.dataset.model);
        setProviderModelIds(pid, ids);
        refresh();
      });

      ul.appendChild(li);
    });

    ul.addEventListener('dragover', (e) => {
      e.preventDefault();
      const dragging = ul.querySelector('.model-chain-item.dragging');
      if (!dragging) return;
      const after = getDragAfter(ul, '.model-chain-item', e.clientY);
      if (after == null) ul.appendChild(dragging);
      else ul.insertBefore(dragging, after);
    });

    ul.addEventListener('drop', (e) => {
      e.preventDefault();
      const ids = [...ul.querySelectorAll('.model-chain-item')].map((node) => node.dataset.model);
      setProviderModelIds(pid, ids);
      refresh();
    });
  }

  modelsSection.appendChild(ul);
  card.appendChild(modelsSection);

  detailContainer.appendChild(card);
}

function renderTranslationGeneralCard(root) {
  const card = el('div', { class: 'trans-general-card' });
  const row = el('div', { class: 'trans-general-row' });

  const fRu = FIELD('SUBS_SOURCE_PREF_RU', 'RU Source Priority', 'text', { mono: true, placeholder: 'ko, ja, en', hint: 'Preferred source languages' });
  const colRu = el('div', { class: 'trans-field-col' });
  const lblRu = el('label', { class: 'trans-field-label' });
  lblRu.appendChild(el('span', null, fRu.label));
  lblRu.appendChild(srcBadge(fRu.key));
  const editRu = buildEditor(fRu);
  editRu.classList.add('trans-field-input');
  colRu.appendChild(lblRu);
  colRu.appendChild(editRu);
  if (fRu.hint) colRu.appendChild(el('span', { class: 'fld-hint' }, fRu.hint));
  row.appendChild(colRu);

  const fTimeout = FIELD('TRANSLATION_TIMEOUT', 'LLM Timeout', 'number', { min: 10, max: 600, unit: 'sec', hint: 'Per-provider timeout' });
  const colTimeout = el('div', { class: 'trans-field-col' });
  const lblTimeout = el('label', { class: 'trans-field-label' });
  lblTimeout.appendChild(el('span', null, fTimeout.label));
  lblTimeout.appendChild(srcBadge(fTimeout.key));
  const editTimeout = buildEditor(fTimeout);
  editTimeout.classList.add('trans-field-input');
  colTimeout.appendChild(lblTimeout);
  colTimeout.appendChild(editTimeout);
  if (fTimeout.hint) colTimeout.appendChild(el('span', { class: 'fld-hint' }, fTimeout.hint));
  row.appendChild(colTimeout);

  const fGoogle = FIELD('SUBS_GOOGLE_ONLY', 'Google Translate Only', 'bool', { hint: 'Skip LLMs' });
  const colGoogle = el('div', { class: 'trans-field-col trans-field-toggle' });
  const lblGoogle = el('label', { class: 'trans-field-label-toggle' });
  const editGoogle = buildEditor(fGoogle);
  lblGoogle.appendChild(editGoogle);
  lblGoogle.appendChild(el('span', null, fGoogle.label));
  lblGoogle.appendChild(srcBadge(fGoogle.key));
  colGoogle.appendChild(lblGoogle);
  if (fGoogle.hint) colGoogle.appendChild(el('span', { class: 'fld-hint' }, fGoogle.hint));
  row.appendChild(colGoogle);

  card.appendChild(row);
  root.appendChild(card);
}

function renderTranslationPrompt(root) {
  const details = el('details', { class: 'pipeline-prompt-details' });
  const summary = el('summary', { class: 'pipeline-prompt-summary' }, 'AI Translation System Prompt');
  const body = el('div', { class: 'pipeline-prompt-body' });
  const desc = el('div', { class: 'pipeline-section-desc' }, 'Instructions and guidelines passed to all LLM translation providers.');
  const promptArea = el('textarea', {
    rows: 6,
    class: 'pipeline-prompt-textarea',
    placeholder: 'Enter translation system prompt...',
  });
  promptArea.value = effectiveValue('TRANSLATION_PROMPT');
  promptArea.addEventListener('input', () => {
    state.edited.set('TRANSLATION_PROMPT', promptArea.value);
    updateSaveBar();
  });
  body.appendChild(desc);
  body.appendChild(promptArea);
  details.appendChild(summary);
  details.appendChild(body);
  root.appendChild(details);
}

function renderTranslationTestCard(root) {
  const testCard = el('div', { class: 'pipeline-test-card' });
  const testHead = el('div', { class: 'pipeline-card-header' });
  testHead.appendChild(el('h3', null, 'Test Fallback Chain'));
  testCard.appendChild(testHead);

  const testForm = el('div', { class: 'pipeline-test-form' });
  const testRow = el('div', { class: 'pipeline-test-row' });
  const testInput = el('input', {
    type: 'text',
    class: 'pipeline-test-input',
    id: 'translation-test-input',
    placeholder: '리브 미모 난리도 아니야',
    value: '리브 미모 난리도 아니야',
  });
  const testBtn = el('button', {
    class: 'btn btn-primary btn-sm',
    type: 'button',
    id: 'translation-test-btn',
  }, 'Test Chain');

  testRow.appendChild(testInput);
  testRow.appendChild(testBtn);
  testForm.appendChild(testRow);

  const timeline = el('div', { class: 'chain-timeline', id: 'translation-timeline' });
  testForm.appendChild(timeline);
  testCard.appendChild(testForm);

  testBtn.addEventListener('click', () => runChainTest(testInput.value, timeline, testBtn));

  root.appendChild(testCard);
}

function renderTranslationSection(root) {
  root.textContent = '';

  renderTranslationGeneralCard(root);

  const chain = getEffectiveFallbackChain();
  if (!state.selectedProvider || !chain.includes(state.selectedProvider)) {
    state.selectedProvider = chain[0] || 'gemini';
  }

  const pipeline = el('div', { class: 'pipeline-layout' });
  const master = el('div', { class: 'pipeline-master' });
  const detail = el('div', { class: 'pipeline-detail' });

  const refresh = () => {
    renderPipelineMaster(master, refresh);
    renderPipelineDetail(detail, refresh);
  };

  renderPipelineMaster(master, refresh);
  renderPipelineDetail(detail, refresh);

  pipeline.appendChild(master);
  pipeline.appendChild(detail);
  root.appendChild(pipeline);

  renderTranslationPrompt(root);
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
    const [data, configRows, transConfig] = await Promise.all([
      api('GET', '/api/settings'),
      api('GET', '/api/config').catch(() => []),
      api('GET', '/api/translation?health=1').catch(() => null),
    ]);
    state.entries = new Map(data.settings.map((e) => [e.key, e]));
    state.system = data.system || [];
    state.advancedRows = configRows.filter((c) => c.source === 'db');
    if (transConfig && transConfig.broken_keys) {
      state.brokenKeys = transConfig.broken_keys;
    }
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

export function initSettings() {
  $('#settings-save-btn')?.addEventListener('click', saveChanges);
  $('#settings-revert-btn')?.addEventListener('click', () => {
    state.edited.clear();
    updateSaveBar();
    renderSectionBody(state.active);
  });
  window.addEventListener('mouseup', () => {
    document.querySelectorAll('.pipeline-prov-item[draggable="true"], .model-chain-item[draggable="true"]').forEach((c) => { c.draggable = false; });
  });
}
