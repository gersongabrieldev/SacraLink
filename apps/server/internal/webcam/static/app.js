// SacraLink Câmera — transforma o celular em câmera pelo navegador.
//
// Usa as mesmas APIs do aplicativo nativo:
//   * POST /device/v1/pair           pareamento com o token do QR Code;
//   * /device/v1/ws (controle v1)    tally, comandos remotos e telemetria;
//   * POST /device/v1/whip           publicação WebRTC (WHIP) no MediaMTX.
// O navegador não consegue enviar o cabeçalho Authorization no WebSocket, por
// isso o canal de controle usa um ticket de uso único (/device/v1/ws-ticket).
//
// Diferença de segurança em relação ao app: o navegador não faz pinning do
// certificado; a identidade do servidor depende de o operador aceitar o
// certificado do endereço lido no QR Code (ver SECURITY.md).

const STORE_KEY = 'sacralink.cam.v1';
const LENS_KEY = 'sacralink.cam.lenses';
const LIVE_KEY = 'sacralink.cam.live';
const LOCAL_WINDOW_MS = 3000;

const PROFILES = {
  economico: { id: 'economico', width: 854, height: 480, fps: 30, maxBitrateKbps: 1000, minBitrateKbps: 250, adaptive: true },
  equilibrado: { id: 'equilibrado', width: 1280, height: 720, fps: 30, maxBitrateKbps: 2500, minBitrateKbps: 500, adaptive: true },
  alta: { id: 'alta', width: 1920, height: 1080, fps: 30, maxBitrateKbps: 4500, minBitrateKbps: 800, adaptive: true },
  alta60: { id: 'alta60', width: 1920, height: 1080, fps: 60, maxBitrateKbps: 6500, minBitrateKbps: 1200, adaptive: true },
};

const $ = (id) => document.getElementById(id);
const clamp = (v, lo, hi) => Math.min(Math.max(v, lo), hi);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function load(storage, key) {
  try { return JSON.parse(storage.getItem(key) || 'null'); } catch { return null; }
}
function save(storage, key, value) {
  try {
    if (value === null || value === undefined) storage.removeItem(key);
    else storage.setItem(key, JSON.stringify(value));
  } catch { /* armazenamento indisponível (aba anônima): segue sem lembrar */ }
}

// ---------------------------------------------------------------------------
// Estado

const S = {
  creds: null,
  appVersion: 'web',
  video: null, // MediaStreamTrack de vídeo
  audio: null, // MediaStreamTrack de áudio
  lenses: [],
  lensId: null,
  zoomCaps: null, // {min, max, step}
  zoom: 1,
  torchCap: false,
  torch: false,
  focusCap: false,
  micMuted: false,
  profile: PROFILES.equilibrado,
  limits: null,
  stats: {},
  streamState: 'idle', // idle | connecting | live | reconnecting | error
  wantLive: false,
  liveSince: 0,
  controlConnected: false,
  sessionId: '',
  tally: 'unknown',
  obsLive: false,
  battery: null,
  network: 'unknown',
  error: '',
};

// ---------------------------------------------------------------------------
// HTTP

