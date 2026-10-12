#!/usr/bin/env node
/**
 * shot.mjs - zero-dependency CDP screenshot + interaction harness.
 *
 * Drives a locally installed Chrome over the DevTools Protocol using Node's
 * global WebSocket (Node >= 22 / 26). No npm packages, no `require`.
 *
 *   node tools/shot.mjs --url <url> --out <png> [options]
 *
 * See --help for the full option list.
 */

import { spawn } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, resolve } from 'node:path';
import net from 'node:net';

const CHROME = process.env.CHROME_PATH || '/opt/workbench/app/chrome/chrome';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/** Synchronous (no event loop) sleep - usable from process 'exit' handlers. */
const syncSleep = (ms) => {
  try { Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms); } catch { /* ignore */ }
};

/* CLI parsing                                                         */

const USAGE = `Usage: node tools/shot.mjs --url <url> --out <png path> [options]

  --url <url>              page to load (http/https/file/data)
  --out <path>             where to write the final PNG (parent dirs created)
  --w <px>                 viewport width (default 1280)
  --h <px>                 viewport height (default 800)
  --scale <n>              device scale factor (default 1)
  --wait <ms>              settle time after load (default 700)
  --click <x,y>            click at CSS px coords (repeatable)
  --drag <x1,y1,x2,y2>     press, ~12 interpolated moves, release (repeatable)
  --key <name>             key down+up: single char or named key (repeatable)
  --wheel <x,y,deltaY>     mouse wheel at a point (repeatable)
  --eval <js>              evaluate JS, print JSON result (repeatable)
  --pre-eval <js>          run JS in every new document before its own scripts (repeatable)
  --wait-for <expr>        poll JS expression until truthy (10s timeout)
  --console                forward page console/errors to stderr (always on)
  --full                   full-page screenshot beyond the viewport
  --keep-open <ms>         wait after actions, before the screenshot (default 0)
  --port <n>               remote debugging port (default: a free one)
  --quiet                  suppress progress logging
  --help                   show this message
`;

class UsageError extends Error {}

function parseArgs(argv) {
  const o = {
    url: null,
    out: null,
    w: 1280,
    h: 800,
    scale: 1,
    wait: 700,
    full: false,
    keepOpen: 0,
    port: 0,
    quiet: false,
    console: false,
    waitFors: [],
    preEvals: [], // injected into every new document, before its own scripts
    actions: [], // ordered {kind, ...}
  };

  const num = (raw, flag, min = 0) => {
    const n = Number(raw);
    if (!Number.isFinite(n) || n < min) {
      throw new UsageError(`${flag} expects a number >= ${min}, got "${raw}"`);
    }
    return n;
  };
  const pair = (raw, flag, count) => {
    const parts = String(raw).split(',').map((s) => Number(s.trim()));
    if (parts.length !== count || parts.some((n) => !Number.isFinite(n))) {
      throw new UsageError(`${flag} expects ${count} comma-separated numbers, got "${raw}"`);
    }
    return parts;
  };

  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    const need = () => {
      if (i + 1 >= argv.length) throw new UsageError(`${a} requires a value`);
      return argv[++i];
    };
    switch (a) {
      case '--url': o.url = need(); break;
      case '--out': o.out = need(); break;
      case '--w': o.w = num(need(), '--w', 1); break;
      case '--h': o.h = num(need(), '--h', 1); break;
      case '--scale': o.scale = num(need(), '--scale', 0.01); break;
      case '--wait': o.wait = num(need(), '--wait', 0); break;
      case '--keep-open': o.keepOpen = num(need(), '--keep-open', 0); break;
      case '--port': o.port = num(need(), '--port', 1); break;
      case '--quiet': o.quiet = true; break;
      case '--full': o.full = true; break;
      case '--console': o.console = true; break;
      case '--wait-for': o.waitFors.push(need()); break;
      case '--click': {
        const [x, y] = pair(need(), '--click', 2);
        o.actions.push({ kind: 'click', x, y });
        break;
      }
      case '--drag': {
        const [x1, y1, x2, y2] = pair(need(), '--drag', 4);
        o.actions.push({ kind: 'drag', x1, y1, x2, y2 });
        break;
      }
      case '--key': o.actions.push({ kind: 'key', name: need() }); break;
      case '--wheel': {
        const [x, y, deltaY] = pair(need(), '--wheel', 3);
        o.actions.push({ kind: 'wheel', x, y, deltaY });
        break;
      }
      case '--eval': o.actions.push({ kind: 'eval', expr: need() }); break;
      case '--pre-eval': o.preEvals.push(need()); break;
      case '--help': case '-h': process.stdout.write(USAGE); process.exit(0); break;
      default:
        throw new UsageError(`unknown option "${a}"\n\n${USAGE}`);
    }
  }
  if (!o.url) throw new UsageError('--url is required');
  if (!o.out) throw new UsageError('--out is required');
  return o;
}

