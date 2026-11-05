// Captura telas do painel com o Microsoft Edge em modo headless (Chrome
// DevTools Protocol), usando apenas Node.js. Usado para revisão visual e
// para as imagens da documentação.
//
// Uso: node scripts/dev/screenshot.mjs <url> <token> <saida.png> [largura] [altura] [página]
import { spawn } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const [url, token, out, width = '1600', height = '1000', page = ''] = process.argv.slice(2);
const edge = process.env.EDGE_PATH ?? 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
const port = 9333 + Math.floor(Math.random() * 500);
const profile = mkdtempSync(join(tmpdir(), 'sl-edge-'));
const proc = spawn(edge, ['--headless=new', `--remote-debugging-port=${port}`, `--user-data-dir=${profile}`, `--window-size=${width},${height}`,
  '--no-first-run', '--disable-extensions', '--mute-audio', 'about:blank'], { stdio: 'ignore' });

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function target() {
  for (let i = 0; i < 50; i++) {
    try {
      const list = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
      const t = list.find((x) => x.type === 'page');
      if (t) return t.webSocketDebuggerUrl;
    } catch { /* aguardando o Edge */ }
    await sleep(200);
  }
  throw new Error('Edge não respondeu');
}

let seq = 0;
const pending = new Map();
function send(ws, method, params = {}) {
  const id = ++seq;
  ws.send(JSON.stringify({ id, method, params }));
  return new Promise((res) => pending.set(id, res));
}

try {
  const ws = new WebSocket(await target());
  await new Promise((r) => (ws.onopen = r));
  ws.onmessage = (ev) => {
    const m = JSON.parse(ev.data);
    if (m.id && pending.has(m.id)) { pending.get(m.id)(m.result); pending.delete(m.id); }
  };
  await send(ws, 'Page.enable');
  await send(ws, 'Emulation.setDeviceMetricsOverride', { width: +width, height: +height, deviceScaleFactor: 1, mobile: false });
  await send(ws, 'Page.navigate', { url });
  await sleep(1500);
  if (token) {
    await send(ws, 'Runtime.evaluate', { expression: `sessionStorage.setItem('sacralink.token', ${JSON.stringify(token)}); location.reload();` });
    await sleep(4000);
  }
  if (page) {
    await send(ws, 'Runtime.evaluate', { expression: `[...document.querySelectorAll('nav button')].find(b => b.title === ${JSON.stringify(page)})?.click()` });
    await sleep(2500);
  }
  const shot = await send(ws, 'Page.captureScreenshot', { format: 'png' });
  writeFileSync(out, Buffer.from(shot.data, 'base64'));
  console.log('captura salva em', out);
  ws.close();
} finally {
  proc.kill();
  await sleep(500);
  try { rmSync(profile, { recursive: true, force: true }); } catch { /* perfil em uso */ }
}