class ApiError extends Error {
  constructor(status, code, message) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

function authHeader() {
  return `Bearer ${S.creds.deviceId}.${S.creds.secret}`;
}

async function request(method, path, { body, type = 'application/json', auth = false, timeoutMs = 10000, keepalive = false } = {}) {
  const ctl = new AbortController();
  const timer = setTimeout(() => ctl.abort(), timeoutMs);
  const headers = {};
  if (body !== undefined) headers['Content-Type'] = type;
  if (auth) headers.Authorization = authHeader();
  try {
    return await fetch(path, {
      method, headers, keepalive, cache: 'no-store', signal: ctl.signal,
      body: body === undefined ? undefined : type === 'application/json' ? JSON.stringify(body) : body,
    });
  } catch (e) {
    throw new ApiError(0, 'network', e.name === 'AbortError' ? 'O computador da igreja não respondeu a tempo.' : 'Sem conexão com o computador da igreja.');
  } finally {
    clearTimeout(timer);
  }
}

async function errorFrom(res, fallback) {
  let code = 'http';
  let message = fallback;
  try {
    const j = await res.json();
    if (j && j.error) {
      code = j.error.code || code;
      message = j.error.message || message;
    }
  } catch { /* corpo não JSON */ }
  return new ApiError(res.status, code, message);
}

const AUTH_CODES = new Set(['unauthorized', 'device_revoked']);

// Credencial recusada: o aparelho foi removido no painel.
function credentialRejected(err) {
  if (err.code === 'device_blocked') {
    S.error = 'Este celular foi bloqueado no painel do SacraLink.';
    render();
    return false;
  }
  if (!AUTH_CODES.has(err.code)) return false;
  S.wantLive = false;
  save(sessionStorage, LIVE_KEY, null);
  save(localStorage, STORE_KEY, null);
  control.close();
  publisher.stop();
  stopCapture();
  S.creds = null; // nada mais reabre câmera ou reconecta com a credencial revogada
  showMessage('Este celular foi desconectado do SacraLink. No computador, gere um novo QR Code (Câmeras → Adicionar câmera) e leia-o com a câmera do celular.');
  return true;
}

// ---------------------------------------------------------------------------
// Pareamento

function parseFragment(hash) {
  const q = new URLSearchParams(hash.replace(/^#/, ''));
  if (q.get('v') !== '1' || !q.get('t')) return null;
  return { token: q.get('t'), serverName: q.get('n') || 'SacraLink', serverId: q.get('id') || '', exp: Number(q.get('exp') || 0) };
}

function platformInfo() {
  const ua = navigator.userAgent;
  let platform = 'other';
  let os = '';
  let m;
  if ((m = ua.match(/Android\s([\d.]+)/))) {
    platform = 'android';
    os = `Android ${m[1]}`;
  } else if (/iPhone|iPad|iPod/.test(ua) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)) {
    platform = 'ios';
    m = ua.match(/OS (\d+(?:_\d+)*)/);
    os = `iOS ${m ? m[1].replace(/_/g, '.') : ''}`.trim();
  }
  const browser = /EdgA?\//.test(ua) ? 'Edge' : /SamsungBrowser/.test(ua) ? 'Samsung Internet' : /CriOS|Chrome\//.test(ua) ? 'Chrome'
    : /FxiOS|Firefox\//.test(ua) ? 'Firefox' : /Safari\//.test(ua) ? 'Safari' : 'Navegador';
  return { platform, osVersion: os, model: `${browser} (navegador)` };
}

async function pair(info, name) {
  const p = platformInfo();
  const res = await request('POST', '/device/v1/pair', {
    body: { token: info.token, device: { name, platform: p.platform, model: p.model, osVersion: p.osVersion, appVersion: S.appVersion } },
  });
  if (res.status !== 201) throw await errorFrom(res, 'Não foi possível conectar este celular.');
  const r = await res.json();
  return {
    deviceId: r.deviceId, secret: r.secret, cameraId: r.cameraId, cameraName: r.cameraName, slot: r.slot,
    serverId: r.serverId, serverName: r.serverName, name,
  };
}

// ---------------------------------------------------------------------------
// Captura (câmera e microfone)

const AUDIO = { echoCancellation: true, noiseSuppression: true, autoGainControl: true };

function lensById(id) {
  return S.lenses.find((l) => l.id === id) || null;
}

function videoConstraints() {
  const p = S.profile;
  return { width: { ideal: p.width }, height: { ideal: p.height }, frameRate: { ideal: p.fps } };
}

function describeMediaError(e) {
  switch (e && e.name) {
    case 'NotAllowedError':
    case 'SecurityError':
      return 'Permissão da câmera negada. Libere câmera e microfone para este site nas configurações do navegador e recarregue a página.';
    case 'NotReadableError':
    case 'AbortError':
      return 'A câmera está em uso por outro aplicativo. Feche-o e tente novamente.';
    case 'NotFoundError':
    case 'OverconstrainedError':
      return 'Câmera não encontrada neste aparelho.';
  }
  return `Não foi possível abrir a câmera (${(e && e.message) || e}).`;
}

// Abre (ou reabre) a câmera. Muitos celulares não abrem duas câmeras ao mesmo
// tempo: a faixa anterior é parada antes de abrir a nova.
async function openCapture() {
  const video = { ...videoConstraints(), zoom: true };
  const lens = lensById(S.lensId);
  if (lens) video.deviceId = { exact: lens.deviceId };
  else video.facingMode = { ideal: 'environment' };

  if (S.video) S.video.stop();
  const needAudio = !S.audio || S.audio.readyState === 'ended';
  let st;
  try {
    st = await navigator.mediaDevices.getUserMedia({ video, audio: needAudio ? AUDIO : false });
  } catch (e) {
    if (needAudio && e.name === 'NotFoundError') {
      st = await navigator.mediaDevices.getUserMedia({ video, audio: false }); // aparelho sem microfone
    } else if (lens && e.name === 'OverconstrainedError') {
      delete video.deviceId;
      st = await navigator.mediaDevices.getUserMedia({ video, audio: needAudio ? AUDIO : false });
    } else {
      throw e;
    }
  }
  S.video = st.getVideoTracks()[0];
  if ('contentHint' in S.video) S.video.contentHint = 'motion';
  const a = st.getAudioTracks()[0];
  if (a) {
    S.audio = a;
    S.audio.enabled = !S.micMuted;
  }
  S.video.onended = () => {
    notice('A câmera foi interrompida pelo sistema. Reabrindo…', 'warn');
    reopenSoon();
  };
  S.video.onmute = () => notice('O sistema pausou a câmera. Mantenha esta página aberta e a tela ligada.', 'warn', 6000);
  $('preview').srcObject = new MediaStream([S.video]);

  if (!S.lenses.length || S.lenses.some((l) => !l.label)) S.lenses = await enumerateLenses();
  const devId = S.video.getSettings().deviceId;
  const current = S.lenses.find((l) => l.deviceId === devId);
  if (current) S.lensId = current.id;
  await readCapabilities();
  await publisher.replaceTracks();
  render();
}

function stopCapture() {
  for (const t of [S.video, S.audio]) if (t) t.stop();
  S.video = null;
  S.audio = null;
}

let reopenTimer = 0;
function reopenSoon() {
  clearTimeout(reopenTimer);
  reopenTimer = setTimeout(async () => {
    if (document.visibilityState !== 'visible' || !S.creds) return;
    try {
      await openCapture();
    } catch (e) {
      notice(describeMediaError(e), 'error', 8000);
    }
  }, 800);
}

function lensKind(label, facing) {
  if (facing === 'front') return 'front';
  if (/ultra/.test(label)) return 'ultra_wide';
  if (/tele/.test(label)) return 'telephoto';
  return 'wide';
}

const KIND_LABEL = { front: 'Frontal', ultra_wide: 'Ultra-angular', telephoto: 'Teleobjetiva', wide: 'Principal', unknown: 'Câmera' };

async function enumerateLenses() {
  const known = load(localStorage, LENS_KEY) || {};
  const devs = (await navigator.mediaDevices.enumerateDevices()).filter((d) => d.kind === 'videoinput');
  const count = {};
  return devs.map((d, i) => {
    const label = (d.label || '').toLowerCase();
    let facing = '';
    try {
      const fm = d.getCapabilities ? d.getCapabilities().facingMode || [] : [];
      if (fm.includes('environment')) facing = 'back';
      else if (fm.includes('user')) facing = 'front';
    } catch { /* InputDeviceInfo sem getCapabilities */ }
    if (!facing) {
      if (/front|frontal|user|selfie|face/.test(label)) facing = 'front';
      else if (/back|rear|traseira|environment/.test(label)) facing = 'back';
      else facing = 'external';
    }
    const kind = facing === 'external' ? 'unknown' : lensKind(label, facing);
    count[kind] = (count[kind] || 0) + 1;
    const id = d.deviceId ? d.deviceId.slice(0, 32) : `cam${i}`;
    const z = known[id] || [1, 1];
    return { id, deviceId: d.deviceId, facing, kind, label: KIND_LABEL[kind] + (count[kind] > 1 ? ` ${count[kind]}` : ''), minZoom: z[0], maxZoom: z[1] };
  });
}

// Lê as capacidades REAIS da câmera aberta (zoom, lanterna, foco).
async function readCapabilities() {
  let caps = S.video.getCapabilities ? S.video.getCapabilities() : {};
  // O Chrome do Android pode informar o zoom alguns instantes depois de abrir.
  if (!caps.zoom && /Android/.test(navigator.userAgent)) {
    await sleep(600);
    if (!S.video) return;
    caps = S.video.getCapabilities();
  }
  const z = caps.zoom;
  S.zoomCaps = z && z.max > z.min ? { min: z.min, max: z.max, step: z.step || 0.1 } : null;
  const settings = S.video.getSettings();
  S.zoom = S.zoomCaps ? settings.zoom || S.zoomCaps.min : 1;
  S.torchCap = caps.torch === true;
  S.torch = S.torchCap && settings.torch === true;
  const supported = navigator.mediaDevices.getSupportedConstraints();
  S.focusCap = !!supported.pointsOfInterest && Array.isArray(caps.focusMode) && caps.focusMode.length > 0;

  const lens = lensById(S.lensId);
  if (lens) {
    const range = S.zoomCaps ? [S.zoomCaps.min, S.zoomCaps.max] : [1, 1];
    if (lens.minZoom !== range[0] || lens.maxZoom !== range[1]) {
      lens.minZoom = range[0];
      lens.maxZoom = range[1];
      const known = load(localStorage, LENS_KEY) || {};
      known[lens.id] = range;
      save(localStorage, LENS_KEY, known);
      // Faixa de zoom descoberta agora: reapresenta as capacidades ao servidor
      // depois que o resultado do comando em andamento for enviado.
      if (S.controlConnected) setTimeout(() => control.restart(), 1500);
    }
  }
}

function maxCaps() {
  const caps = S.video && S.video.getCapabilities ? S.video.getCapabilities() : {};
  const w = caps.width && caps.width.max ? Math.min(caps.width.max, 1920) : 1280;
  const h = caps.height && caps.height.max ? Math.min(caps.height.max, 1080) : 720;
  const fps = caps.frameRate && caps.frameRate.max ? Math.min(Math.round(caps.frameRate.max), 60) : 30;
  return { w, h, fps };
}

// Reaplica restrições preservando zoom e lanterna atuais.
async function applyTrackConstraints(extra = {}) {
  const adv = {};
  if (S.zoomCaps) adv.zoom = 'zoom' in extra ? extra.zoom : S.zoom;
  if (S.torchCap) adv.torch = 'torch' in extra ? extra.torch : S.torch;
  if (extra.pointsOfInterest) Object.assign(adv, extra.pointsOfInterest);
  await S.video.applyConstraints({ ...videoConstraints(), advanced: Object.keys(adv).length ? [adv] : [] });
}

// ---------------------------------------------------------------------------
// Controles (locais sempre funcionam e ganham prioridade de 3 s)

class Unsupported extends Error {}

const priority = new Map();
const lastSent = new Map();
const trailing = new Map();
// Avisa o servidor (no máximo 4 por segundo por controle; o valor final
// sempre é enviado — o controle deslizante de zoom gera dezenas de eventos).
function localChange(param, value) {
  const now = Date.now();
  priority.set(param, now);
  clearTimeout(trailing.get(param));
  const send = () => {
    lastSent.set(param, Date.now());
    control.send('local_change', { param, value });
  };
  const wait = 250 - (now - (lastSent.get(param) || 0));
  if (wait <= 0) send();
  else trailing.set(param, setTimeout(send, wait));
}
function locked(param) {
  const t = priority.get(param);
  return t !== undefined && Date.now() - t < LOCAL_WINDOW_MS;
}

let zoomBusy = false;
let zoomPending = null;
async function setZoom(v, local) {
  if (!S.video || !S.zoomCaps) throw new Unsupported();
  const target = clamp(v, S.zoomCaps.min, S.zoomCaps.max);
  S.zoom = target;
  if (local) localChange('zoom', Math.round(target * 100) / 100);
  if (zoomBusy) { // controle deslizante: aplica só o valor mais recente
    zoomPending = target;
    render();
    return;
  }
  zoomBusy = true;
  try {
    await applyTrackConstraints({ zoom: target });
    while (zoomPending !== null) {
      const next = zoomPending;
      zoomPending = null;
      await applyTrackConstraints({ zoom: next });
    }
    const real = S.video && S.video.getSettings().zoom;
    if (typeof real === 'number') S.zoom = real;
  } finally {
    zoomBusy = false;
    render();
  }
}

async function selectLens(id, local) {
  if (!lensById(id)) throw new Unsupported();
  S.lensId = id;
  await openCapture();
  if (local) localChange('camera', id);
}

function flipTarget() {
  const cur = lensById(S.lensId);
  const facing = cur ? cur.facing : 'back';
  return S.lenses.find((l) => l.facing !== facing && (l.facing === 'front' || l.facing === 'back')) || null;
}

async function flipCamera(local) {
  const t = flipTarget();
  if (!t) throw new Unsupported();
  await selectLens(t.id, false);
  if (local) localChange('camera', 'flip');
}

async function setTorch(on, local) {
  if (!S.video || !S.torchCap) throw new Unsupported();
  await applyTrackConstraints({ torch: on });
  S.torch = on;
  if (local) localChange('torch', on);
  render();
}

function setMicMuted(muted, local) {
  if (!S.audio) throw new Unsupported();
  S.micMuted = muted;
  S.audio.enabled = !muted;
  if (local) localChange('mic', muted);
  render();
}

async function focusAt(x, y, local) {
  if (!S.video || !S.focusCap) throw new Unsupported();
  const caps = S.video.getCapabilities();
  const poi = { pointsOfInterest: [{ x, y }] };
  const fm = caps.focusMode || [];
  if (fm.includes('continuous')) poi.focusMode = 'continuous';
  else if (fm.includes('single-shot')) poi.focusMode = 'single-shot';
  const em = caps.exposureMode || [];
  if (em.includes('continuous')) poi.exposureMode = 'continuous';
  await applyTrackConstraints({ pointsOfInterest: poi });
  if (local) localChange('focus', [x, y]);
}

function profileFromJson(j) {
  const n = (v, d) => (typeof v === 'number' ? v : d);
  return {
    id: typeof j.id === 'string' ? j.id : typeof j.profileId === 'string' ? j.profileId : 'auto',
    width: n(j.width, 1280), height: n(j.height, 720), fps: n(j.fps, 30),
    maxBitrateKbps: n(j.maxBitrateKbps, 2500), minBitrateKbps: n(j.minBitrateKbps, 300),
    adaptive: typeof j.adaptive === 'boolean' ? j.adaptive : true,
  };
}

async function setProfile(p, local) {
  S.profile = p;
  ensureQualityOption(p);
  if (S.video) {
    try {
      await applyTrackConstraints();
    } catch {
      await openCapture();
    }
  }
  S.limits = adapt(S.profile, conditions());
  await publisher.applyLimits(S.limits);
  if (local) localChange('quality', p.id);
  render();
}

function ensureQualityOption(p) {
  const sel = $('quality');
  if (![...sel.options].some((o) => o.value === p.id)) {
    const o = document.createElement('option');
    o.value = p.id;
    o.textContent = `${p.height}p${p.fps === 30 ? '' : ` ${p.fps}`}`;
    sel.appendChild(o);
  }
  sel.value = p.id;
}

// ---------------------------------------------------------------------------
// Adaptação de qualidade (mesma regra de apps/mobile/lib/src/adaptive.dart)

function adapt(p, c) {
  let bitrate = p.maxBitrateKbps;
  let scale = 1;
  let fps = p.fps;
  const reasons = [];
  if (p.adaptive) {
    if (c.availableKbps > 0 && c.availableKbps * 0.85 < bitrate) {
      bitrate = c.availableKbps * 0.85;
      reasons.push('banda limitada');
    }
    if (c.lossPercent > 8) {
      bitrate *= 0.7;
      reasons.push('perda de pacotes alta');
    } else if (c.lossPercent > 3) {
      bitrate *= 0.85;
      reasons.push('perda de pacotes');
    }
    if (c.cellular && bitrate > 2500) bitrate = 2500;
  }
  if (c.batteryLevel < 0.15 && !c.charging && scale < 1.5) {
    scale = 1.5;
    reasons.push('bateria baixa');
  }
  if (bitrate < p.minBitrateKbps * 1.2 && scale < 1.5 && p.adaptive) scale = 1.5;
  if (bitrate < p.minBitrateKbps) bitrate = p.minBitrateKbps;
  return { maxBitrateKbps: Math.round(bitrate), scaleDownBy: scale, maxFps: fps, reason: reasons.join(', ') };
}

function conditions() {
  return {
    availableKbps: S.stats.availableKbps || 0,
    lossPercent: S.stats.lossPercent || 0,
    batteryLevel: S.battery ? S.battery.level : 1,
    charging: S.battery ? S.battery.charging : true,
    cellular: S.network === 'cellular',
  };
}

// ---------------------------------------------------------------------------
// Publicação WHIP

class Publisher {
  constructor() {
    this.pc = null;
    this.videoSender = null;
    this.audioSender = null;
    this.location = null;
    this.gen = 0;
    this.prev = { bytes: 0, at: 0, lost: 0, packets: 0 };
  }

  get active() {
    return this.pc !== null;
  }

  async start() {
    this.stop();
    const gen = ++this.gen;
    // Sem servidores ICE externos: a conexão é direta com a igreja.
    const pc = new RTCPeerConnection({ iceServers: [], bundlePolicy: 'max-bundle' });
    this.pc = pc;
    pc.onconnectionstatechange = () => {
      if (this.pc === pc) onPeerState(pc.connectionState);
    };
    const tracks = [S.video, S.audio].filter(Boolean);
    const stream = new MediaStream(tracks);
    if (S.audio) this.audioSender = pc.addTransceiver(S.audio, { direction: 'sendonly', streams: [stream] }).sender;
    const vt = pc.addTransceiver(S.video, { direction: 'sendonly', streams: [stream] });
    this.videoSender = vt.sender;
    preferH264(vt);

    await pc.setLocalDescription(await pc.createOffer());
    await waitIceGathering(pc, 3000);
    if (gen !== this.gen) return;
    const res = await request('POST', '/device/v1/whip', { body: pc.localDescription.sdp, type: 'application/sdp', auth: true, timeoutMs: 15000 });
    if (gen !== this.gen) return;
    if (res.status !== 201) {
      this.stop();
      throw await errorFrom(res, `O servidor recusou a transmissão (HTTP ${res.status}).`);
    }
    this.location = res.headers.get('Location');
    const answer = await res.text();
    if (gen !== this.gen) return;
    await pc.setRemoteDescription({ type: 'answer', sdp: withStartBitrate(answer, startBitrateKbps()) });
    S.limits = adapt(S.profile, conditions());
    await this.applyLimits(S.limits);
  }

  async applyLimits(l) {
    const s = this.videoSender;
    if (!s || !l) return;
    try {
      const params = s.getParameters();
      if (!params.encodings || !params.encodings.length) return;
      for (const e of params.encodings) {
        e.maxBitrate = l.maxBitrateKbps * 1000;
        e.maxFramerate = l.maxFps;
        e.scaleResolutionDownBy = l.scaleDownBy;
      }
      await s.setParameters(params);
    } catch { /* navegador sem suporte a algum limite: mantém o padrão */ }
  }

  // Troca de câmera/lente sem renegociar a sessão.
  async replaceTracks() {
    try {
      if (this.videoSender && S.video) await this.videoSender.replaceTrack(S.video);
      if (this.audioSender && S.audio && this.audioSender.track !== S.audio) await this.audioSender.replaceTrack(S.audio);
    } catch {
      if (S.wantLive) restartPublish();
    }
  }

  async stats() {
    const pc = this.pc;
    if (!pc) return {};
    const reports = await pc.getStats();
    let bytes = 0, packets = 0, lost = 0, width = 0, height = 0, fps = 0, rtt = 0, jitter = 0, available = 0;
    reports.forEach((r) => {
      if (r.type === 'outbound-rtp' && r.kind === 'video') {
        bytes += r.bytesSent || 0;
        packets += r.packetsSent || 0;
        fps = r.framesPerSecond || fps;
        width = r.frameWidth || width;
        height = r.frameHeight || height;
      } else if (r.type === 'remote-inbound-rtp' && r.kind === 'video') {
        lost += r.packetsLost || 0;
        jitter = (r.jitter || 0) * 1000;
        if (r.roundTripTime) rtt = r.roundTripTime * 1000;
      } else if (r.type === 'candidate-pair' && r.state === 'succeeded' && (r.nominated || r.selected)) {
        if (r.currentRoundTripTime) rtt = r.currentRoundTripTime * 1000;
        if (r.availableOutgoingBitrate) available = r.availableOutgoingBitrate / 1000;
      }
    });
    const now = performance.now();
    const p = this.prev;
    const dt = p.at ? (now - p.at) / 1000 : 0;
    const kbps = dt > 0 && bytes >= p.bytes ? ((bytes - p.bytes) * 8) / dt / 1000 : 0;
    const dPackets = packets - p.packets;
    const dLost = lost - p.lost;
    const lossPercent = dPackets > 0 && dLost > 0 ? (dLost * 100) / (dPackets + dLost) : 0;
    this.prev = { bytes, at: now, lost, packets };
    return { bitrateKbps: kbps, fps, width, height, packetsLost: lost, lossPercent, rttMs: rtt, jitterMs: jitter, availableKbps: available };
  }

  // Encerra a sessão (DELETE libera o servidor na hora).
  stop(keepalive = false) {
    this.gen++;
    const loc = this.location;
    this.location = null;
    if (loc && S.creds) request('DELETE', loc, { auth: true, timeoutMs: 3000, keepalive }).catch(() => {});
    const pc = this.pc;
    this.pc = null;
    this.videoSender = null;
    this.audioSender = null;
    this.prev = { bytes: 0, at: 0, lost: 0, packets: 0 };
    if (pc) pc.close();
  }
}

function preferH264(transceiver) {
  try {
    const caps = RTCRtpSender.getCapabilities && RTCRtpSender.getCapabilities('video');
    if (!caps || !transceiver.setCodecPreferences) return;
    const h264 = caps.codecs.filter((c) => c.mimeType.toLowerCase() === 'video/h264');
    if (!h264.length) return;
    // Perfil "constrained baseline" primeiro: decodificado por qualquer OBS.
    h264.sort((a, b) => (/profile-level-id=42e0/.test(b.sdpFmtpLine || '') ? 1 : 0) - (/profile-level-id=42e0/.test(a.sdpFmtpLine || '') ? 1 : 0));
    const rest = caps.codecs.filter((c) => c.mimeType.toLowerCase() !== 'video/h264');
    transceiver.setCodecPreferences([...h264, ...rest]);
  } catch { /* o servidor negocia H.264 mesmo sem preferência */ }
}

// Taxa inicial do vídeo. Sem ela o WebRTC começa em ~300 kb/s e a imagem fica
// pequena nos primeiros segundos; na rede da igreja dá para começar perto da
// meta — o controle de congestionamento reduz se a rede não suportar.
function startBitrateKbps() {
  const cap = S.network === 'cellular' ? 1000 : 2500;
  return Math.round(Math.max(Math.min(S.profile.maxBitrateKbps * 0.6, cap), S.profile.minBitrateKbps));
}

function withStartBitrate(sdp, kbps) {
  const video = new Set([...sdp.matchAll(/^a=rtpmap:(\d+) (?:H264|VP8|VP9|AV1)\/90000/gim)].map((m) => m[1]));
  return sdp.replace(/^(a=fmtp:(\d+) [^\r\n]*)/gm, (line, _all, pt) =>
    video.has(pt) && !line.includes('x-google-start-bitrate') ? `${line};x-google-start-bitrate=${kbps}` : line);
}

function waitIceGathering(pc, ms) {
  if (pc.iceGatheringState === 'complete') return Promise.resolve();
  return new Promise((resolve) => {
    const t = setTimeout(resolve, ms);
    pc.addEventListener('icegatheringstatechange', () => {
      if (pc.iceGatheringState === 'complete') {
        clearTimeout(t);
        resolve();
      }
    });
  });
}

const publisher = new Publisher();
let retryTimer = 0;
let retryMs = 1000;
let disconnectTimer = 0;

function onPeerState(state) {
  clearTimeout(disconnectTimer);
  if (state === 'connected') {
    S.streamState = 'live';
    if (!S.liveSince) S.liveSince = Date.now();
    retryMs = 1000;
    S.error = '';
  } else if (state === 'failed') {
    if (S.wantLive) scheduleRetry();
  } else if (state === 'disconnected') {
    // Oscilação curta do Wi-Fi costuma se recuperar sozinha.
    if (S.wantLive) {
      S.streamState = 'reconnecting';
      disconnectTimer = setTimeout(() => S.wantLive && restartPublish(), 4000);
    }
  }
  render();
}

async function publish() {
  if (!S.wantLive || !S.video) return;
  clearTimeout(retryTimer);
  S.streamState = S.liveSince ? 'reconnecting' : 'connecting';
  render();
  try {
    await publisher.start();
    S.error = '';
  } catch (e) {
    if (e instanceof ApiError && credentialRejected(e)) return;
    S.error = e.message || String(e);
    if (e instanceof ApiError && e.code === 'no_permission') {
      S.streamState = 'error';
      S.wantLive = false;
    } else {
      scheduleRetry(e instanceof ApiError && e.code === 'stream_limit' ? 10000 : 0);
    }
  }
  render();
}

function scheduleRetry(minMs = 0) {
  if (!S.wantLive) return;
  S.streamState = 'reconnecting';
  clearTimeout(retryTimer);
  retryTimer = setTimeout(publish, Math.max(retryMs, minMs));
  retryMs = Math.min(retryMs * 2, 10000);
}

function restartPublish() {
  retryMs = 1000;
  publisher.stop();
  publish();
}

async function startStream() {
  S.wantLive = true;
  save(sessionStorage, LIVE_KEY, true);
  requestWakeLock();
  await publish();
}

function stopStream() {
  S.wantLive = false;
  save(sessionStorage, LIVE_KEY, null);
  clearTimeout(retryTimer);
  clearTimeout(disconnectTimer);
  publisher.stop();
  S.streamState = 'idle';
  S.liveSince = 0;
  render();
}

// ---------------------------------------------------------------------------
// Canal de controle v1

class Control {
  constructor() {
    this.ws = null;
    this.closed = true;
    this.retryMs = 1000;
    this.timer = 0;
    this.ping = 0;
    this.ackTimer = 0;
    this.connecting = false;
  }

  open() {
    this.closed = false;
    this.connect();
  }

  async connect() {
    if (this.closed || this.ws || this.connecting) return;
    clearTimeout(this.timer);
    this.connecting = true;
    try {
      const res = await request('POST', '/device/v1/ws-ticket', { auth: true });
      if (res.status !== 200) {
        const e = await errorFrom(res, 'O servidor recusou a conexão de controle.');
        if (credentialRejected(e)) return;
        throw e;
      }
      const { ticket } = await res.json();
      if (this.closed || this.ws) return;
      const ws = new WebSocket(`wss://${location.host}/device/v1/ws?ticket=${encodeURIComponent(ticket)}`, 'sacralink.control.v1');
      this.ws = ws;
      ws.onopen = () => {
        this.raw(ws, 'hello', helloData());
        this.ackTimer = setTimeout(() => ws.close(), 10000);
      };
      ws.onmessage = (ev) => this.onMessage(ws, ev.data);
      ws.onclose = () => this.onClose(ws);
    } catch {
      this.schedule();
    } finally {
      this.connecting = false;
    }
  }

  onMessage(ws, text) {
    let env;
    try { env = JSON.parse(text); } catch { return; }
    if (!env || env.v !== 1) {
      ws.close();
      return;
    }
    if (env.type === 'hello_ack') {
      clearTimeout(this.ackTimer);
      this.retryMs = 1000;
      S.controlConnected = true;
      document.body.dataset.control = 'on';
      const hb = Math.max(2, Number(env.data && env.data.heartbeatSec) || 5);
      clearInterval(this.ping);
      this.ping = setInterval(() => this.raw(ws, 'ping', {}), hb * 1000);
    }
    onControl(env.type, env.data || {});
  }

  onClose(ws) {
    if (this.ws !== ws) return;
    this.ws = null;
    clearInterval(this.ping);
    clearTimeout(this.ackTimer);
    S.controlConnected = false;
    S.sessionId = '';
    S.tally = 'unknown'; // sem controle: nunca manter NO AR desatualizado
    document.body.dataset.control = 'off';
    render();
    this.schedule();
  }

  schedule() {
    if (this.closed) return;
    clearTimeout(this.timer);
    this.timer = setTimeout(() => this.connect(), this.retryMs);
    this.retryMs = Math.min(this.retryMs * 2, 10000);
  }

  raw(ws, type, data) {
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ v: 1, type, ts: Date.now(), data }));
  }

  send(type, data) {
    if (S.controlConnected) this.raw(this.ws, type, data);
  }

  // Reconecta já (troca de rede ou novas capacidades).
  restart() {
    this.retryMs = 1000;
    const ws = this.ws;
    if (ws) ws.close();
    else this.connect();
  }

  close() {
    this.closed = true;
    clearTimeout(this.timer);
    const ws = this.ws;
    if (ws) ws.close();
  }
}