/* CDP client                                                          */

class CDP {
  constructor(ws) {
    this.ws = ws;
    this.nextId = 1;
    this.pending = new Map();
    this.listeners = new Map();
    this.sessionId = null;
    this.closed = false;
    this.closeReason = null;

    ws.addEventListener('message', (ev) => this._onMessage(ev.data));
    ws.addEventListener('close', () => {
      this.closed = true;
      this.closeReason = 'websocket closed';
      for (const [, p] of this.pending) p.reject(new Error('CDP connection closed'));
      this.pending.clear();
    });
    ws.addEventListener('error', () => {
      this.closeReason = 'websocket error';
    });
  }

  _onMessage(raw) {
    let msg;
    try {
      msg = JSON.parse(typeof raw === 'string' ? raw : String(raw));
    } catch {
      return;
    }
    if (msg.id !== undefined) {
      const p = this.pending.get(msg.id);
      if (!p) return;
      this.pending.delete(msg.id);
      if (msg.error) {
        const e = new Error(`${msg.error.message || 'CDP error'}${msg.error.data ? `: ${msg.error.data}` : ''}`);
        e.cdp = msg.error;
        p.reject(e);
      } else {
        p.resolve(msg.result ?? {});
      }
      return;
    }
    if (msg.method) {
      const fns = this.listeners.get(msg.method);
      if (fns) for (const fn of [...fns]) {
        try { fn(msg.params ?? {}, msg.sessionId ?? null); } catch { /* listener errors are not fatal */ }
      }
    }
  }

  /** send(method, params) -> Promise<result>; pass sessionId explicitly for root-level calls. */
  send(method, params = {}, sessionId = this.sessionId) {
    if (this.closed) return Promise.reject(new Error(`cannot send ${method}: CDP connection closed`));
    const id = this.nextId++;
    const payload = { id, method, params };
    if (sessionId) payload.sessionId = sessionId;
    return new Promise((resolve_, reject_) => {
      this.pending.set(id, { resolve: resolve_, reject: reject_ });
      try {
        this.ws.send(JSON.stringify(payload));
      } catch (err) {
        this.pending.delete(id);
        reject_(err);
      }
    });
  }

  on(method, fn) {
    if (!this.listeners.has(method)) this.listeners.set(method, new Set());
    this.listeners.get(method).add(fn);
    return () => this.listeners.get(method)?.delete(fn);
  }
}

/* Chrome lifecycle                                                    */

function freePort() {
  return new Promise((res, rej) => {
    const srv = net.createServer();
    srv.unref();
    srv.on('error', rej);
    srv.listen(0, '127.0.0.1', () => {
      const { port } = srv.address();
      srv.close(() => res(port));
    });
  });
}

