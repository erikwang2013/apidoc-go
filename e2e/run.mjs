#!/usr/bin/env node
// Real-browser E2E for the apidoc debug UI.
//
// No npm dependencies: builds and starts e2e/server, launches the installed
// Chrome headless, and drives it over CDP using Node's global WebSocket.
// Run from anywhere: node e2e/run.mjs
import { spawn, spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const sleep = ms => new Promise(r => setTimeout(r, ms));
const q = JSON.stringify;
const step = name => console.log('\n== ' + name);

function fail(msg) { throw new Error(msg); }
function assert(cond, msg) { if (!cond) fail(msg); }

function findChrome() {
  const names = ['google-chrome', 'google-chrome-stable', 'chromium', 'chromium-browser'];
  const env = process.env.CHROME;
  if (env) {
    if (!existsSync(env)) fail(`CHROME=${env} does not exist`);
    return env;
  }
  for (const n of names) {
    const r = spawnSync('which', [n], { encoding: 'utf8' });
    if (r.status === 0 && r.stdout.trim()) return r.stdout.trim();
  }
  return null;
}

// ---- fixture server -------------------------------------------------------

function startServer(bin) {
  const proc = spawn(bin, [], { cwd: here, stdio: ['ignore', 'pipe', 'pipe'] });
  let out = '', err = '';
  proc.stderr.on('data', d => { err = (err + d).slice(-8000); });
  const ready = new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('fixture server start timeout: ' + err)), 15000);
    proc.stdout.on('data', d => {
      out += d;
      const nl = out.indexOf('\n');
      if (nl < 0) return;
      clearTimeout(timer);
      try { resolve(JSON.parse(out.slice(0, nl))); }
      catch (e) { reject(new Error('bad fixture output: ' + out)); }
    });
    proc.once('exit', (code, signal) => {
      clearTimeout(timer);
      reject(new Error(`fixture server exited early (${code}/${signal}): ${err}`));
    });
  });
  return { proc, ready };
}

function stopChild(child) {
  return new Promise(resolve => {
    if (!child || child.exitCode !== null) return resolve();
    let done = false;
    const finish = () => {
      if (done) return;
      done = true;
      clearTimeout(killTimer);
      clearTimeout(giveUp);
      resolve();
    };
    child.once('exit', finish);
    child.kill('SIGTERM');
    const killTimer = setTimeout(() => child.kill('SIGKILL'), 2000);
    const giveUp = setTimeout(finish, 6000);
  });
}

// ---- minimal CDP client ---------------------------------------------------

class CDP {
  constructor(ws) {
    this.ws = ws;
    this.seq = 0;
    this.pending = new Map();
    this.handlers = new Map();
    ws.addEventListener('message', ev => {
      const m = JSON.parse(ev.data);
      if (m.id !== undefined) {
        const p = this.pending.get(m.id);
        if (!p) return;
        this.pending.delete(m.id);
        if (m.error) p.reject(new Error(`${m.error.message} (${m.error.code})`));
        else p.resolve(m.result);
        return;
      }
      for (const h of this.handlers.get(m.method) || []) h(m.params, m.sessionId);
    });
  }
  send(method, params = {}, sessionId) {
    const id = ++this.seq;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.ws.send(JSON.stringify({ id, method, params, sessionId }));
    });
  }
  on(method, fn) {
    if (!this.handlers.has(method)) this.handlers.set(method, []);
    this.handlers.get(method).push(fn);
  }
}

async function connectWS(url, timeoutMs = 10000) {
  let lastErr;
  for (let i = 0; i < 20; i++) {
    try {
      return await new Promise((resolve, reject) => {
        const ws = new WebSocket(url);
        const timer = setTimeout(() => { ws.close(); reject(new Error('websocket connect timeout')); }, timeoutMs);
        ws.addEventListener('open', () => { clearTimeout(timer); resolve(ws); }, { once: true });
        ws.addEventListener('error', e => {
          clearTimeout(timer);
          reject(new Error('websocket error' + (e.message ? ': ' + e.message : '')));
        }, { once: true });
      });
    } catch (e) {
      lastErr = e;
      await sleep(200);
    }
  }
  throw lastErr;
}