const control = new Control();

function helloData() {
  const p = platformInfo();
  const m = maxCaps();
  return {
    appVersion: S.appVersion,
    platform: p.platform,
    model: p.model,
    osVersion: p.osVersion,
    capabilities: {
      lenses: S.lenses.map(({ id, facing, kind, label, minZoom, maxZoom }) => ({ id, facing, kind, label, minZoom, maxZoom })),
      torch: S.torchCap,
      focusPoint: S.focusCap,
      exposurePoint: S.focusCap,
      maxWidth: m.w,
      maxHeight: m.h,
      maxFps: m.fps,
      hardwareH264: false, // o navegador não informa se a codificação é por hardware
      thermal: false,
    },
  };
}

function onControl(type, data) {
  switch (type) {
    case 'hello_ack':
      S.sessionId = data.sessionId || '';
      if (data.cameraName) S.creds.cameraName = data.cameraName;
      if (data.serverName) S.creds.serverName = data.serverName;
      save(localStorage, STORE_KEY, S.creds);
      if (data.profile && typeof data.profile === 'object') {
        const p = profileFromJson(data.profile);
        if (p.id !== S.profile.id || p.width !== S.profile.width || p.fps !== S.profile.fps) setProfile(p, false).catch(() => {});
      }
      break;
    case 'tally': {
      const connected = data.obsConnected === true;
      const prev = S.tally;
      S.obsLive = data.live === true;
      // OBS desconectado: nunca exibir NO AR desatualizado.
      S.tally = connected && ['on_air', 'preview', 'connected'].includes(data.state) ? data.state : 'unknown';
      if (S.tally === 'on_air' && prev !== 'on_air' && navigator.vibrate) navigator.vibrate(200);
      break;
    }
    case 'config':
      if (data.cameraName) {
        S.creds.cameraName = data.cameraName;
        save(localStorage, STORE_KEY, S.creds);
      }
      if (data.profile && typeof data.profile === 'object') setProfile(profileFromJson(data.profile), false).catch(() => {});
      break;
    case 'command':
      runCommand(data, Date.now());
      break;
  }
  render();
}

