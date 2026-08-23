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

let defaultPromptCached = '';
let defaultGeminiModelsCached = '';
let defaultGroqModelsCached = '';

export async function loadTranslationConfig() {
  const geminiInput = $('#gemini-api-key');
  const groqInput = $('#groq-api-key');
  const opencodeInput = $('#opencode-api-key');
  const opencodeModelsInput = $('#opencode-models-input');
  const nvidiaInput = $('#nvidia-api-key');
  const nvidiaModelsInput = $('#nvidia-models-input');
  const geminiModelsInput = $('#gemini-models-input');
  const groqModelsInput = $('#groq-models-input');
  const promptArea = $('#translation-prompt');
  const sourcePrefInput = $('#source-pref-ru-input');
  const fallbackOrderInput = $('#fallback-order-input');
  const concurrencyInput = $('#downloader-concurrency-input');
  const clipCRFInput = $('#clip-crf-input');
  const subsCRFInput = $('#subs-crf-input');
  const clipPresetSelect = $('#clip-preset-select');
  const clipAudioBitrateSelect = $('#clip-audio-bitrate-select');
  if (!$('#fallback-chain-badge')) return;

  try {
    const data = await api('GET', '/api/translation');
    if (data) {
      if (geminiInput && data.gemini_masked) {
        geminiInput.value = data.gemini_masked;
      }
      if (groqInput && data.groq_masked) {
        groqInput.value = data.groq_masked;
      }
      if (opencodeInput && data.opencode_masked) {
        opencodeInput.value = data.opencode_masked;
      }
      if (data.default_gemini_models) {
        defaultGeminiModelsCached = data.default_gemini_models;
      }
      if (data.default_groq_models) {
        defaultGroqModelsCached = data.default_groq_models;
      }
      if (geminiModelsInput && data.gemini_models) {
        geminiModelsInput.value = data.gemini_models;
      }
      if (groqModelsInput && data.groq_models) {
        groqModelsInput.value = data.groq_models;
      }
      if (opencodeModelsInput && data.opencode_models) {
        opencodeModelsInput.value = data.opencode_models;
      }
      if (nvidiaInput && data.nvidia_masked) {
        nvidiaInput.value = data.nvidia_masked;
      }
      if (nvidiaModelsInput && data.nvidia_models) {
        nvidiaModelsInput.value = data.nvidia_models;
      }
      if (sourcePrefInput && data.source_pref_ru) {
        sourcePrefInput.value = data.source_pref_ru;
      }
      if (fallbackOrderInput) {
        fallbackOrderInput.value = data.fallback_order || 'gemini, nvidia, groq, opencode';
      }
      if (data.default_prompt) {
        defaultPromptCached = data.default_prompt;
      }
      if (promptArea && data.prompt) {
        promptArea.value = data.prompt;
      }
      if (concurrencyInput && data.concurrency) {
        concurrencyInput.value = data.concurrency;
      }
      if (clipCRFInput && data.clip_crf) {
        clipCRFInput.value = data.clip_crf;
      }
      if (subsCRFInput && data.subs_crf) {
        subsCRFInput.value = data.subs_crf;
      }
      if (clipPresetSelect && data.clip_preset) {
        clipPresetSelect.value = data.clip_preset;
      }
      if (clipAudioBitrateSelect && data.clip_audio_bitrate) {
        clipAudioBitrateSelect.value = data.clip_audio_bitrate;
      }
      const clipDeleteStatusSelect = $('#clip-delete-status-select');
      if (clipDeleteStatusSelect && typeof data.clip_delete_status === 'boolean') {
        clipDeleteStatusSelect.value = data.clip_delete_status ? 'true' : 'false';
      }
      updateFallbackBadge(data.chain);
    }
  } catch (err) {
    console.error('Failed to load translation config:', err);
  }
}