function launchChrome(o, userDataDir, port, onStderrLine) {
  const args = [
    '--headless=new',
    '--no-sandbox',
    '--disable-dev-shm-usage',
    '--disable-gpu-sandbox',
    '--hide-scrollbars',
    '--mute-audio',
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-extensions',
    '--disable-background-timer-throttling',
    '--disable-renderer-backgrounding',
    `--force-device-scale-factor=${o.scale}`,
    `--window-size=${o.w},${o.h}`,
    `--remote-debugging-port=${port}`,
    `--user-data-dir=${userDataDir}`,
    'about:blank',
  ];
  const proc = spawn(CHROME, args, { stdio: ['ignore', 'ignore', 'pipe'] });
  proc.stderr.setEncoding('utf8');
  let buf = '';
  proc.stderr.on('data', (chunk) => {
    buf += chunk;
    let idx;
    while ((idx = buf.indexOf('\n')) >= 0) {
      const line = buf.slice(0, idx).trim();
      buf = buf.slice(idx + 1);
      if (line) onStderrLine(line);
    }
  });
  return proc;
}

/**
 * Discover the DevTools browser websocket endpoint. Primary path: HTTP poll of
 * /json/version. Fallback: the "DevTools listening on ws://..." stderr line.
 */
async function discoverEndpoint(proc, port, stderrState, log) {
  const deadline = Date.now() + 20000;
  const url = `http://127.0.0.1:${port}/json/version`;
  let lastErr = 'no attempt made';

  while (Date.now() < deadline) {
    if (proc.exitCode !== null) {
      throw new Error(
        `chrome exited early (code ${proc.exitCode}) before the DevTools endpoint came up` +
        tail(stderrState.lines)
      );
    }
    // stderr may already know the answer; try it as soon as it appears.
    if (stderrState.wsUrl) return stderrState.wsUrl;
    try {
      const res = await fetch(url);
      if (res.ok) {
        const info = await res.json();
        if (info && info.webSocketDebuggerUrl) return info.webSocketDebuggerUrl;
        lastErr = '/json/version had no webSocketDebuggerUrl';
      } else {
        lastErr = `HTTP ${res.status}`;
      }
    } catch (err) {
      lastErr = err.message;
    }
    await sleep(100);
  }

  if (stderrState.wsUrl) return stderrState.wsUrl;
  throw new Error(`could not discover the DevTools endpoint on port ${port} (${lastErr})${tail(stderrState.lines)}`);
}

function tail(lines, n = 12) {
  const t = lines.slice(-n).join('\n');
  return t ? `\n--- chrome stderr (tail) ---\n${t}` : '';
}

async function findPageSession(cdp, log) {
  // The target may take a moment to appear after the endpoint is live.
  for (let attempt = 0; attempt < 60; attempt++) {
    const { targetInfos = [] } = await cdp.send('Target.getTargets', {}, null);
    let page = targetInfos.find((t) => t.type === 'page');
    if (!page) {
      const { targetId } = await cdp.send('Target.createTarget', { url: 'about:blank' }, null);
      page = { targetId };
    }
    if (page) {
      try {
        const { sessionId } = await cdp.send(
          'Target.attachToTarget',
          { targetId: page.targetId, flatten: true },
          null,
        );
        if (sessionId) return sessionId;
      } catch (err) {
        log(`attach attempt ${attempt + 1} failed: ${err.message}`);
      }
    }
    await sleep(100);
  }
  throw new Error('could not attach to a page target');
}

/* Input helpers                                                       */

const NAMED_KEYS = {
  Escape: { key: 'Escape', code: 'Escape', vk: 27 },
  Enter: { key: 'Enter', code: 'Enter', vk: 13, text: '\r' },
  Tab: { key: 'Tab', code: 'Tab', vk: 9, text: '\t' },
  Backspace: { key: 'Backspace', code: 'Backspace', vk: 8 },
  Delete: { key: 'Delete', code: 'Delete', vk: 46 },
  Insert: { key: 'Insert', code: 'Insert', vk: 45 },
  Space: { key: ' ', code: 'Space', vk: 32, text: ' ' },
  ArrowLeft: { key: 'ArrowLeft', code: 'ArrowLeft', vk: 37 },
  ArrowUp: { key: 'ArrowUp', code: 'ArrowUp', vk: 38 },
  ArrowRight: { key: 'ArrowRight', code: 'ArrowRight', vk: 39 },
  ArrowDown: { key: 'ArrowDown', code: 'ArrowDown', vk: 40 },
  Home: { key: 'Home', code: 'Home', vk: 36 },
  End: { key: 'End', code: 'End', vk: 35 },
  PageUp: { key: 'PageUp', code: 'PageUp', vk: 33 },
  PageDown: { key: 'PageDown', code: 'PageDown', vk: 34 },
  Shift: { key: 'Shift', code: 'ShiftLeft', vk: 16 },
  Control: { key: 'Control', code: 'ControlLeft', vk: 17 },
  Alt: { key: 'Alt', code: 'AltLeft', vk: 18 },
  Meta: { key: 'Meta', code: 'MetaLeft', vk: 91 },
  CapsLock: { key: 'CapsLock', code: 'CapsLock', vk: 20 },
};
for (let i = 1; i <= 12; i++) {
  NAMED_KEYS[`F${i}`] = { key: `F${i}`, code: `F${i}`, vk: 111 + i };
}

