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

let toastHost = null;

const hasPopover = 'popover' in HTMLElement.prototype;

export function toast(message, kind = 'info', timeout = 3500) {
  if (!toastHost) {
    toastHost = el('div', {
      class: 'toast-host',
      role: 'region',
      'aria-label': 'Notifications',
      'aria-live': 'polite',
      popover: hasPopover ? 'manual' : null,
    });
    document.body.appendChild(toastHost);
  }
  // Re-showing moves the host to the top of the top layer, above any modal opened since.
  if (hasPopover) {
    if (toastHost.matches(':popover-open')) toastHost.hidePopover();
    toastHost.showPopover();
  }
  const close = el('button', { type: 'button', class: 'toast-close', 'aria-label': 'Dismiss notification' }, '×');
  const t = el('div', { class: `toast toast-${kind}` }, [el('span', null, message), close]);
  const dismiss = () => {
    t.classList.remove('show');
    setTimeout(() => t.remove(), 300);
  };
  let timer = setTimeout(dismiss, timeout);
  const pause = () => clearTimeout(timer);
  const resume = () => {
    if (!t.matches(':hover, :focus-within')) timer = setTimeout(dismiss, timeout);
  };
  t.addEventListener('pointerenter', pause);
  t.addEventListener('focusin', pause);
  t.addEventListener('pointerleave', resume);
  t.addEventListener('focusout', (e) => { if (!t.contains(e.relatedTarget)) resume(); });
  close.addEventListener('click', () => { pause(); dismiss(); });
  toastHost.appendChild(t);
  requestAnimationFrame(() => t.classList.add('show'));
}

export function confirmDialog(message) {
  return new Promise((resolve) => {
    const dlg = el('dialog', { class: 'confirm-dialog', 'aria-labelledby': 'confirm-text' });
    const text = el('p', { class: 'confirm-text', id: 'confirm-text' }, message);
    const cancel = el('button', { class: 'btn btn-ghost btn-sm', type: 'submit', value: 'cancel' }, 'Cancel');
    const ok = el('button', { class: 'btn btn-danger btn-sm', type: 'submit', value: 'ok' }, 'Confirm');
    dlg.addEventListener('close', () => { dlg.remove(); resolve(dlg.returnValue === 'ok'); });
    // method="dialog" closes the dialog with the clicked button's value as returnValue.
    const form = el('form', { method: 'dialog', class: 'confirm-box' }, [text, el('div', { class: 'confirm-actions' }, [cancel, ok])]);
    dlg.appendChild(form);
    document.body.appendChild(dlg);
    dlg.showModal();
  });
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

  // BOM so Excel reads UTF-8 (Korean names) correctly.
  const blob = new Blob(['\uFEFF' + csvContent], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  // Revoking synchronously can abort the download in some browsers.
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