function updateFallbackBadge(chain) {
  const badge = $('#fallback-chain-badge');
  if (!badge) return;
  if (!chain) return;
  badge.textContent = chain.startsWith('Chain:') ? chain : 'Chain: ' + chain;
}

export async function saveTranslationConfig() {
  const btn = $('#translation-save-btn');
  const geminiInput = $('#gemini-api-key');
  const groqInput = $('#groq-api-key');
  const nvidiaInput = $('#nvidia-api-key');
  const nvidiaModelsInput = $('#nvidia-models-input');
  const opencodeInput = $('#opencode-api-key');
  const opencodeModelsInput = $('#opencode-models-input');
  const geminiModelsInput = $('#gemini-models-input');
  const groqModelsInput = $('#groq-models-input');
  const promptArea = $('#translation-prompt');
  const sourcePrefInput = $('#source-pref-ru-input');
  const fallbackOrderInput = $('#fallback-order-input');

  const payload = {
    gemini_api_key: geminiInput ? geminiInput.value : undefined,
    groq_api_key: groqInput ? groqInput.value : undefined,
    opencode_api_key: opencodeInput ? opencodeInput.value : undefined,
    nvidia_api_key: nvidiaInput ? nvidiaInput.value : undefined,
    gemini_models: geminiModelsInput ? geminiModelsInput.value : undefined,
    groq_models: groqModelsInput ? groqModelsInput.value : undefined,
    opencode_models: opencodeModelsInput ? opencodeModelsInput.value : undefined,
    nvidia_models: nvidiaModelsInput ? nvidiaModelsInput.value : undefined,
    source_pref_ru: sourcePrefInput ? sourcePrefInput.value : undefined,
    fallback_order: fallbackOrderInput ? fallbackOrderInput.value : undefined,
    prompt: promptArea ? promptArea.value : undefined,
  };

  try {
    if (btn) {
      btn.disabled = true;
      btn.textContent = 'Saving...';
    }
    await api('POST', '/api/translation', payload);
    try {
      const fresh = await api('GET', '/api/translation');
      updateFallbackBadge(fresh && fresh.chain);
    } catch (_) {}
    if (btn) {
      btn.textContent = 'Saved!';
      setTimeout(() => {
        btn.textContent = 'Save Translation Settings';
        btn.disabled = false;
      }, 1500);
    }
  } catch (err) {
    alert(`Failed to save translation config: ${err.message}`);
    if (btn) {
      btn.disabled = false;
      btn.textContent = 'Save Translation Settings';
    }
  }
}