function resolveKey(name) {
  if (Object.hasOwn(NAMED_KEYS, name)) return { ...NAMED_KEYS[name] };
  if ([...name].length === 1) {
    const ch = name;
    const upper = ch.toUpperCase();
    if (/[a-z]/i.test(ch)) {
      return { key: ch, code: `Key${upper}`, vk: upper.charCodeAt(0), text: ch };
    }
    if (/[0-9]/.test(ch)) {
      return { key: ch, code: `Digit${ch}`, vk: ch.charCodeAt(0), text: ch };
    }
    return { key: ch, code: ch, vk: ch.toUpperCase().charCodeAt(0) || 0, text: ch };
  }
  throw new Error(`unsupported key "${name}" (use a single char or one of: ${Object.keys(NAMED_KEYS).join(', ')})`);
}

async function mouse(cdp, type, x, y, extra = {}) {
  await cdp.send('Input.dispatchMouseEvent', {
    type,
    x,
    y,
    button: 'left',
    clickCount: 1,
    modifiers: 0,
    ...extra,
  });
}

async function doClick(cdp, x, y) {
  await mouse(cdp, 'mouseMoved', x, y, { button: 'none', buttons: 0 });
  await mouse(cdp, 'mousePressed', x, y, { buttons: 1 });
  await mouse(cdp, 'mouseReleased', x, y, { buttons: 0 });
}

async function doDrag(cdp, x1, y1, x2, y2, steps = 12) {
  await mouse(cdp, 'mouseMoved', x1, y1, { button: 'none', buttons: 0 });
  await mouse(cdp, 'mousePressed', x1, y1, { buttons: 1 });
  for (let i = 1; i <= steps; i++) {
    const t = i / steps;
    await mouse(cdp, 'mouseMoved', x1 + (x2 - x1) * t, y1 + (y2 - y1) * t, { buttons: 1 });
    await sleep(8); // let the page's rAF loop observe the motion
  }
  await mouse(cdp, 'mouseReleased', x2, y2, { buttons: 0 });
  await flushFrames(cdp); // let rAF-driven pages settle on the final position
}

async function doKey(cdp, name) {
  const k = resolveKey(name);
  const base = {
    key: k.key,
    code: k.code,
    windowsVirtualKeyCode: k.vk,
    nativeVirtualKeyCode: k.vk,
    modifiers: 0,
  };
  await cdp.send('Input.dispatchKeyEvent', { type: 'keyDown', ...base, ...(k.text ? { text: k.text } : {}) });
  await cdp.send('Input.dispatchKeyEvent', { type: 'keyUp', ...base });
}

async function doWheel(cdp, x, y, deltaY) {
  await mouse(cdp, 'mouseMoved', x, y, { button: 'none', buttons: 0 });
  await cdp.send('Input.dispatchMouseEvent', {
    type: 'mouseWheel',
    x,
    y,
    button: 'none',
    buttons: 0,
    clickCount: 0,
    deltaX: 0,
    deltaY,
  });
  // Wheel input is routed through the compositor; it only reaches the page's
  // main thread when a frame is produced. Force two frames so a following
  // --eval (or the screenshot) observes the event instead of racing it.
  await flushFrames(cdp);
}