// Parâmetros afetados por comando (política de conflito).
function paramsOf(name) {
  switch (name) {
    case 'zoom.set': case 'zoom.step': return ['zoom'];
    case 'lens.select': case 'camera.flip': return ['camera'];
    case 'torch.set': return ['torch'];
    case 'audio.mute': return ['mic'];
    case 'quality.set': return ['quality'];
    case 'focus.point': case 'exposure.point': return ['focus'];
    case 'preset.apply': return ['camera', 'zoom', 'quality', 'torch', 'mic'];
  }
  return [name];
}

const results = new Map(); // idempotência por commandId
function rememberResult(id, r) {
  results.delete(id);
  results.set(id, r);
  while (results.size > 64) results.delete(results.keys().next().value);
}

function stateSnapshot() {
  return { zoom: Math.round(S.zoom * 100) / 100, lensId: S.lensId, torch: S.torch, micMuted: S.micMuted, profileId: S.profile.id };
}

async function runCommand(cmd, receivedAt) {
  const id = String(cmd.commandId || '');
  const cached = results.get(id);
  if (cached) {
    control.send('command_result', cached);
    return;
  }
  const name = String(cmd.name || '');
  const params = cmd.params && typeof cmd.params === 'object' ? cmd.params : {};
  const result = (status, message = '') => ({ commandId: id, status, ...(message ? { message } : {}), state: stateSnapshot() });
  // Validade relativa: imune à diferença de relógio entre celular e PC.
  const ttl = clamp((Number(cmd.expiresAt) || 0) - (Number(cmd.issuedAt) || 0), 0, 60000);

  let r;
  if (cmd.sessionId !== S.sessionId) {
    r = result('rejected', 'sessão diferente');
  } else if (Date.now() > receivedAt + ttl) {
    r = result('expired');
  } else if (cmd.force !== true && paramsOf(name).some(locked)) {
    r = result('conflict', 'o operador do celular está ajustando este controle');
  } else {
    try {
      r = (await execute(name, params)) ? result('executed') : result('unsupported', 'recurso não disponível neste aparelho');
    } catch (e) {
      r = e instanceof Unsupported ? result('unsupported', 'recurso não disponível neste navegador') : result('failed', String((e && e.message) || e));
    }
  }
  if (r.status === 'executed') notice(describeCommand(name), 'info', 2500);
  rememberResult(id, r);
  control.send('command_result', r);
  render();
}