export async function testTranslation() {
  const btn = $('#translation-test-btn');
  const inputEl = $('#translation-test-input');
  const resultEl = $('#translation-test-result');
  const geminiInput = $('#gemini-api-key');
  const groqInput = $('#groq-api-key');
  const opencodeInput = $('#opencode-api-key');
  const opencodeModelsInput = $('#opencode-models-input');
  const nvidiaInput = $('#nvidia-api-key');
  const nvidiaModelsInput = $('#nvidia-models-input');
  const geminiModelsInput = $('#gemini-models-input');
  const groqModelsInput = $('#groq-models-input');
  const promptArea = $('#translation-prompt');

  const text = inputEl.value.trim() || '리브 미모 난리도 아니야';

  if (btn) {
    btn.disabled = true;
    btn.textContent = 'Testing...';
  }
  if (resultEl) {
    resultEl.textContent = 'Probing fallback chain...';
    resultEl.style.color = 'var(--text-color, #eee)';
  }

  const appendRow = (e) => {
    if (!resultEl) return;
    const row = document.createElement('div');
    row.style.marginBottom = '0.35rem';
    row.style.wordBreak = 'break-word';
    const name = e.provider === 'google' ? 'Google Translate' : `${e.provider}/${e.model || '?'}`;
    if (e.ok) {
      row.style.color = 'var(--success-color, #4caf50)';
      row.innerHTML = `<b>${name}:</b> ${e.result ?? ''}`;
    } else {
      row.style.color = 'var(--danger-color, #f44336)';
      row.innerHTML = `<b>${name}:</b> FAIL — ${e.error ?? 'unknown error'}${e.result ? `<br>Raw: ${e.result}` : ''}`;
    }
    resultEl.appendChild(row);
  };

  try {
    const res = await fetch('/api/translation/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        text: text,
        target_lang: 'ru',
        gemini_api_key: geminiInput ? geminiInput.value : undefined,
        groq_api_key: groqInput ? groqInput.value : undefined,
        opencode_api_key: opencodeInput ? opencodeInput.value : undefined,
        nvidia_api_key: nvidiaInput ? nvidiaInput.value : undefined,
        gemini_models: geminiModelsInput ? geminiModelsInput.value : undefined,
        groq_models: groqModelsInput ? groqModelsInput.value : undefined,
        opencode_models: opencodeModelsInput ? opencodeModelsInput.value : undefined,
        nvidia_models: nvidiaModelsInput ? nvidiaModelsInput.value : undefined,
        prompt: promptArea ? promptArea.value : undefined,
      }),
    });
    if (!res.ok) {
      throw new Error((await res.text()) || res.statusText);
    }
    if (resultEl) {
      resultEl.textContent = '';
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let done = false;
    while (!done) {
      const { value, done: streamDone } = await reader.read();
      if (streamDone) break;
      buffer += decoder.decode(value, { stream: true });
      let idx;
      while ((idx = buffer.indexOf('\n')) >= 0) {
        const line = buffer.slice(0, idx).trim();
        buffer = buffer.slice(idx + 1);
        if (!line) continue;
        const event = JSON.parse(line);
        if (event.type === 'start' && resultEl && !resultEl.hasChildNodes()) {
          resultEl.textContent = '';
        }
        if (event.type === 'result') {
          appendRow(event);
        }
        if (event.type === 'done') {
          done = true;
          if (!resultEl?.hasChildNodes() && !event.success) {
            resultEl.textContent = 'No provider succeeded';
            resultEl.style.color = 'var(--danger-color, #f44336)';
          }
        }
      }
    }
  } catch (err) {
    if (resultEl) {
      resultEl.textContent = `Error: ${err.message}`;
      resultEl.style.color = 'var(--danger-color, #f44336)';
    }
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = 'Test Translation';
    }
  }
}

export async function loadConfig() {
  try {
    const rows = await api('GET', '/api/config');
    renderConfigTable(rows.filter((c) => c.source === 'db'), true);
    renderConfigTable(rows.filter((c) => c.source === 'env'), false);
    await loadStorageUsage();
    await loadTranslationConfig();
  } catch (err) {
    console.error('Failed to load config:', err);
  }
}

export async function saveDownloaderSettings() {
  const btn = $('#save-downloader-btn');
  const input = $('#downloader-concurrency-input');
  const clipCRFInput = $('#clip-crf-input');
  const subsCRFInput = $('#subs-crf-input');
  const clipPresetSelect = $('#clip-preset-select');
  const clipAudioBitrateSelect = $('#clip-audio-bitrate-select');
  const clipDeleteStatusSelect = $('#clip-delete-status-select');

  const payload = {};
  if (input) {
    const val = parseInt(input.value, 10);
    if (isNaN(val) || val < 1 || val > 20) {
      alert('Concurrency must be between 1 and 20');
      return;
    }
    payload.concurrency = val;
  }
  if (clipCRFInput && clipCRFInput.value.trim()) {
    payload.clip_crf = clipCRFInput.value.trim();
  }
  if (subsCRFInput && subsCRFInput.value.trim()) {
    payload.subs_crf = subsCRFInput.value.trim();
  }
  if (clipPresetSelect && clipPresetSelect.value) {
    payload.clip_preset = clipPresetSelect.value;
  }
  if (clipAudioBitrateSelect && clipAudioBitrateSelect.value) {
    payload.clip_audio_bitrate = clipAudioBitrateSelect.value;
  }
  if (clipDeleteStatusSelect && clipDeleteStatusSelect.value) {
    payload.clip_delete_status = clipDeleteStatusSelect.value === 'true';
  }

  try {
    if (btn) {
      btn.disabled = true;
      btn.textContent = 'Saving...';
    }
    await api('POST', '/api/translation', payload);
    if (btn) {
      btn.textContent = 'Saved!';
      setTimeout(() => {
        btn.textContent = 'Save Downloader Settings';
        btn.disabled = false;
      }, 1500);
    }
  } catch (err) {
    alert(`Failed to save downloader settings: ${err.message}`);
    if (btn) {
      btn.disabled = false;
      btn.textContent = 'Save Downloader Settings';
    }
  }
}

