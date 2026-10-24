// Protocol-agnostic debug panel. The server supplies a per-protocol
// spec; this file only renders it and sends the values back.
let debugDoc = null;

function setupDebug(d) {
  debugDoc = d;
  const spec = d.debug_spec;
  const unavailable = document.getElementById('dg-unavailable');
  const browser = document.getElementById('dg-browser');
  const server = document.getElementById('dg-server');
  unavailable.classList.add('hidden');
  browser.classList.add('hidden');
  server.classList.add('hidden');
  showResult('', '');
  if (!spec) {
    unavailable.textContent = 'No debugger registered for protocol ' + (d.protocol || 'http');
    unavailable.classList.remove('hidden');
    return;
  }
  if ((spec.mode || 'server') === 'browser') {
    browser.classList.remove('hidden');
    setupBrowserDebug(d);
  } else {
    server.classList.remove('hidden');
    setupServerDebug(d, spec);
  }
}

function setupBrowserDebug(d) {
  const spec = d.debug_spec || {};
  const sel = document.getElementById('dg-method');
  if (spec.actions && spec.actions.length) {
    sel.textContent = '';
    for (const a of spec.actions) {
      const o = document.createElement('option');
      o.value = a;
      o.textContent = a;
      sel.appendChild(o);
    }
  }
  sel.value = d.method;
  document.getElementById('dg-url').value = d.url;
  const box = document.getElementById('dg-params');
  box.textContent = '';
  for (const p of d.params || []) {
    const row = document.createElement('div');
    row.className = 'row';
    const lab = document.createElement('span');
    lab.textContent = '[' + p.in + '] ' + p.name;
    lab.style.minWidth = '140px';
    const inp = document.createElement('input');
    inp.type = 'text';
    inp.dataset.in = p.in;
    inp.dataset.name = p.name;
    inp.placeholder = p.desc || '';
    inp.value = p.mock || p.default || '';
    row.appendChild(lab);
    row.appendChild(inp);
    box.appendChild(row);
  }
}

function setupServerDebug(d, spec) {
  const target = document.getElementById('dg-target');
  target.value = d.url;
  target.disabled = !spec.allow_target_override;

  const action = document.getElementById('dg-action');
  action.textContent = '';
  const actions = (spec.actions && spec.actions.length) ? spec.actions : [d.method];
  for (const a of actions) {
    const o = document.createElement('option');
    o.value = a;
    o.textContent = a;
    action.appendChild(o);
  }
  action.value = d.method;

  const enc = document.getElementById('dg-encoding');
  enc.textContent = '';
  const encodings = (spec.encodings && spec.encodings.length) ? spec.encodings : ['json', 'text'];
  for (const e of encodings) {
    const o = document.createElement('option');
    o.value = e;
    o.textContent = e;
    enc.appendChild(o);
  }

  const connect = document.getElementById('dg-connect');
  connect.textContent = '';
  for (const f of spec.connect_fields || []) {
    const row = document.createElement('div');
    row.className = 'row';
    const lab = document.createElement('span');
    lab.textContent = f.label || f.name;
    lab.style.minWidth = '140px';
    const inp = fieldInput(f);
    row.appendChild(lab);
    row.appendChild(inp);
    connect.appendChild(row);
  }

  const box = document.getElementById('dg-s-params');
  box.textContent = '';
  const bodyIn = spec.body_in || 'payload';
  let bodyParam = null;
  for (const p of d.params || []) {
    if ((p.in || 'query') === bodyIn) {
      bodyParam = p;
      continue;
    }
    const row = document.createElement('div');
    row.className = 'row dg-param';
    const lab = document.createElement('span');
    lab.textContent = '[' + (p.in || 'query') + '] ' + p.name;
    lab.style.minWidth = '140px';
    const inp = paramInput(p);
    row.appendChild(lab);
    row.appendChild(inp);
    box.appendChild(row);
  }
  const body = document.getElementById('dg-body');
  body.value = bodyParam ? (bodyParam.mock || bodyParam.default || '') : '';
  body.placeholder = bodyParam ? (bodyParam.desc || 'message body') : 'message body';
  if (bodyParam && encodings.includes(bodyParam.type)) {
    enc.value = bodyParam.type;
  }
}