async function execute(name, p) {
  const num = (k) => {
    const v = Number(p[k]);
    if (!Number.isFinite(v)) throw new Error(`parâmetro ${k} inválido`);
    return v;
  };
  switch (name) {
    case 'zoom.set':
      await setZoom(num('value'), false);
      return true;
    case 'zoom.step':
      await setZoom(S.zoom + num('delta'), false);
      return true;
    case 'lens.select':
      if (!lensById(p.lensId)) return false;
      await selectLens(p.lensId, false);
      return true;
    case 'camera.flip':
      await flipCamera(false);
      return true;
    case 'torch.set':
      await setTorch(p.on === true, false);
      return true;
    case 'audio.mute':
      setMicMuted(p.muted === true, false);
      return true;
    case 'focus.point':
    case 'exposure.point':
      await focusAt(clamp(num('x'), 0, 1), clamp(num('y'), 0, 1), false);
      return true;
    case 'quality.set':
      await setProfile(profileFromJson(p), false);
      return true;
    case 'preset.apply':
      if (typeof p.lensId === 'string' && lensById(p.lensId) && p.lensId !== S.lensId) await selectLens(p.lensId, false);
      if (p.profile && typeof p.profile === 'object') await setProfile(profileFromJson(p.profile), false);
      if (typeof p.zoom === 'number' && S.zoomCaps) await setZoom(p.zoom, false);
      if (typeof p.torch === 'boolean' && S.torchCap) await setTorch(p.torch, false);
      if (typeof p.micMuted === 'boolean' && S.audio) setMicMuted(p.micMuted, false);
      return true;
    case 'stream.reconnect':
      if (S.wantLive) restartPublish();
      return true;
  }
  return false;
}