export function initSettings() {
  $('#clean-storage-btn')?.addEventListener('click', cleanStorage);
  $('#save-downloader-btn')?.addEventListener('click', saveDownloaderSettings);
  $('#translation-save-btn')?.addEventListener('click', saveTranslationConfig);
  $('#translation-test-btn')?.addEventListener('click', testTranslation);

  $('#translation-prompt-reset')?.addEventListener('click', () => {
    const promptArea = $('#translation-prompt');
    if (promptArea && defaultPromptCached) {
      promptArea.value = defaultPromptCached;
    }
  });

  $('#gemini-models-reset')?.addEventListener('click', () => {
    const input = $('#gemini-models-input');
    if (input && defaultGeminiModelsCached) {
      input.value = defaultGeminiModelsCached;
    }
  });

  $('#groq-models-reset')?.addEventListener('click', () => {
    const input = $('#groq-models-input');
    if (input && defaultGroqModelsCached) {
      input.value = defaultGroqModelsCached;
    }
  });

  $('#gemini-key-toggle')?.addEventListener('click', () => {
    const input = $('#gemini-api-key');
    const btn = $('#gemini-key-toggle');
    if (input && btn) {
      if (input.type === 'password') {
        input.type = 'text';
        btn.textContent = 'Hide';
      } else {
        input.type = 'password';
        btn.textContent = 'Show';
      }
    }
  });

  $('#opencode-models-reset')?.addEventListener('click', () => {
    const input = $('#opencode-models-input');
    fetch('/api/translation').then(r=>r.json()).then(d=>{
      if (input && d.default_opencode_models) input.value = d.default_opencode_models;
    }).catch(()=>{});
  });

  $('#opencode-key-toggle')?.addEventListener('click', () => {
    const input = $('#opencode-api-key');
    const btn = $('#opencode-key-toggle');
    if (input && btn) {
      if (input.type === 'password') {
        input.type = 'text';
        btn.textContent = 'Hide';
      } else {
        input.type = 'password';
        btn.textContent = 'Show';
      }
    }
  });

  $('#nvidia-models-reset')?.addEventListener('click', () => {
    const input = $('#nvidia-models-input');
    fetch('/api/translation').then(r=>r.json()).then(d=>{
      if (input && d.default_nvidia_models) input.value = d.default_nvidia_models;
    }).catch(()=>{});
  });

  $('#nvidia-key-toggle')?.addEventListener('click', () => {
    const input = $('#nvidia-api-key');
    const btn = $('#nvidia-key-toggle');
    if (input && btn) {
      if (input.type === 'password') {
        input.type = 'text';
        btn.textContent = 'Hide';
      } else {
        input.type = 'password';
        btn.textContent = 'Show';
      }
    }
  });

  $('#groq-key-toggle')?.addEventListener('click', () => {
    const input = $('#groq-api-key');
    const btn = $('#groq-key-toggle');
    if (input && btn) {
      if (input.type === 'password') {
        input.type = 'text';
        btn.textContent = 'Hide';
      } else {
        input.type = 'password';
        btn.textContent = 'Show';
      }
    }
  });
}
