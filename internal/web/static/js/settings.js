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
  const providerSelect = $('#translation-provider-select');
  const geminiInput = $('#gemini-api-key');
  const groqInput = $('#groq-api-key');
  const geminiModelsInput = $('#gemini-models-input');
  const groqModelsInput = $('#groq-models-input');
  const promptArea = $('#translation-prompt');
  const concurrencyInput = $('#downloader-concurrency-input');
  if (!providerSelect) return;

  try {
    const data = await api('GET', '/api/translation');
    if (data) {
      providerSelect.value = data.provider || 'google';
      if (geminiInput && data.gemini_masked) {
        geminiInput.value = data.gemini_masked;
      }
      if (groqInput && data.groq_masked) {
        groqInput.value = data.groq_masked;
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
      if (data.default_prompt) {
        defaultPromptCached = data.default_prompt;
      }
      if (promptArea && data.prompt) {
        promptArea.value = data.prompt;
      }
      if (concurrencyInput && data.concurrency) {
        concurrencyInput.value = data.concurrency;
      }
      updateFallbackBadge(data.provider);
    }
  } catch (err) {
    console.error('Failed to load translation config:', err);
  }
}

function updateFallbackBadge(provider) {
  const badge = $('#fallback-chain-badge');
  if (!badge) return;
  switch (provider) {
    case 'gemini':
      badge.textContent = 'Chain: Gemini ➔ Groq ➔ Google Translate';
      break;
    case 'groq':
      badge.textContent = 'Chain: Groq ➔ Gemini ➔ Google Translate';
      break;
    default:
      badge.textContent = 'Chain: Google Translate';
      break;
  }
}

export async function saveTranslationConfig() {
  const btn = $('#translation-save-btn');
  const providerSelect = $('#translation-provider-select');
  const geminiInput = $('#gemini-api-key');
  const groqInput = $('#groq-api-key');
  const geminiModelsInput = $('#gemini-models-input');
  const groqModelsInput = $('#groq-models-input');
  const promptArea = $('#translation-prompt');

  const payload = {
    provider: providerSelect.value,
    gemini_api_key: geminiInput ? geminiInput.value : undefined,
    groq_api_key: groqInput ? groqInput.value : undefined,
    gemini_models: geminiModelsInput ? geminiModelsInput.value : undefined,
    groq_models: groqModelsInput ? groqModelsInput.value : undefined,
    prompt: promptArea ? promptArea.value : undefined,
  };

  try {
    if (btn) {
      btn.disabled = true;
      btn.textContent = 'Saving...';
    }
    await api('POST', '/api/translation', payload);
    updateFallbackBadge(providerSelect.value);
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
  const providerSelect = $('#translation-provider-select');
  const inputEl = $('#translation-test-input');
  const resultEl = $('#translation-test-result');
  const geminiInput = $('#gemini-api-key');
  const groqInput = $('#groq-api-key');
  const geminiModelsInput = $('#gemini-models-input');
  const groqModelsInput = $('#groq-models-input');
  const promptArea = $('#translation-prompt');

  const text = inputEl.value.trim() || '대박! 오늘 무대 진짜 레전드였어!';

  try {
    if (btn) {
      btn.disabled = true;
      btn.textContent = 'Testing...';
    }
    if (resultEl) {
      resultEl.textContent = 'Translating with ' + providerSelect.value + '...';
      resultEl.style.color = 'var(--text-color, #eee)';
    }

    const res = await api('POST', '/api/translation/test', {
      provider: providerSelect.value,
      text: text,
      target_lang: 'ru',
      gemini_api_key: geminiInput ? geminiInput.value : undefined,
      groq_api_key: groqInput ? groqInput.value : undefined,
      gemini_models: geminiModelsInput ? geminiModelsInput.value : undefined,
      groq_models: groqModelsInput ? groqModelsInput.value : undefined,
      prompt: promptArea ? promptArea.value : undefined,
    });

    if (res && res.success) {
      if (resultEl) {
        resultEl.innerHTML = `<strong>Result (${res.provider}):</strong> ${res.result}`;
        resultEl.style.color = 'var(--success-color, #4caf50)';
      }
    } else {
      if (resultEl) {
        resultEl.textContent = `Error: ${res ? res.error : 'Unknown error'}`;
        resultEl.style.color = 'var(--danger-color, #f44336)';
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

export async function saveConcurrency() {
  const btn = $('#save-concurrency-btn');
  const input = $('#downloader-concurrency-input');
  if (!input) return;
  const val = parseInt(input.value, 10);
  if (isNaN(val) || val < 1 || val > 20) {
    alert('Concurrency must be between 1 and 20');
    return;
  }
  try {
    if (btn) {
      btn.disabled = true;
      btn.textContent = 'Saving...';
    }
    await api('POST', '/api/translation', { concurrency: val });
    if (btn) {
      btn.textContent = 'Saved!';
      setTimeout(() => {
        btn.textContent = 'Save';
        btn.disabled = false;
      }, 1500);
    }
  } catch (err) {
    alert(`Failed to save concurrency: ${err.message}`);
    if (btn) {
      btn.disabled = false;
      btn.textContent = 'Save';
    }
  }
}

export function initSettings() {
  $('#clean-storage-btn')?.addEventListener('click', cleanStorage);
  $('#save-concurrency-btn')?.addEventListener('click', saveConcurrency);
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

  $('#translation-provider-select')?.addEventListener('change', (e) => {
    updateFallbackBadge(e.target.value);
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
