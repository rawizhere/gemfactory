import { $, el, tableWrap, api, showMsg } from './api.js';

function cookieHealthBadge(c) {
  let cls = 'badge on';
  let label = 'valid';

  if (c.health === 'expired') {
    cls = 'badge expired';
    label = 'expired';
  } else if (c.health === 'expiring_soon') {
    cls = 'badge expiring';
    label = `expires in ${c.expires_in_days}d`;
  } else if (c.health === 'valid' && c.expires_in_days > 0) {
    cls = 'badge on';
    label = `valid (${c.expires_in_days}d left)`;
  } else if (c.health === 'session') {
    cls = 'badge';
    label = 'session';
  } else if (c.health === 'empty') {
    cls = 'badge expired';
    label = 'no cookies';
  }

  return el('span', { class: cls }, label);
}

export async function loadCookies() {
  const box = $('#cookies-table');
  try {
    const rows = await api('GET', '/api/cookies');
    box.textContent = '';

    if (!rows || !rows.length) {
      box.appendChild(el('p', { class: 'empty' }, 'No cookies stored yet. Paste a Netscape export above.'));
      return;
    }

    const table = el('table');
    const thead = el('thead');
    const headRow = el('tr');
    ['Domain', 'Count', 'Status', 'Updated', ''].forEach((title) => {
      headRow.appendChild(el('th', null, title));
    });
    thead.appendChild(headRow);
    table.appendChild(thead);

    const tbody = el('tbody');
    rows.forEach((c) => {
      const tr = el('tr');

      // Domain
      const tdDomain = el('td');
      tdDomain.appendChild(el('code', { class: 'mono' }, c.domain));
      tr.appendChild(tdDomain);

      // Cookie count
      const tdCount = el('td', { class: 'num' }, `${c.cookie_count || 0} cookies`);
      tr.appendChild(tdCount);

      // Health status badge
      const tdHealth = el('td');
      tdHealth.appendChild(cookieHealthBadge(c));
      tr.appendChild(tdHealth);

      // Updated date
      const dateStr = (c.updated_at || '').slice(0, 19);
      tr.appendChild(el('td', { class: 'date' }, dateStr));

      // Actions
      const tdAct = el('td', { style: 'text-align:right; white-space:nowrap;' });

      const replaceBtn = el('button', { class: 'btn btn-ghost btn-sm', type: 'button' }, 'Replace');
      replaceBtn.addEventListener('click', () => {
        const textarea = $('#cookie-add textarea[name=content]');
        textarea.value = '';
        textarea.placeholder = `# Paste new cookies for ${c.domain}...`;
        textarea.focus();
      });
      tdAct.appendChild(replaceBtn);
      tdAct.appendChild(document.createTextNode(' '));

      const delBtn = el('button', { class: 'btn btn-danger btn-sm', type: 'button' }, 'Delete');
      delBtn.addEventListener('click', async () => {
        if (!confirm(`Delete cookies for "${c.domain}"?`)) return;
        try {
          await api('DELETE', `/api/cookies/${encodeURIComponent(c.domain)}`);
          await loadCookies();
        } catch (err) {
          alert(err.message);
        }
      });
      tdAct.appendChild(delBtn);
      tr.appendChild(tdAct);

      tbody.appendChild(tr);
    });

    table.appendChild(tbody);
    box.appendChild(tableWrap(table));
  } catch (err) {
    box.textContent = '';
    box.appendChild(el('p', { class: 'msg' }, `Failed to load cookies: ${err.message}`));
  }
}

export function initCookies() {
  $('#cookie-add').addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const form = ev.target;
    const msg = form.querySelector('.msg');
    const content = form.content.value.trim();

    if (!content) {
      showMsg(msg, 'Please paste Netscape cookies content.', false);
      return;
    }

    try {
      const res = await api('PUT', '/api/cookies/_', { content });
      form.reset();
      const statusText = res && res.domains
        ? `Saved cookies for: ${res.domains.join(', ')}`
        : 'Cookies saved successfully.';
      showMsg(msg, statusText, true);
      await loadCookies();
    } catch (err) {
      showMsg(msg, err.message, false);
    }
  });
}
