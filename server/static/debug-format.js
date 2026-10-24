// Response rendering and byte/encoding helpers for the debug panel.
// Served concatenated after debug.js to keep each source file small.

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