function describeCommand(name) {
  switch (name) {
    case 'zoom.set': case 'zoom.step': return 'Zoom ajustado pelo operador';
    case 'lens.select': case 'camera.flip': return 'Câmera trocada pelo operador';
    case 'quality.set': return 'Qualidade alterada pelo operador';
    case 'preset.apply': return 'Preset aplicado pelo operador';
    case 'audio.mute': return 'Microfone alterado pelo operador';
  }
  return 'Ajuste remoto aplicado';
}

// ---------------------------------------------------------------------------
// Telemetria (a cada 2 s)

function networkType() {
  const c = navigator.connection;
  if (!c || !c.type) return navigator.onLine ? 'unknown' : 'none';
  if (c.type === 'wifi') return 'wifi';
  if (c.type === 'cellular') return 'cellular';
  if (c.type === 'ethernet') return 'ethernet';
  if (c.type === 'none') return 'none';
  return 'other';
}

function networkQuality() {
  if (S.streamState !== 'live') return 'unknown';
  if (S.stats.lossPercent > 8 || S.stats.rttMs > 400) return 'poor';
  if (S.stats.lossPercent > 3 || S.stats.rttMs > 150) return 'fair';
  return 'good';
}

async function tick() {
  if (!S.creds) return;
  if (publisher.active) {
    try {
      S.stats = await publisher.stats();
    } catch {
      S.stats = {};
    }
    const next = adapt(S.profile, conditions());
    const l = S.limits;
    if (!l || l.maxBitrateKbps !== next.maxBitrateKbps || l.scaleDownBy !== next.scaleDownBy || l.maxFps !== next.maxFps) {
      S.limits = next;
      await publisher.applyLimits(next);
    }
  } else {
    S.stats = {};
  }
  const st = S.stats;
  const lens = lensById(S.lensId);
  control.send('telemetry', {
    ...(S.battery ? { battery: { level: S.battery.level, charging: S.battery.charging } } : {}),
    stream: {
      state: S.streamState,
      bitrateKbps: st.bitrateKbps || 0, fps: st.fps || 0, width: st.width || 0, height: st.height || 0,
      packetsLost: st.packetsLost || 0, rttMs: st.rttMs || 0, jitterMs: st.jitterMs || 0, availableKbps: st.availableKbps || 0,
    },
    camera: { lensId: S.lensId || '', facing: lens ? lens.facing : 'back', zoom: S.zoom, torch: S.torch, micMuted: S.micMuted },
    network: { type: S.network, quality: networkQuality() },
  });
  render();
}