/** Best-effort: wait for two rendered frames in the page (bounded by timeoutMs). */
async function flushFrames(cdp, timeoutMs = 400) {
  let timer;
  const fallback = new Promise((r) => { timer = setTimeout(() => r(false), timeoutMs); });
  try {
    await Promise.race([
      evaluate(cdp, 'new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(()=>r(true))))'),
      fallback,
    ]);
  } catch {
    /* page may be mid-navigation; flushing is best-effort */
  } finally {
    clearTimeout(timer);
  }
}

/* Page helpers                                                        */

async function evaluate(cdp, expression, { awaitPromise = true } = {}) {
  return cdp.send('Runtime.evaluate', {
    expression,
    returnByValue: true,
    awaitPromise,
    userGesture: true,
  });
}

async function waitFor(cdp, expression, timeoutMs = 10000) {
  const deadline = Date.now() + timeoutMs;
  let last = null;
  while (Date.now() < deadline) {
    const res = await evaluate(cdp, expression);
    if (res.exceptionDetails) {
      last = res.exceptionDetails.text || 'exception';
    } else if (res.result && res.result.value) {
      return res.result.value;
    } else {
      last = res.result ? res.result.value : 'undefined';
    }
    await sleep(100);
  }
  throw new Error(`--wait-for "${expression}" was not truthy within ${timeoutMs}ms (last value: ${JSON.stringify(last)})`);
}

function pngInfo(buf) {
  const sig = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
  const valid = buf.length > 24 && buf.subarray(0, 8).equals(sig);
  let width = null;
  let height = null;
  if (valid) {
    width = buf.readUInt32BE(16);
    height = buf.readUInt32BE(20);
  }
  return { valid, width, height };
}

function formatRemoteObject(obj) {
  if (!obj) return 'undefined';
  if (obj.type === 'string') return obj.value;
  if ('value' in obj) return JSON.stringify(obj.value);
  if (obj.unserializableValue) return obj.unserializableValue;
  return obj.description || obj.type;
}

/* Main                                                                */

let activeCleanup = null;
let syncCleanup = null;
let shuttingDown = false;

/* Last-resort synchronous cleanup: runs on every exit path, including
   process.exit() from a signal handler or from main(). */
process.on('exit', () => {
  try { syncCleanup?.(); } catch { /* ignore */ }
});