function fieldInput(f) {
  let el;
  if (f.type === 'select' && f.options && f.options.length) {
    el = document.createElement('select');
    for (const o of f.options) {
      const opt = document.createElement('option');
      opt.value = o;
      opt.textContent = o;
      el.appendChild(opt);
    }
    el.dataset.name = f.name;
    if (f.default) el.value = f.default;
    return el;
  }
  if (f.type === 'textarea') {
    el = document.createElement('textarea');
  } else if (f.type === 'bool' || f.type === 'checkbox') {
    el = document.createElement('input');
    el.type = 'checkbox';
    el.checked = f.default === 'true' || f.default === '1';
  } else {
    el = document.createElement('input');
    el.type = f.secret ? 'password' : (f.type === 'number' ? 'number' : 'text');
  }
  el.dataset.name = f.name;
  if (f.placeholder) el.placeholder = f.placeholder;
  if (f.default && el.type !== 'checkbox') el.value = f.default;
  return el;
}

function paramInput(p) {
  let el;
  if (p.type === 'bool' || p.type === 'boolean') {
    el = document.createElement('input');
    el.type = 'checkbox';
  } else if (p.type === 'int' || p.type === 'int64' || p.type === 'number' ||
             p.type === 'float' || p.type === 'float64' || p.type === 'double') {
    el = document.createElement('input');
    el.type = 'number';
  } else {
    el = document.createElement('input');
    el.type = 'text';
  }
  el.dataset.name = p.name;
  el.dataset.in = p.in || '';
  el.placeholder = p.desc || '';
  if (el.type !== 'checkbox') el.value = p.mock || p.default || '';
  return el;
}

function sendDebug() {
  if (!debugDoc || !debugDoc.debug_spec) return;
  if ((debugDoc.debug_spec.mode || 'server') === 'browser') {
    sendBrowserDebug();
  } else {
    sendServerDebug();
  }
}

async function sendBrowserDebug() {
  let url = document.getElementById('dg-url').value;
  const method = document.getElementById('dg-method').value;
  const query = [], headers = {}, body = {};
  document.querySelectorAll('#dg-params input').forEach(inp => {
    const v = inp.value.trim();
    if (!v) return;
    if (inp.dataset.in === 'query') query.push(encodeURIComponent(inp.dataset.name) + '=' + encodeURIComponent(v));
    else if (inp.dataset.in === 'header') headers[inp.dataset.name] = v;
    else if (inp.dataset.in === 'path') url = url.replace('{' + inp.dataset.name + '}', encodeURIComponent(v));
    else body[inp.dataset.name] = v;
  });
  if (query.length) url += (url.includes('?') ? '&' : '?') + query.join('&');
  const opts = { method, credentials: 'include', headers };
  if (method !== 'GET' && Object.keys(body).length) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  try {
    const res = await fetch(url, opts);
    showResult(res.status + ' ' + res.statusText, (await res.text()) || '');
  } catch (e) {
    showResult('request failed', String(e));
  }
}

async function sendServerDebug() {
  const params = [];
  document.querySelectorAll('#dg-s-params .dg-param').forEach(row => {
    const inp = row.querySelector('input');
    if (!inp) return;
    params.push({
      name: inp.dataset.name,
      in: inp.dataset.in,
      value: inp.type === 'checkbox' ? (inp.checked ? 'true' : 'false') : inp.value
    });
  });
  const options = {};
  document.querySelectorAll('#dg-connect .row').forEach(row => {
    const inp = row.querySelector('input,select,textarea');
    if (!inp) return;
    options[inp.dataset.name] = inp.type === 'checkbox' ? (inp.checked ? 'true' : 'false') : inp.value;
  });
  const encoding = document.getElementById('dg-encoding').value || 'json';
  const bodyBytes = encodeText(document.getElementById('dg-body').value, encoding);
  const headers = { 'X-Apidoc-Debug': '1' };
  if (appTokens[debugDoc.app]) headers['X-Apidoc-App-Token'] = appTokens[debugDoc.app];
  const payload = {
    id: debugDoc.id,
    method: document.getElementById('dg-action').value || debugDoc.method,
    url: document.getElementById('dg-target').value || debugDoc.url,
    params,
    body: bytesToB64(bodyBytes),
    encoding,
    headers: {},
    options
  };
  const { res, data } = await api('/api/debug/exec', {
    method: 'POST', headers, body: JSON.stringify(payload)
  });
  if (res.status !== 200) {
    showResult(String(res.status) + ' ' + res.statusText, (data && data.msg) || '');
    return;
  }
  renderDebugResponse(data.data);
}

