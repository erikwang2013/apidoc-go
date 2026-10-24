// Protocol-agnostic debug panel. The server supplies a per-protocol
// spec; this file only renders it and sends the values back.
// Response rendering lives in debug-format.js; the server serves both
// files as one script.
let debugDoc = null;
let stream = null; // {id, src} while a streaming session is open
const maxDebugFile = 256 * 1024;

function setupDebug(d) {
  if (stream) closeStream();
  debugDoc = d;
  const spec = d.debug_spec;
  const unavailable = document.getElementById('dg-unavailable');
  const browser = document.getElementById('dg-browser');
  const server = document.getElementById('dg-server');
  unavailable.classList.add('hidden');
  browser.classList.add('hidden');
  server.classList.add('hidden');
  showResult('', '');
  const send = document.getElementById('dg-send');
  if (send) send.classList.toggle('hidden', !spec || !!spec.streaming);
  if (!spec) {
    setupSnippets({});
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
    row.appendChild(lab);
    row.appendChild(paramInput(p));
    box.appendChild(row);
  }
  setupSnippets(spec);
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

  const streaming = !!spec.streaming;
  document.getElementById('dg-stream').classList.toggle('hidden', !streaming);
  document.getElementById('dg-send').classList.toggle('hidden', streaming);
  if (!streaming) closeStreamUI();
  document.getElementById('dg-transcript').textContent = '';
  setupSnippets(spec);
}

function truthy(v) { return v === 'true' || v === '1'; }

function fieldInput(f) {
  if (f.type === 'file') return fileField(f);
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
    el.dataset.required = f.required ? '1' : '';
    return el;
  }
  if (f.type === 'textarea') {
    el = document.createElement('textarea');
  } else if (f.type === 'bool' || f.type === 'checkbox') {
    el = document.createElement('input');
    el.type = 'checkbox';
    el.checked = truthy(f.default);
  } else {
    el = document.createElement('input');
    el.type = f.secret ? 'password' : (f.type === 'number' ? 'number' : 'text');
  }
  el.dataset.name = f.name;
  el.dataset.required = f.required ? '1' : '';
  if (f.placeholder) el.placeholder = f.placeholder;
  if (f.default && el.type !== 'checkbox') el.value = f.default;
  return el;
}

// File fields send base64 through options; nothing is stored locally.
function fileField(f) {
  const wrap = document.createElement('div');
  wrap.className = 'dg-file';
  const el = document.createElement('input');
  el.type = 'file';
  el.dataset.name = f.name;
  el.dataset.required = f.required ? '1' : '';
  el.dataset.file = '1';
  const status = document.createElement('span');
  status.className = 'meta';
  el.onchange = () => readFileOption(el, status);
  wrap.appendChild(el);
  wrap.appendChild(status);
  return wrap;
}

function readFileOption(inp, status) {
  inp.dataset.b64 = '';
  status.textContent = '';
  const file = inp.files && inp.files[0];
  if (!file) return;
  if (file.size > maxDebugFile) {
    status.textContent = 'file too large (max 256 KiB)';
    return;
  }
  const reader = new FileReader();
  reader.onload = () => {
    inp.dataset.b64 = bytesToB64(new Uint8Array(reader.result));
    status.textContent = file.name + ' (' + file.size + ' bytes)';
  };
  reader.onerror = () => { status.textContent = 'file read failed'; };
  reader.readAsArrayBuffer(file);
}