// Troca Wi-Fi ↔ 4G/5G: nova sessão WHIP e reconexão do controle imediatas.
function onNetworkChange() {
  const before = S.network;
  S.network = networkType();
  if (before !== 'unknown' && before !== S.network && S.network !== 'none') {
    control.restart();
    if (S.wantLive) restartPublish();
  }
  render();
}

// ---------------------------------------------------------------------------
// Tela ligada

let wakeLock = null;
async function requestWakeLock() {
  if (!('wakeLock' in navigator) || document.visibilityState !== 'visible' || wakeLock) return;
  try {
    wakeLock = await navigator.wakeLock.request('screen');
    wakeLock.addEventListener('release', () => { wakeLock = null; });
  } catch { /* economia de bateria ativa: o aviso na tela orienta o operador */ }
}

// ---------------------------------------------------------------------------
// Interface

let noticeSeq = 0;
function notice(text, kind = 'info', ms = 4000) {
  const box = $('notices');
  const el = document.createElement('div');
  el.className = `notice ${kind}`;
  el.textContent = text;
  el.dataset.seq = String(++noticeSeq);
  box.appendChild(el);
  while (box.children.length > 3) box.firstChild.remove();
  setTimeout(() => el.remove(), ms);
}

function showMessage(text) {
  document.body.dataset.screen = 'message';
  $('message-text').textContent = text;
}

const TALLY_TEXT = { on_air: '● NO AR', preview: 'PREVIEW', connected: 'CONECTADA' };
const STREAM_TEXT = { idle: 'Parada', connecting: 'Conectando…', live: 'Transmitindo', reconnecting: 'Reconectando…', error: 'Erro' };
const QUALITY_TEXT = { good: 'Rede boa', fair: 'Rede instável', poor: 'Rede ruim' };

function fmtZoom(v) {
  return `${v.toFixed(1).replace('.', ',')}×`;
}

function fmtDuration(ms) {
  const s = Math.floor(ms / 1000);
  const p = (n) => String(n).padStart(2, '0');
  return `${p(Math.floor(s / 3600))}:${p(Math.floor((s % 3600) / 60))}:${p(s % 60)}`;
}

function chip(text, cls = '') {
  const el = document.createElement('span');
  el.className = `chip ${cls}`;
  el.textContent = text;
  return el;
}

function render() {
  const b = document.body;
  b.dataset.tally = S.tally;
  b.dataset.stream = S.streamState;
  if (!S.creds) return;

  $('tally').textContent = TALLY_TEXT[S.tally] || (S.controlConnected ? 'OBS SEM CONEXÃO' : 'SEM CONEXÃO COM O PC');
  $('cam-name').textContent = S.creds.cameraName || 'Câmera';
  $('server-name').textContent = S.creds.serverName || '';

  const chips = [];
  const st = STREAM_TEXT[S.streamState] || S.streamState;
  chips.push(chip(S.streamState === 'live' && S.liveSince ? `${st} ${fmtDuration(Date.now() - S.liveSince)}` : st,
    S.streamState === 'reconnecting' || S.streamState === 'error' ? 'bad' : ''));
  if (!S.controlConnected) chips.push(chip('Controle desconectado', 'bad'));
  if (S.obsLive) chips.push(chip('OBS ao vivo'));
  if (S.streamState === 'live') {
    const q = networkQuality();
    chips.push(chip(`${QUALITY_TEXT[q] || ''} ${((S.stats.bitrateKbps || 0) / 1000).toFixed(1).replace('.', ',')} Mb/s`.trim(), q === 'poor' ? 'bad' : q === 'fair' ? 'warn' : ''));
    if (S.stats.width) chips.push(chip(`${S.stats.width}×${S.stats.height} ${Math.round(S.stats.fps || 0)} fps`));
    if (S.limits && S.limits.reason) chips.push(chip(`Qualidade reduzida: ${S.limits.reason}`, 'warn'));
  }
  if (S.battery) {
    const pct = Math.round(S.battery.level * 100);
    chips.push(chip(`Bateria ${pct}%${S.battery.charging ? ' ⚡' : ''}`, pct < 20 && !S.battery.charging ? 'bad' : ''));
  }
  if (S.micMuted) chips.push(chip('Microfone mudo', 'warn'));
  if (S.network === 'cellular') chips.push(chip('4G/5G'));
  if (S.error) chips.push(chip(S.error, 'bad'));
  $('chips').replaceChildren(...chips);

  const live = S.wantLive;
  const start = $('start');
  start.textContent = live ? 'Parar transmissão' : 'Iniciar transmissão';
  start.disabled = !S.video;

  const zb = $('zoom-box');
  zb.hidden = !S.zoomCaps;
  if (S.zoomCaps) {
    const z = $('zoom');
    z.min = String(S.zoomCaps.min);
    z.max = String(S.zoomCaps.max);
    z.step = String(S.zoomCaps.step);
    if (document.activeElement !== z) z.value = String(S.zoom);
    $('zoom-value').textContent = fmtZoom(S.zoom);
  }
  const torch = $('torch');
  torch.hidden = !S.torchCap;
  torch.classList.toggle('on', S.torch);
  const mic = $('mic');
  mic.hidden = !S.audio;
  mic.classList.toggle('off', S.micMuted);
  mic.setAttribute('aria-pressed', String(S.micMuted));
  $('flip').hidden = !flipTarget();

  const cur = lensById(S.lensId);
  const same = S.lenses.filter((l) => cur && l.facing === cur.facing);
  const lensBox = $('lenses');
  if (same.length > 1) {
    lensBox.replaceChildren(...same.map((l) => {
      const btn = document.createElement('button');
      btn.textContent = l.label;
      btn.dataset.lens = l.id;
      btn.className = l.id === S.lensId ? 'active' : '';
      return btn;
    }));
  } else {
    lensBox.replaceChildren();
  }
  if ($('quality').value !== S.profile.id) ensureQualityOption(S.profile);
}