async function chromeDevtoolsURL(profile, proc, stderr) {
  const file = join(profile, 'DevToolsActivePort');
  const deadline = Date.now() + 20000;
  while (Date.now() < deadline) {
    if (proc.exitCode !== null) fail(`chrome exited early (${proc.exitCode}): ${stderr()}`);
    if (existsSync(file)) {
      const lines = readFileSync(file, 'utf8').trim().split('\n');
      if (lines.length >= 2 && lines[0].trim()) {
        return `ws://127.0.0.1:${lines[0].trim()}${lines[1].trim()}`;
      }
    }
    await sleep(100);
  }
  fail('chrome did not expose DevToolsActivePort: ' + stderr());
}

// ---- page helpers ---------------------------------------------------------

function makePage(cdp, sessionId) {
  const send = (m, p) => cdp.send(m, p, sessionId);

  async function evalJS(expression) {
    const r = await send('Runtime.evaluate', {
      expression, returnByValue: true, awaitPromise: true, userGesture: true,
    });
    if (r.exceptionDetails) {
      const ex = r.exceptionDetails.exception;
      fail('page JS exception: ' + ((ex && ex.description) || r.exceptionDetails.text));
    }
    return r.result.value;
  }

  async function waitFor(description, expression, timeoutMs = 10000) {
    const deadline = Date.now() + timeoutMs;
    let last;
    while (Date.now() < deadline) {
      last = await evalJS(expression);
      if (last) return last;
      await sleep(100);
    }
    fail(`timeout waiting for ${description}; last value: ${q(last)}`);
  }

  const clickById = id => evalJS(`(() => {
    const el = document.getElementById(${q(id)});
    if (!el) return false;
    el.click();
    return true;
  })()`);

  const clickAction = title => evalJS(`(() => {
    const li = [...document.querySelectorAll('#tree li.act')]
      .find(x => x.textContent.trim() === ${q(title)});
    if (!li) return false;
    li.click();
    return true;
  })()`);

  const clickButton = (root, label) => evalJS(`(() => {
    const b = [...document.querySelectorAll(${q(root + ' button')})]
      .find(x => x.textContent.trim() === ${q(label)});
    if (!b) return false;
    b.click();
    return true;
  })()`);

  const setValue = (id, value) => evalJS(`(() => {
    const el = document.getElementById(${q(id)});
    if (!el) return false;
    el.value = ${q(value)};
    el.dispatchEvent(new Event('input', { bubbles: true }));
    return el.value;
  })()`);

  const text = id => evalJS(`(() => {
    const el = document.getElementById(${q(id)});
    return el ? el.textContent : null;
  })()`);

  return { send, evalJS, waitFor, clickById, clickAction, clickButton, setValue, text };
}

// ---- test -----------------------------------------------------------------

