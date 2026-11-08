// Screenshot HTML files with headless Chrome over the DevTools protocol.
// `chrome --screenshot` writes the file but does not always exit.
//
//   node shot.mjs <chrome> <page.html> <out.png> [<page.html> <out.png> ...]
//
// Needs Node 22 or newer (built-in WebSocket).
import { spawn } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const [chrome, ...pairs] = process.argv.slice(2);
const profile = mkdtempSync(join(tmpdir(), 'relic-chrome-'));
const proc = spawn(chrome, ['--headless=new', '--disable-gpu', '--hide-scrollbars', '--no-first-run',
  '--remote-debugging-port=0', `--user-data-dir=${profile}`, 'about:blank'], { stdio: ['ignore', 'ignore', 'pipe'] });

const browserUrl = await new Promise((ok, fail) => {
  let log = '';
  proc.stderr.on('data', (d) => {
    log += d;
    const m = log.match(/DevTools listening on (ws:\S+)/);
    if (m) ok(m[1]);
  });
  proc.on('exit', () => fail(new Error('chrome exited: ' + log)));
});
const port = new URL(browserUrl).port;
const targets = await (await fetch(`http://127.0.0.1:${port}/json`)).json();
const ws = new WebSocket(targets.find((t) => t.type === 'page').webSocketDebuggerUrl);
await new Promise((ok) => { ws.onopen = ok; });

let nextId = 1;
const pending = new Map();
const waiting = new Map();
ws.onmessage = (e) => {
  const msg = JSON.parse(e.data);
  if (msg.id && pending.has(msg.id)) {
    const { ok, fail } = pending.get(msg.id);
    pending.delete(msg.id);
    if (msg.error) fail(new Error(msg.error.message)); else ok(msg.result);
  } else if (msg.method && waiting.has(msg.method)) {
    waiting.get(msg.method)();
    waiting.delete(msg.method);
  }
};
const send = (method, params = {}) => new Promise((ok, fail) => {
  const id = nextId++;
  pending.set(id, { ok, fail });
  ws.send(JSON.stringify({ id, method, params }));
});
const once = (method) => new Promise((ok) => waiting.set(method, ok));

let code = 0;
try {
  await send('Page.enable');
  await send('Runtime.enable');
  await send('Emulation.setDeviceMetricsOverride', { width: 1440, height: 900, deviceScaleFactor: 2, mobile: false });
  for (let i = 0; i < pairs.length; i += 2) {
    const loaded = once('Page.loadEventFired');
    await send('Page.navigate', { url: 'file://' + resolve(pairs[i]) });
    await loaded;
    await send('Runtime.evaluate', { expression: 'document.fonts.ready.then(() => document.fonts.size)', awaitPromise: true });
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    writeFileSync(pairs[i + 1], Buffer.from(data, 'base64'));
  }
} catch (err) {
  console.error(err);
  code = 1;
}
ws.close();
proc.removeAllListeners('exit');
proc.kill();
await new Promise((ok) => setTimeout(ok, 300));
rmSync(profile, { recursive: true, force: true });
process.exit(code);