function renderDebugResponse(r) {
  document.getElementById('res-status').textContent =
    (r.status || 'OK') + ' · ' + (r.duration_ms || 0) + ' ms';
  document.getElementById('res-body').textContent = renderBody(r);
  const extra = document.getElementById('res-extra');
  extra.textContent = '';

  const headers = r.headers || {};
  const hk = Object.keys(headers);
  if (hk.length) {
    extra.appendChild(sectionTitle('Headers'));
    const t = document.createElement('table');
    t.appendChild(tableHead(['Name', 'Value']));
    for (const k of hk) {
      const tr = document.createElement('tr');
      tr.appendChild(textCell(k));
      tr.appendChild(textCell(headers[k]));
      t.appendChild(tr);
    }
    extra.appendChild(t);
  }

  const events = r.events || [];
  if (events.length) {
    extra.appendChild(sectionTitle('Events'));
    const t = document.createElement('table');
    t.appendChild(tableHead(['Time', 'Direction', 'Name', 'Data']));
    for (const e of events) {
      const tr = document.createElement('tr');
      tr.appendChild(textCell(e.time || ''));
      tr.appendChild(textCell(e.direction || ''));
      tr.appendChild(textCell(e.name || ''));
      tr.appendChild(textCell(e.data || ''));
      t.appendChild(tr);
    }
    extra.appendChild(t);
  }

  if (r.truncated) {
    const p = document.createElement('p');
    p.className = 'meta';
    p.textContent = 'Response truncated by the server.';
    extra.appendChild(p);
  }
}

function renderBody(r) {
  const bytes = b64ToBytes(r.body || '');
  const spec = (debugDoc && debugDoc.debug_spec) || {};
  const view = (spec.response_view || 'auto').toLowerCase();
  const enc = (r.encoding || '').toLowerCase();
  if (view === 'hex' || enc === 'hex' || enc === 'binary') return hexDump(bytes);
  const text = new TextDecoder('utf-8', { fatal: false }).decode(bytes);
  if (view === 'json' || view === 'auto' || enc === 'json') {
    try { return JSON.stringify(JSON.parse(text), null, 2); } catch (e) {}
  }
  return text;
}

function sectionTitle(s) {
  const h = document.createElement('h3');
  h.textContent = s;
  return h;
}

function tableHead(names) {
  const tr = document.createElement('tr');
  for (const n of names) {
    const th = document.createElement('th');
    th.textContent = n;
    tr.appendChild(th);
  }
  return tr;
}

function textCell(s) {
  const td = document.createElement('td');
  td.textContent = s == null ? '' : String(s);
  return td;
}

function showResult(status, body) {
  document.getElementById('res-status').textContent = status || '';
  document.getElementById('res-body').textContent = body || '';
  const extra = document.getElementById('res-extra');
  if (extra) extra.textContent = '';
}

function encodeText(s, enc) {
  if (enc === 'base64') return b64ToBytes(s.trim());
  if (enc === 'hex') {
    const clean = (s || '').replace(/\s+/g, '');
    const out = new Uint8Array(Math.floor(clean.length / 2));
    for (let i = 0; i < out.length; i++) {
      const v = parseInt(clean.substr(i * 2, 2), 16);
      out[i] = isNaN(v) ? 0 : v;
    }
    return out;
  }
  return new TextEncoder().encode(s || '');
}

function bytesToB64(bytes) {
  let bin = '';
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    bin += String.fromCharCode.apply(null, bytes.subarray(i, i + chunk));
  }
  return btoa(bin);
}

function b64ToBytes(s) {
  const bin = atob(s || '');
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function hexDump(bytes) {
  const lines = [];
  for (let i = 0; i < bytes.length; i += 16) {
    const chunk = bytes.subarray(i, i + 16);
    const hex = Array.from(chunk).map(b => b.toString(16).padStart(2, '0')).join(' ');
    lines.push(i.toString(16).padStart(8, '0') + '  ' + hex);
  }
  return lines.join('\n');
}
