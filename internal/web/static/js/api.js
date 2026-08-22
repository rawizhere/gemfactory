export const $ = (sel) => document.querySelector(sel);
export const $$ = (sel) => document.querySelectorAll(sel);

export function el(tag, attrs, children) {
  const node = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v !== null && v !== undefined) node.setAttribute(k, v);
    }
  }
  if (children !== undefined && children !== null) {
    if (Array.isArray(children)) {
      children.forEach((c) => {
        if (c) node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
      });
    } else if (typeof children === 'string' || typeof children === 'number') {
      node.textContent = String(children);
    } else if (children instanceof Node) {
      node.appendChild(children);
    }
  }
  return node;
}

export function tableWrap(table) {
  const wrap = el('div', { class: 'tablewrap' });
  wrap.appendChild(table);
  return wrap;
}

export async function api(method, url, body) {
  const res = await fetch(url, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
    cache: 'no-store',
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
  if (res.status === 204 || res.headers.get('Content-Length') === '0') return null;
  return res.json();
}

export function showMsg(target, text, isOk = false, timeout = 4000) {
  if (!target) return;
  target.textContent = text;
  target.className = 'msg' + (isOk ? ' ok' : '');
  if (timeout > 0 && text) {
    setTimeout(() => {
      if (target.textContent === text) {
        target.textContent = '';
        target.className = 'msg';
      }
    }, timeout);
  }
}

export function exportToCSV(filename, rows, headers) {
  if (!rows || !rows.length) return;
  const headerKeys = Object.keys(headers);
  const headerLabels = Object.values(headers);

  const csvContent = [
    headerLabels.map((h) => `"${String(h).replace(/"/g, '""')}"`).join(','),
    ...rows.map((row) =>
      headerKeys
        .map((k) => `"${String(row[k] ?? '').replace(/"/g, '""')}"`)
        .join(',')
    ),
  ].join('\n');

  const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}