// Toque na imagem: foco/exposição no ponto (quando o navegador permite).
function onPreviewTap(ev) {
  if (!S.focusCap || !S.video) return;
  const v = $('preview');
  const rect = v.getBoundingClientRect();
  const vw = v.videoWidth || rect.width;
  const vh = v.videoHeight || rect.height;
  const scale = Math.min(rect.width / vw, rect.height / vh);
  const w = vw * scale;
  const h = vh * scale;
  const x = (ev.clientX - rect.left - (rect.width - w) / 2) / w;
  const y = (ev.clientY - rect.top - (rect.height - h) / 2) / h;
  if (x < 0 || x > 1 || y < 0 || y > 1) return;
  const ring = $('focus-ring');
  ring.style.left = `${ev.clientX}px`;
  ring.style.top = `${ev.clientY}px`;
  ring.hidden = false;
  setTimeout(() => { ring.hidden = true; }, 900);
  focusAt(x, y, true).catch(() => {});
}

function bindUi() {
  $('start').addEventListener('click', () => {
    if (S.wantLive) {
      if (S.tally === 'on_air' && !confirm('Esta câmera está NO AR. Parar a transmissão mesmo assim?')) return;
      stopStream();
    } else {
      startStream();
    }
  });
  $('flip').addEventListener('click', () => flipCamera(true).catch((e) => notice(describeMediaError(e), 'error')));
  $('torch').addEventListener('click', () => setTorch(!S.torch, true).catch(() => notice('Lanterna indisponível.', 'warn')));
  $('mic').addEventListener('click', () => {
    try { setMicMuted(!S.micMuted, true); } catch { /* sem microfone */ }
  });
  $('zoom').addEventListener('input', (e) => setZoom(Number(e.target.value), true).catch(() => {}));
  const step = () => (S.zoomCaps ? Math.max((S.zoomCaps.max - S.zoomCaps.min) / 10, S.zoomCaps.step) : 0.5);
  $('zoom-in').addEventListener('click', () => setZoom(S.zoom + step(), true).catch(() => {}));
  $('zoom-out').addEventListener('click', () => setZoom(S.zoom - step(), true).catch(() => {}));
  $('lenses').addEventListener('click', (e) => {
    const id = e.target && e.target.dataset ? e.target.dataset.lens : '';
    if (id && id !== S.lensId) selectLens(id, true).catch((err) => notice(describeMediaError(err), 'error'));
  });
  $('quality').addEventListener('change', (e) => {
    const p = PROFILES[e.target.value];
    if (p) setProfile(p, true).catch(() => notice('Não foi possível mudar a qualidade.', 'warn'));
  });
  $('preview').addEventListener('click', onPreviewTap);
  const fs = $('fullscreen');
  fs.hidden = !document.fullscreenEnabled;
  fs.addEventListener('click', async () => {
    try {
      if (document.fullscreenElement) {
        await document.exitFullscreen();
      } else {
        await document.documentElement.requestFullscreen();
        if (screen.orientation && screen.orientation.lock) await screen.orientation.lock('landscape').catch(() => {});
      }
    } catch { /* tela cheia indisponível */ }
  });

  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState !== 'visible' || !S.creds) return;
    requestWakeLock();
    if (!S.video || S.video.readyState === 'ended' || S.video.muted) reopenSoon();
    if (!S.controlConnected) control.restart();
    if (S.wantLive && S.streamState !== 'live') restartPublish();
  });
  window.addEventListener('online', onNetworkChange);
  window.addEventListener('offline', onNetworkChange);
  if (navigator.connection && navigator.connection.addEventListener) navigator.connection.addEventListener('change', onNetworkChange);
  window.addEventListener('pagehide', () => publisher.stop(true));
  window.addEventListener('beforeunload', (e) => {
    if (!S.wantLive) return;
    e.preventDefault(); // saída acidental derrubaria a câmera
    e.returnValue = '';
  });
}

// ---------------------------------------------------------------------------
// Início

async function startCamera() {
  document.body.dataset.screen = 'camera';
  $('rotate-hint').hidden = false;
  render();
  try {
    await openCapture();
  } catch (e) {
    showMessage(describeMediaError(e));
    return;
  }
  S.network = networkType();
  if (navigator.getBattery) {
    navigator.getBattery().then((bat) => {
      S.battery = bat;
      render();
    }).catch(() => {});
  }
  requestWakeLock();
  control.open();
  setInterval(tick, 2000);
  setInterval(render, 1000);
  if (load(sessionStorage, LIVE_KEY)) startStream(); // página recarregada durante a transmissão
}

function showPairing(info) {
  document.body.dataset.screen = 'pair';
  $('pair-server').textContent = info.serverName;
  const prev = load(localStorage, STORE_KEY);
  const name = $('pair-name');
  if (prev && prev.name) name.value = prev.name;
  const status = $('pair-status');
  if (info.exp && Date.now() / 1000 > info.exp + 60) {
    status.textContent = 'Este QR Code parece expirado. Se a conexão falhar, gere um novo no painel.';
  }
  const btn = $('pair-btn');
  btn.addEventListener('click', async () => {
    btn.disabled = true;
    status.textContent = 'Conectando…';
    try {
      S.creds = await pair(info, name.value.trim());
      save(localStorage, STORE_KEY, S.creds);
      save(localStorage, LENS_KEY, null);
      status.textContent = '';
      await startCamera();
    } catch (e) {
      status.textContent = e.code === 'pairing_invalid'
        ? 'QR Code inválido, expirado ou já usado. Gere um novo no painel (Câmeras → Adicionar câmera).'
        : e.message || String(e);
      btn.disabled = false;
    }
  });
}

async function main() {
  bindUi();
  if (!window.isSecureContext || !navigator.mediaDevices || !navigator.mediaDevices.getUserMedia || !window.RTCPeerConnection) {
    showMessage('Este navegador não permite usar a câmera aqui. Abra o link https do QR Code no Chrome (Android) ou no Safari (iPhone) atualizados.');
    return;
  }
  request('GET', '/device/v1/info', { timeoutMs: 5000 }).then(async (res) => {
    if (res.ok) {
      const j = await res.json();
      if (j.version) S.appVersion = `web-${j.version}`;
    }
  }).catch(() => {});

  const info = parseFragment(location.hash);
  if (location.hash) history.replaceState(null, '', location.pathname + location.search); // o token sai do histórico
  if (info) {
    showPairing(info);
    return;
  }
  S.creds = load(localStorage, STORE_KEY);
  if (S.creds && S.creds.deviceId && S.creds.secret) {
    await startCamera();
    return;
  }
  S.creds = null;
  showMessage('Este celular ainda não está conectado. No computador, abra o SacraLink → Câmeras → Adicionar câmera e leia o QR Code com a câmera do celular.');
}

main();