async function main() {
  let opts;
  try {
    opts = parseArgs(process.argv.slice(2));
  } catch (err) {
    process.stderr.write(`[error] ${err.message}\n`);
    process.exit(2);
  }

  const log = (msg) => { if (!opts.quiet) process.stderr.write(`[shot] ${msg}\n`); };

  const port = opts.port || (await freePort());
  const userDataDir = mkdtempSync(`${tmpdir()}/shot-chrome-`);
  const stderrState = { lines: [], wsUrl: null };
  let proc = null;
  let cdp = null;
  let exitCode = 0;
  let cleanedUp = false;

  const onStderrLine = (line) => {
    stderrState.lines.push(line);
    if (stderrState.lines.length > 60) stderrState.lines.shift();
    const m = line.match(/DevTools listening on (ws:\/\/\S+)/);
    if (m) stderrState.wsUrl = m[1];
    // Chrome is chatty about dbus/crashpad in containers; only echo it on demand.
    if (process.env.SHOT_DEBUG) process.stderr.write(`[chrome] ${line}\n`);
  };

  const cleanup = async () => {
    if (cleanedUp) return;
    cleanedUp = true;
    try { cdp?.ws?.close(); } catch { /* ignore */ }
    if (proc && proc.exitCode === null && !proc.killed) {
      try { proc.kill('SIGKILL'); } catch { /* ignore */ }
      // give the OS a moment to reap before the caller checks for orphans
      for (let i = 0; i < 25 && proc.exitCode === null; i++) await sleep(20);
    }
    try { rmSync(userDataDir, { recursive: true, force: true }); } catch { /* ignore */ }
  };
  activeCleanup = cleanup;
  syncCleanup = () => {
    if (!proc || proc.exitCode !== null || proc.killed) {
      try { rmSync(userDataDir, { recursive: true, force: true }); } catch { /* ignore */ }
      return;
    }
    try { proc.kill('SIGKILL'); } catch { /* ignore */ }
    syncSleep(150); // let the kernel reap it before deleting its profile
    try { rmSync(userDataDir, { recursive: true, force: true }); } catch { /* ignore */ }
  };

  try {
    log(`launching chrome (port ${port})`);
    proc = launchChrome(opts, userDataDir, port, onStderrLine);

    const wsUrl = await discoverEndpoint(proc, port, stderrState, log);
    log(`devtools endpoint ${wsUrl}`);

    const ws = new WebSocket(wsUrl);
    await new Promise((res, rej) => {
      const to = setTimeout(() => rej(new Error(`timed out connecting to ${wsUrl}`)), 10000);
      ws.addEventListener('open', () => { clearTimeout(to); res(); }, { once: true });
      ws.addEventListener('error', () => { clearTimeout(to); rej(new Error(`websocket error connecting to ${wsUrl}`)); }, { once: true });
    });
    cdp = new CDP(ws);

    /* ---- page-error plumbing (always on) ---- */
    cdp.on('Runtime.exceptionThrown', (p) => {
      const d = p.exceptionDetails || {};
      const desc = d.exception?.description || d.text || 'uncaught exception';
      process.stderr.write(`[page-error] ${desc}\n`);
    });
    cdp.on('Log.entryAdded', (p) => {
      const e = p.entry || {};
      if (e.level === 'error') {
        process.stderr.write(`[page-error] ${e.source || 'log'}: ${e.text}${e.url ? ` (${e.url})` : ''}\n`);
      } else {
        process.stderr.write(`[page-log:${e.level || 'info'}] ${e.text}\n`);
      }
    });
    cdp.on('Runtime.consoleAPICalled', (p) => {
      const args = (p.args || []).map(formatRemoteObject).join(' ');
      const text = p.type === 'error' ? `[page-error] console.${p.type}: ${args}` : `[page-console:${p.type}] ${args}`;
      process.stderr.write(`${text}\n`);
    });

    cdp.sessionId = await findPageSession(cdp, log);
    log('attached to page target');

    await cdp.send('Page.enable');
    await cdp.send('Runtime.enable');
    await cdp.send('Log.enable');
    await cdp.send('Emulation.setDeviceMetricsOverride', {
      width: opts.w,
      height: opts.h,
      deviceScaleFactor: opts.scale,
      mobile: false,
    });

    /* ---- navigate + load ---- */
    for (const expr of opts.preEvals) {
      log(`pre-eval: ${expr}`);
      await cdp.send('Page.addScriptToEvaluateOnNewDocument', { source: expr });
    }
    let loadFired = false;
    const onLoad = cdp.on('Page.loadEventFired', () => { loadFired = true; });
    log(`navigating to ${opts.url}`);
    const nav = await cdp.send('Page.navigate', { url: opts.url });
    if (nav.errorText) {
      throw new Error(`navigation to ${opts.url} failed: ${nav.errorText}`);
    }
    const loadDeadline = Date.now() + 30000;
    while (!loadFired && Date.now() < loadDeadline) {
      if (proc.exitCode !== null) throw new Error(`chrome exited (code ${proc.exitCode}) during navigation`);
      await sleep(50);
    }
    onLoad();
    if (!loadFired) log('warning: Page.loadEventFired did not arrive; continuing after fallback timeout');

    for (const expr of opts.waitFors) {
      log(`waiting for ${expr}`);
      await waitFor(cdp, expr);
    }

    if (opts.wait > 0) {
      log(`settling ${opts.wait}ms`);
      await sleep(opts.wait);
    }

    /* ---- ordered actions ---- */
    for (const a of opts.actions) {
      switch (a.kind) {
        case 'click':
          log(`click ${a.x},${a.y}`);
          await doClick(cdp, a.x, a.y);
          break;
        case 'drag':
          log(`drag ${a.x1},${a.y1} -> ${a.x2},${a.y2}`);
          await doDrag(cdp, a.x1, a.y1, a.x2, a.y2);
          break;
        case 'key':
          log(`key ${a.name}`);
          await doKey(cdp, a.name);
          break;
        case 'wheel':
          log(`wheel ${a.x},${a.y} dy=${a.deltaY}`);
          await doWheel(cdp, a.x, a.y, a.deltaY);
          break;
        case 'eval': {
          const res = await evaluate(cdp, a.expr);
          if (res.exceptionDetails) {
            const d = res.exceptionDetails;
            const desc = d.exception?.description || d.text || 'evaluation error';
            process.stderr.write(`[eval] ${a.expr} => ERROR: ${desc}\n`);
            throw new Error(`--eval failed: ${desc}`);
          }
          const value = res.result && 'value' in res.result ? res.result.value : undefined;
          process.stdout.write(`[eval] ${a.expr} => ${JSON.stringify(value)}\n`);
          break;
        }
        default:
          throw new Error(`internal error: unknown action ${a.kind}`);
      }
    }

    /* ---- pre-shot delay + screenshot ---- */
    if (opts.keepOpen > 0) {
      log(`keeping open ${opts.keepOpen}ms`);
      await sleep(opts.keepOpen);
    }

    const shotParams = { format: 'png', captureBeyondViewport: !!opts.full };
    if (opts.full) {
      const metrics = await cdp.send('Page.getLayoutMetrics');
      const size = metrics.cssContentSize || metrics.contentSize;
      shotParams.clip = {
        x: 0,
        y: 0,
        width: Math.ceil(size.width),
        height: Math.ceil(size.height),
        scale: 1,
      };
      log(`full-page capture ${shotParams.clip.width}x${shotParams.clip.height}`);
    }

    const { data } = await cdp.send('Page.captureScreenshot', shotParams);
    if (!data) throw new Error('Page.captureScreenshot returned no data');
    const buf = Buffer.from(data, 'base64');
    const info = pngInfo(buf);
    if (!info.valid) throw new Error(`captured data is not a valid PNG (${buf.length} bytes)`);

    const outPath = resolve(opts.out);
    mkdirSync(dirname(outPath), { recursive: true });
    writeFileSync(outPath, buf);

    process.stdout.write(`[shot] wrote ${outPath} (${buf.length} bytes, ${info.width}x${info.height})\n`);
  } catch (err) {
    if (shuttingDown) {
      // interrupted: the signal handler owns cleanup and the exit code
    } else {
      process.stderr.write(`[error] ${err?.message || String(err)}\n`);
      if (/websocket|endpoint|chrome exited/i.test(err?.message || '')) {
        process.stderr.write(tail(stderrState.lines) + '\n');
      }
      exitCode = 1;
    }
  } finally {
    await cleanup();
  }

  process.exit(exitCode);
}

/* Ctrl-C / termination: kill chrome and remove the profile before exiting. */
for (const sig of ['SIGINT', 'SIGTERM']) {
  process.on(sig, () => {
    if (shuttingDown) process.exit(130);
    shuttingDown = true;
    process.stderr.write(`\n[shot] received ${sig}, cleaning up\n`);
    // Run the async cleanup first (bounded), then the sync one as a backstop.
    const hard = setTimeout(() => { try { syncCleanup?.(); } catch { /* ignore */ } process.exit(130); }, 1000);
    hard.unref?.();
    Promise.resolve(activeCleanup ? activeCleanup() : null)
      .catch(() => {})
      .finally(() => { clearTimeout(hard); process.exit(130); });
  });
}

try {
  await main();
} catch (err) {
  // Anything thrown outside main()'s own error handling (e.g. tmpdir/port setup).
  process.stderr.write(`[error] ${err?.message || String(err)}\n`);
  try { await activeCleanup?.(); } catch { /* ignore */ }
  process.exit(1);
}