async function main() {
  const browser = findChrome();
  if (!browser) fail('no Chrome/Chromium found; set CHROME=/path/to/chrome');
  if (typeof WebSocket !== 'function') fail('Node has no global WebSocket (need Node >= 22)');
  console.log('browser:', browser);

  // Build the fixture server from the nested e2e module.
  const binDir = mkdtempSync(join(tmpdir(), 'apidoc-e2e-bin-'));
  const serverBin = join(binDir, 'apidoc-e2e-server');
  const build = spawnSync(process.env.GO || 'go', ['build', '-o', serverBin, './server'], {
    cwd: here, encoding: 'utf8',
  });
  if (build.status !== 0) {
    rmSync(binDir, { recursive: true, force: true });
    fail('go build failed:\n' + (build.stderr || build.stdout));
  }

  const server = startServer(serverBin);
  let fixture;
  try {
    fixture = await server.ready;
  } catch (e) {
    await stopChild(server.proc);
    rmSync(binDir, { recursive: true, force: true });
    fail(e.message);
  }
  console.log('fixture:', fixture);

  const profile = mkdtempSync(join(tmpdir(), 'apidoc-e2e-chrome-'));
  const chromeArgs = [
    '--headless=new',
    '--remote-debugging-port=0',
    `--user-data-dir=${profile}`,
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-background-networking',
    '--disable-component-update',
    '--disable-sync',
    '--disable-extensions',
    '--disable-dev-shm-usage',
    '--disable-gpu',
    '--hide-scrollbars',
    '--window-size=1400,900',
    'about:blank',
  ];
  if ((process.getuid && process.getuid() === 0) || process.env.E2E_NO_SANDBOX === '1') {
    chromeArgs.unshift('--no-sandbox');
  }
  const chrome = spawn(browser, chromeArgs, { stdio: ['ignore', 'ignore', 'pipe'] });
  let chromeErr = '';
  chrome.stderr.on('data', d => { chromeErr = (chromeErr + d).slice(-8000); });

  let ws, cdp, page, sessionId;
  const pageErrors = [];
  const consoleErrors = [];
  let ok = false;

  try {
    const browserWS = await chromeDevtoolsURL(profile, chrome, () => chromeErr);
    ws = await connectWS(browserWS);
    cdp = new CDP(ws);

    const created = await cdp.send('Target.createTarget', { url: 'about:blank' });
    ({ sessionId } = await cdp.send('Target.attachToTarget', {
      targetId: created.targetId, flatten: true,
    }));
    page = makePage(cdp, sessionId);
    await page.send('Page.enable');
    await page.send('Runtime.enable');
    cdp.on('Runtime.exceptionThrown', p => {
      pageErrors.push((p.exceptionDetails.exception && p.exceptionDetails.exception.description) || p.exceptionDetails.text);
    });
    cdp.on('Runtime.consoleAPICalled', p => {
      if (p.type !== 'error') return;
      consoleErrors.push(p.args.map(a => a.value ?? a.description ?? '').join(' '));
    });
    try {
      await cdp.send('Browser.grantPermissions', {
        origin: fixture.url,
        permissions: ['clipboardReadWrite', 'clipboardSanitizedWrite'],
      });
    } catch (_) { /* clipboard check accepts the fallback path too */ }

    step('load /apidoc and wait for the sidebar');
    await page.send('Page.navigate', { url: fixture.url + '/apidoc' });
    await page.waitFor('menu tree',
      `document.readyState === 'complete' && document.querySelectorAll('#tree li.act').length >= 2`,
      15000);
    const titles = await page.evalJS(
      `[...document.querySelectorAll('#tree li.act')].map(x => x.textContent.trim())`);
    assert(titles.includes('TCP Echo'), 'sidebar missing TCP Echo: ' + q(titles));
    assert(titles.includes('No Responses'), 'sidebar missing No Responses: ' + q(titles));
    assert(titles.includes('Fake Stream'), 'sidebar missing Fake Stream: ' + q(titles));

    step('one-shot: select TCP Echo, fill payload, click Send');
    assert(await page.clickAction('TCP Echo'), 'could not click TCP Echo');
    await page.waitFor('TCP detail', `document.getElementById('d-title').textContent === 'TCP Echo'`);
    await page.clickById('tab-debug');
    const tcpUI = await page.evalJS(`(() => ({
      server: !document.getElementById('dg-server').classList.contains('hidden'),
      stream: !document.getElementById('dg-stream').classList.contains('hidden'),
      sendHidden: document.getElementById('dg-send').classList.contains('hidden'),
      action: document.getElementById('dg-action').value,
      target: document.getElementById('dg-target').value,
    }))()`);
    assert(tcpUI.server, 'server panel is hidden for TCP');
    assert(!tcpUI.stream, 'stream controls are visible for a one-shot protocol');
    assert(!tcpUI.sendHidden, 'Send button is hidden for a one-shot protocol');
    assert(tcpUI.action === 'SEND', 'action select is ' + q(tcpUI.action));
    assert(tcpUI.target === fixture.tcp, `target is ${q(tcpUI.target)}, want ${q(fixture.tcp)}`);

    const payload = 'e2e-echo-' + Date.now();
    assert((await page.setValue('dg-body', payload)) === payload, 'could not fill payload');
    assert(await page.clickById('dg-send'), 'could not click Send');
    await page.waitFor('TCP echo response body',
      `document.getElementById('res-body').textContent === ${q(payload)}`, 15000);
    const resStatus = await page.text('res-status');
    assert(resStatus.startsWith('OK'), 'response status is ' + q(resStatus));
    console.log('one-shot response:', q(await page.text('res-body')), '|', q(resStatus));

    step('regression: action with no documented responses');
    const noRespTarget = fixture.tcp + '?variant=no-responses';
    const noRespReady = `document.getElementById('dg-target').value === ${q(noRespTarget)}`;
    const beforeErrors = pageErrors.length;
    assert(await page.clickAction('No Responses'), 'could not click No Responses');
    await page.waitFor('No Responses detail title',
      `document.getElementById('d-title').textContent === 'No Responses'`);
    // showDetail sets the title before renderResponses throws, so wait for
    // either the exception event or the debug panel being rebuilt.
    let noRespErr = '';
    for (let i = 0; i < 40; i++) {
      if (pageErrors.length > beforeErrors) {
        noRespErr = pageErrors.slice(beforeErrors).join('\n');
        break;
      }
      if (await page.evalJS(noRespReady)) break;
      await sleep(100);
    }
    assert(!noRespErr, 'page exception opening an action with no responses: ' + noRespErr);
    await page.clickById('tab-debug');
    await page.waitFor('debug panel for the no-responses action',
      `!document.getElementById('dg-server').classList.contains('hidden') && ${noRespReady}`, 5000);
    console.log('no-responses action rendered');

    step('streaming + P3: select Fake Stream');
    assert(await page.clickAction('Fake Stream'), 'could not click Fake Stream');
    await page.waitFor('Fake Stream detail', `document.getElementById('d-title').textContent === 'Fake Stream'`);
    await page.clickById('tab-debug');
    await page.waitFor('streaming panel', `!document.getElementById('dg-stream').classList.contains('hidden')`);
    const streamUI = await page.evalJS(`(() => ({
      sendHidden: document.getElementById('dg-send').classList.contains('hidden'),
      target: document.getElementById('dg-target').value,
    }))()`);
    assert(streamUI.sendHidden, 'one-shot Send button is visible for a streaming protocol');
    assert(streamUI.target === 'fake://stream/demo', 'stream target is ' + q(streamUI.target));

    // P3: snippets panel, language tabs, copy button, working copy status.
    const snip = await page.evalJS(`(() => {
      const d = document.querySelector('#dg-snippets details');
      if (!d) return null;
      return {
        summary: d.querySelector('summary') ? d.querySelector('summary').textContent : '',
        pre: d.querySelector('pre') ? d.querySelector('pre').textContent : '',
        buttons: [...d.querySelectorAll('button')].map(b => b.textContent),
      };
    })()`);
    assert(snip, 'snippet panel did not render');
    assert(snip.summary === 'Code snippets', 'snippet summary is ' + q(snip.summary));
    assert(snip.buttons.includes('Copy'), 'copy button missing: ' + q(snip.buttons));
    assert(snip.buttons.includes('curl') && snip.buttons.includes('go'),
      'snippet language tabs missing: ' + q(snip.buttons));
    assert(snip.pre.includes('fake://stream/demo'), 'snippet did not fill {{url}}: ' + q(snip.pre));
    assert(await page.evalJS(`document.querySelector('#dg-snippets details').open = true`), 'open snippets');
    assert(await page.clickButton('#dg-snippets details', 'go'), 'could not click go snippet tab');
    const goSnip = await page.evalJS(`document.querySelector('#dg-snippets details pre').textContent`);
    assert(goSnip.includes('SUBSCRIBE') && goSnip.includes('fake://stream/demo'),
      'go snippet did not fill placeholders: ' + q(goSnip));
    assert(await page.clickButton('#dg-snippets details', 'Copy'), 'could not click Copy');
    const copyStatus = await page.waitFor('copy status',
      `(() => { const s = document.querySelector('#dg-snippets .meta'); return s && s.textContent; })()`, 5000);
    assert(copyStatus === 'copied' || copyStatus === 'copy failed',
      'unexpected copy status: ' + q(copyStatus));
    console.log('snippet copy status:', q(copyStatus));

    // P3: Field.Type == "file" renders a file input.
    const fileField = await page.evalJS(`(() => {
      const i = document.querySelector('#dg-connect input[type=file]');
      if (!i) return null;
      return {
        type: i.type, name: i.dataset.name, file: i.dataset.file,
        hidden: !!i.closest('.hidden'),
      };
    })()`);
    assert(fileField, 'file connect field did not render');
    assert(fileField.type === 'file', 'field type is ' + q(fileField.type));
    assert(fileField.name === 'cert', 'field name is ' + q(fileField.name));
    assert(fileField.file === '1', 'file marker is ' + q(fileField.file));
    assert(!fileField.hidden, 'file field is hidden');

    step('streaming: Connect, assert SSE transcript, Send, Close');
    assert(await page.clickButton('#dg-stream', 'Connect'), 'could not click Connect');
    await page.waitFor('connected status',
      `document.getElementById('dg-session').textContent === 'connected'`, 15000);
    await page.waitFor('welcome event over SSE',
      `document.getElementById('dg-transcript').textContent.includes('welcome')`, 15000);
    await page.waitFor('welcome payload',
      `document.getElementById('dg-transcript').textContent.includes('connected to fake://stream/demo')`, 15000);

    const sPayload = 'e2e-stream-' + Date.now();
    assert((await page.setValue('dg-body', sPayload)) === sPayload, 'could not fill stream payload');
    assert(await page.clickButton('#dg-stream', 'Send'), 'could not click stream Send');
    await page.waitFor('outgoing transcript line',
      `document.getElementById('dg-transcript').textContent.includes(${q('sent: ' + sPayload)})`, 15000);
    await page.waitFor('echo transcript line',
      `document.getElementById('dg-transcript').textContent.includes(${q('echo: ' + sPayload)})`, 15000);
    const transcript = await page.text('dg-transcript');
    console.log('transcript:\n' + transcript);

    assert(await page.clickButton('#dg-stream', 'Close'), 'could not click Close');
    await page.waitFor('session closed status',
      `document.getElementById('dg-session').textContent === 'session closed'`, 10000);

    assert(pageErrors.length === 0, 'page exceptions: ' + pageErrors.join('\n'));
    ok = true;
    console.log('\nPASS: one-shot TCP echo, streaming SSE transcript, P3 snippets/file UI');
  } catch (e) {
    console.error('\nFAIL:', e.message);
    if (consoleErrors.length) console.error('console errors:\n' + consoleErrors.join('\n'));
    if (pageErrors.length) console.error('page exceptions:\n' + pageErrors.join('\n'));
    if (page) {
      try {
        const shot = await page.send('Page.captureScreenshot', { format: 'png' });
        const path = join(tmpdir(), 'apidoc-e2e-failure.png');
        writeFileSync(path, Buffer.from(shot.data, 'base64'));
        console.error('screenshot:', path);
      } catch (_) { /* browser may already be gone */ }
      try {
        console.error('location:', await page.evalJS('location.href'));
        console.error('body text:', (await page.evalJS('document.body.innerText')).slice(0, 3000));
      } catch (_) { /* ignore */ }
    }
    process.exitCode = 1;
  } finally {
    if (cdp) {
      try { await Promise.race([cdp.send('Browser.close').catch(() => {}), sleep(2000)]); } catch (_) {}
    }
    if (ws) { try { ws.close(); } catch (_) {} }
    await stopChild(chrome);
    await stopChild(server.proc);
    rmSync(profile, { recursive: true, force: true });
    rmSync(binDir, { recursive: true, force: true });
    if (ok) console.log('cleaned up browser and fixture server');
  }
}

main().catch(e => {
  console.error('FAIL:', e.message);
  process.exitCode = 1;
});