function paramInput(p) {
  let el;
  if (p.type === 'bool' || p.type === 'boolean') {
    el = document.createElement('input');
    el.type = 'checkbox';
    el.checked = truthy(p.mock || p.default);
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
  el.dataset.required = p.required ? '1' : '';
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
    const v = inp.type === 'checkbox' ? (inp.checked ? 'true' : 'false') : inp.value.trim();
    if (!v && !inp.dataset.required) return;
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

function serverDebugPayload() {
  const params = [];
  document.querySelectorAll('#dg-s-params .dg-param').forEach(row => {
    const inp = row.querySelector('input');
    if (!inp) return;
    const value = inp.type === 'checkbox' ? (inp.checked ? 'true' : 'false') : inp.value;
    if (value === '' && !inp.dataset.required) return;
    params.push({ name: inp.dataset.name, in: inp.dataset.in, value });
  });
  const options = {};
  document.querySelectorAll('#dg-connect .row').forEach(row => {
    const inp = row.querySelector('input,select,textarea');
    if (!inp) return;
    let value;
    if (inp.dataset.file) value = inp.dataset.b64 || '';
    else value = inp.type === 'checkbox' ? (inp.checked ? 'true' : 'false') : inp.value;
    if (value === '' && !inp.dataset.required) return;
    options[inp.dataset.name] = value;
  });
  const encoding = document.getElementById('dg-encoding').value || 'json';
  return {
    id: debugDoc.id,
    method: document.getElementById('dg-action').value || debugDoc.method,
    url: document.getElementById('dg-target').value || debugDoc.url,
    params,
    body: bytesToB64(encodeText(document.getElementById('dg-body').value, encoding)),
    encoding,
    headers: {},
    options
  };
}

function debugHeaders() {
  const headers = { 'X-Apidoc-Debug': '1' };
  if (appTokens[debugDoc.app]) headers['X-Apidoc-App-Token'] = appTokens[debugDoc.app];
  return headers;
}

async function sendServerDebug() {
  let res, data;
  try {
    ({ res, data } = await api('/api/debug/exec', {
      method: 'POST', headers: debugHeaders(), body: JSON.stringify(serverDebugPayload())
    }));
  } catch (e) {
    showResult('request failed', String(e));
    return;
  }
  if (res.status !== 200) {
    showResult(String(res.status) + ' ' + res.statusText, (data && data.msg) || '');
    return;
  }
  renderDebugResponse(data.data);
}

function streamStatus(msg) {
  document.getElementById('dg-session').textContent = msg;
}

function appendTranscript(ev) {
  const box = document.getElementById('dg-transcript');
  const line = '[' + (ev.time || '') + '] ' + (ev.direction || '') +
    (ev.name ? ' ' + ev.name : '') + (ev.data ? ': ' + ev.data : '');
  box.textContent += (box.textContent ? '\n' : '') + line;
  box.scrollTop = box.scrollHeight;
}

async function connectStream() {
  if (stream) return stream.id;
  let res, data;
  try {
    ({ res, data } = await api('/api/debug/open', {
      method: 'POST', headers: debugHeaders(), body: JSON.stringify(serverDebugPayload())
    }));
  } catch (e) {
    streamStatus('open failed: ' + e);
    return null;
  }
  if (res.status !== 200) {
    streamStatus('open failed: ' + ((data && data.msg) || res.status));
    return null;
  }
  const id = data.data.session_id;
  const token = appTokens[debugDoc.app];
  const src = new EventSource(base + '/api/debug/events?session_id=' +
    encodeURIComponent(id) + (token ? '&app_token=' + encodeURIComponent(token) : ''));
  src.onmessage = e => {
    try { appendTranscript(JSON.parse(e.data)); } catch (_) { appendTranscript({ data: e.data }); }
  };
  src.addEventListener('end', () => { streamStatus('session ended'); closeStreamUI(); });
  src.onerror = () => {
    if (!stream) return;
    closeStreamUI();
    streamStatus('stream disconnected; connect again to continue');
  };
  stream = { id, src };
  document.getElementById('dg-transcript').textContent = '';
  streamStatus('connected');
  return id;
}

async function sendStreamMessage() {
  const id = (stream && stream.id) || await connectStream();
  if (!id) return;
  const encoding = document.getElementById('dg-encoding').value || 'json';
  const body = bytesToB64(encodeText(document.getElementById('dg-body').value, encoding));
  let res, data;
  try {
    ({ res, data } = await api('/api/debug/send', {
      method: 'POST', headers: debugHeaders(),
      body: JSON.stringify({ session_id: id, body, encoding })
    }));
  } catch (e) {
    streamStatus('send failed: ' + e);
    return;
  }
  if (res.status !== 200) {
    streamStatus('send failed: ' + ((data && data.msg) || res.status));
    return;
  }
  streamStatus('sent');
}

async function closeStream() {
  if (!stream) return;
  const id = stream.id;
  closeStreamUI();
  streamStatus('session closed');
  try {
    await api('/api/debug/close', {
      method: 'POST', headers: debugHeaders(), body: JSON.stringify({ session_id: id })
    });
  } catch (e) {}
}

function closeStreamUI() {
  if (!stream) return;
  stream.src.close();
  stream = null;
}

// Spec.Snippets: one tab per language, copy with a clipboard fallback.
function setupSnippets(spec) {
  const box = document.getElementById('dg-snippets');
  if (!box) return;
  box.textContent = '';
  const snippets = (spec && spec.snippets) || {};
  const langs = Object.keys(snippets);
  if (!langs.length) return;
  const details = document.createElement('details');
  const summary = document.createElement('summary');
  summary.textContent = 'Code snippets';
  details.appendChild(summary);
  const tabs = document.createElement('div');
  tabs.className = 'row';
  const pre = document.createElement('pre');
  const status = document.createElement('span');
  status.className = 'meta';
  let active = langs[0];
  const show = lang => {
    active = lang;
    pre.textContent = fillSnippet(snippets[lang]);
    status.textContent = '';
    for (const b of Array.from(tabs.children)) {
      b.classList.toggle('on', b.dataset.lang === lang);
    }
  };
  for (const lang of langs) {
    const b = document.createElement('button');
    b.type = 'button';
    b.textContent = lang;
    b.dataset.lang = lang;
    b.onclick = () => show(lang);
    tabs.appendChild(b);
  }
  const copy = document.createElement('button');
  copy.type = 'button';
  copy.textContent = 'Copy';
  copy.onclick = () => copyText(fillSnippet(snippets[active]), status);
  tabs.appendChild(copy);
  details.appendChild(tabs);
  details.appendChild(pre);
  details.appendChild(status);
  box.appendChild(details);
  show(active);
}

function fillSnippet(tmpl) {
  const d = debugDoc || {};
  const method = valueOf('dg-action') || valueOf('dg-method') || d.method || '';
  const url = valueOf('dg-target') || valueOf('dg-url') || d.url || '';
  const body = valueOf('dg-body');
  return String(tmpl).split('{{method}}').join(method)
    .split('{{url}}').join(url).split('{{body}}').join(body);
}

function valueOf(id) {
  const el = document.getElementById(id);
  return el ? el.value : '';
}

function copyText(text, status) {
  const done = ok => { status.textContent = ok ? 'copied' : 'copy failed'; };
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(() => done(true), () => fallbackCopy(text, done));
    return;
  }
  fallbackCopy(text, done);
}

function fallbackCopy(text, done) {
  const area = document.createElement('textarea');
  area.value = text;
  area.style.position = 'fixed';
  area.style.opacity = '0';
  document.body.appendChild(area);
  area.select();
  let ok = false;
  try { ok = document.execCommand('copy'); } catch (e) {}
  area.remove();
  done(ok);
}
