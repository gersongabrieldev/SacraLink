import { useCallback, useEffect, useState } from 'react';
import { Circle, Mic, MicOff, Play, RefreshCw, Square } from 'lucide-react';
import { useStore } from '../lib/store';
import { ApiError } from '../lib/api';
import { formatBytes, formatDuration } from '../lib/format';
import type { SceneInfo, Settings } from '../lib/types';
import { Confirm, Section, Toggle } from '../components/ui';

interface AudioInput { inputName: string; inputKind: string; cameraId?: string; muted: boolean; volumeDb: number }
interface Outputs {
  stream: { outputActive: boolean; outputReconnecting?: boolean; outputDuration: number; outputSkippedFrames?: number; outputTotalFrames?: number; outputCongestion?: number; outputBytes: number };
  record: { outputActive: boolean; outputDuration: number; outputBytes: number };
  stats: { cpuUsage: number; memoryUsage: number; activeFps: number; renderSkippedFrames: number; renderTotalFrames: number; outputSkippedFrames: number; outputTotalFrames: number };
  destination?: { type: string; service?: string; server?: string };
}
interface OBSConfig { host: string; port: number; hasPassword: boolean; autoConnect: boolean; lastObsVersion?: string }

function Meter({ db, clipping }: { db: number; clipping: boolean }) {
  const pct = Math.max(0, Math.min(100, ((db + 60) / 60) * 100));
  return (
    <div className="h-2 w-full overflow-hidden rounded bg-surface-2" title={`${db.toFixed(1)} dBFS`}>
      <div className={`h-full ${clipping ? 'bg-onair' : db > -9 ? 'bg-warn' : 'bg-preview'}`} style={{ width: `${pct}%` }} />
    </div>
  );
}

export function OBSPage() {
  const { api, state, notify, run } = useStore();
  const obs = state.obs;
  const connected = obs?.state === 'connected';
  const [cfg, setCfg] = useState<OBSConfig | null>(null);
  const [password, setPassword] = useState('');
  const [scenes, setScenes] = useState<SceneInfo[]>([]);
  const [transitions, setTransitions] = useState<{ currentSceneTransitionName: string; transitions: { transitionName: string }[] } | null>(null);
  const [outputs, setOutputs] = useState<Outputs | null>(null);
  const [audio, setAudio] = useState<AudioInput[]>([]);
  const [confirm, setConfirm] = useState<{ title: string; message: string; action: () => Promise<unknown> } | null>(null);
  const settings = state.settings;

  const refresh = useCallback(() => {
    api.get<OBSConfig>('/api/v1/obs/config').then(setCfg).catch(() => {});
    if (!connected) return;
    api.get<SceneInfo[]>('/api/v1/obs/scenes').then(setScenes).catch(() => {});
    api.get<typeof transitions>('/api/v1/obs/transitions').then(setTransitions).catch(() => {});
    api.get<Outputs>('/api/v1/obs/outputs').then(setOutputs).catch(() => {});
    api.get<AudioInput[]>('/api/v1/obs/audio').then(setAudio).catch(() => {});
  }, [api, connected]);

  useEffect(() => {
    refresh();
    const id = window.setInterval(() => connected && api.get<Outputs>('/api/v1/obs/outputs').then(setOutputs).catch(() => {}), 3000);
    return () => window.clearInterval(id);
  }, [refresh, connected, api, obs?.programScene, obs?.previewScene]);

  const saveConfig = () =>
    cfg && run(() => api.put('/api/v1/obs/config', { host: cfg.host, port: cfg.port, autoConnect: cfg.autoConnect, ...(password ? { password } : {}) }), 'Configuração do OBS salva');

  const program = async (sceneName: string, confirmed = false) => {
    try {
      await api.post('/api/v1/obs/program', { sceneName, confirm: confirmed });
    } catch (e) {
      if (e instanceof ApiError && e.code === 'confirmation_required')
        setConfirm({ title: 'Levar direto ao ar?', message: `Com o Studio Mode desligado, a cena "${sceneName}" vai imediatamente para a transmissão.`, action: () => program(sceneName, true) });
      else notify('error', (e as Error).message);
    }
  };

  const output = (kind: 'stream' | 'record', start: boolean) =>
    setConfirm({
      title: start ? (kind === 'stream' ? 'Iniciar transmissão?' : 'Iniciar gravação?') : kind === 'stream' ? 'Encerrar transmissão?' : 'Parar gravação?',
      message: start
        ? kind === 'stream' ? `O OBS começará a transmitir para ${outputs?.destination?.service || 'o destino configurado'}.` : 'O OBS começará a gravar o Program.'
        : kind === 'stream' ? 'A transmissão ao vivo será encerrada para todos os espectadores.' : 'A gravação será finalizada.',
      action: () => api.post(`/api/v1/obs/outputs/${kind}`, { action: start ? 'start' : 'stop', confirm: true }),
    });

  const updateSettings = (f: (s: Settings) => Settings) =>
    settings && run(() => api.put('/api/v1/settings', f(settings)), 'Configurações salvas');

  const setAudioInput = (name: string, body: Record<string, unknown>) =>
    run(async () => {
      await api.patch(`/api/v1/obs/audio/${encodeURIComponent(name)}`, body);
      setAudio(await api.get<AudioInput[]>('/api/v1/obs/audio'));
    });

  const camName = (id: string) => state.cameras[id]?.name ?? id;

  return (
    <div className="grid gap-4 p-4 xl:grid-cols-2">
      <Section title="Conexão com o OBS" actions={<span className={`text-sm ${connected ? 'text-preview' : 'text-warn'}`}>{connected ? `OBS ${obs?.obsVersion} · WebSocket ${obs?.wsVersion}` : obs?.error || obs?.state}</span>}>
        {cfg && (
          <div className="space-y-3">
            <div className="grid gap-3 sm:grid-cols-[1fr_100px_1fr]">
              <div><label className="label">Endereço</label><input className="input" value={cfg.host} onChange={(e) => setCfg({ ...cfg, host: e.target.value })} /></div>
              <div><label className="label">Porta</label><input className="input" type="number" value={cfg.port} onChange={(e) => setCfg({ ...cfg, port: +e.target.value })} /></div>
              <div><label className="label">Senha {cfg.hasPassword && '(salva)'}</label><input className="input" type="password" placeholder={cfg.hasPassword ? '••••••••' : ''} value={password} onChange={(e) => setPassword(e.target.value)} /></div>
            </div>
            <Toggle checked={cfg.autoConnect} onChange={(v) => setCfg({ ...cfg, autoConnect: v })} label="Conectar automaticamente e reconectar se o OBS reiniciar" />
            <div className="flex flex-wrap gap-2">
              <button className="btn-primary" onClick={saveConfig}>Salvar e conectar</button>
              <button className="btn-ghost" onClick={() => run(() => api.post('/api/v1/obs/config/import'), 'Configuração importada do obs-websocket deste computador')}>Importar do OBS deste PC</button>
            </div>
            <p className="text-xs text-muted">A senha é cifrada com a proteção de dados do Windows (DPAPI). O obs-websocket nunca é exposto na rede.</p>
          </div>
        )}
      </Section>

      <Section title="Transmissão e gravação" actions={<button className="btn-ghost px-2" onClick={refresh} aria-label="Atualizar"><RefreshCw size={14} /></button>}>
        {outputs ? (
          <div className="space-y-3 text-sm">
            <div className="flex flex-wrap items-center gap-3">
              <span className={`pill ${outputs.stream.outputActive ? 'border-onair bg-onair text-white' : ''}`}>
                <Circle size={10} className={outputs.stream.outputActive ? 'pulse-dot fill-current' : ''} />
                {outputs.stream.outputActive ? `AO VIVO ${formatDuration(Date.now() - outputs.stream.outputDuration)}` : 'Transmissão parada'}
              </span>
              {outputs.stream.outputReconnecting && <span className="text-warn">reconectando ao destino…</span>}
              <button className={outputs.stream.outputActive ? 'btn-ghost' : 'btn-danger'} onClick={() => output('stream', !outputs.stream.outputActive)}>
                {outputs.stream.outputActive ? <><Square size={14} /> Encerrar</> : <><Play size={14} /> Iniciar transmissão</>}
              </button>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              <span className={`pill ${outputs.record.outputActive ? 'border-onair text-onair' : ''}`}>
                {outputs.record.outputActive ? `GRAVANDO ${formatDuration(Date.now() - outputs.record.outputDuration)} · ${formatBytes(outputs.record.outputBytes)}` : 'Gravação parada'}
              </span>
              <button className="btn-ghost" onClick={() => output('record', !outputs.record.outputActive)}>{outputs.record.outputActive ? 'Parar gravação' : 'Gravar'}</button>
            </div>
            <dl className="grid grid-cols-2 gap-2 text-xs sm:grid-cols-4">
              <div><dt className="text-muted">Destino</dt><dd>{outputs.destination?.service || outputs.destination?.type || '—'}</dd></div>
              <div><dt className="text-muted">FPS do OBS</dt><dd>{outputs.stats.activeFps.toFixed(1)}</dd></div>
              <div><dt className="text-muted">CPU do OBS</dt><dd>{outputs.stats.cpuUsage.toFixed(1)}%</dd></div>
              <div><dt className="text-muted">Quadros perdidos</dt><dd className={outputs.stats.outputSkippedFrames + outputs.stats.renderSkippedFrames > 0 ? 'text-warn' : ''}>{outputs.stats.renderSkippedFrames} render · {outputs.stats.outputSkippedFrames} saída</dd></div>
            </dl>
            <p className="text-xs text-muted">O OBS continua responsável pela transmissão para YouTube, Facebook etc. A chave de transmissão nunca é lida pelo SacraLink.</p>
          </div>
        ) : <p className="text-sm text-muted">Conecte o OBS para ver o estado da transmissão.</p>}
      </Section>

      <Section title="Cenas" actions={transitions && (
        <select className="input w-44 py-1" value={transitions.currentSceneTransitionName} onChange={(e) => run(() => api.put('/api/v1/obs/transitions/current', { name: e.target.value }))} aria-label="Transição">
          {transitions.transitions.map((t) => <option key={t.transitionName}>{t.transitionName}</option>)}
        </select>
      )}>
        <ul className="divide-y divide-line">
          {scenes.map((s) => (
            <li key={s.name} className="flex flex-wrap items-center gap-2 py-2">
              <span className="min-w-0 flex-1 truncate text-sm">
                {s.program && <span className="mr-1 font-bold text-onair">●</span>}
                {s.preview && <span className="mr-1 font-bold text-preview">●</span>}
                {s.name}
                {s.cameraIds && s.cameraIds.length > 0 && <span className="ml-2 text-xs text-muted">{s.cameraIds.map(camName).join(', ')}</span>}
              </span>
              <button className="btn-preview py-1 text-xs" disabled={!obs?.studioMode} onClick={() => run(() => api.post('/api/v1/obs/preview', { sceneName: s.name }))}>Preview</button>
              <button className="btn-danger py-1 text-xs" onClick={() => program(s.name)}>{obs?.studioMode ? 'Cortar' : 'No ar'}</button>
            </li>
          ))}
          {scenes.length === 0 && <li className="py-4 text-sm text-muted">Conecte o OBS para listar as cenas.</li>}
        </ul>
        {connected && (
          <button className="btn-ghost mt-3" onClick={() => run(() => api.post('/api/v1/obs/sources/sync'), 'Fontes sincronizadas')}>Criar fontes que faltam</button>
        )}
      </Section>

      <Section title="Áudio (mixer do OBS)">
        <ul className="space-y-3">
          {audio.map((a) => {
            const lvl = state.audio[a.inputName];
            const main = settings?.audio.mainInput === a.inputName;
            const backup = settings?.audio.backupInput === a.inputName;
            return (
              <li key={a.inputName} className="space-y-1">
                <div className="flex items-center gap-2 text-sm">
                  <button className={a.muted ? 'text-onair' : 'text-preview'} onClick={() => setAudioInput(a.inputName, { muted: !a.muted })} aria-label={a.muted ? 'Ativar' : 'Silenciar'}>
                    {a.muted ? <MicOff size={16} /> : <Mic size={16} />}
                  </button>
                  <span className="flex-1 truncate">{a.inputName}{main && <span className="ml-1 text-xs text-accent">principal</span>}{backup && <span className="ml-1 text-xs text-muted">reserva</span>}</span>
                  {lvl?.clipping && <span className="text-xs font-bold text-onair">CLIP</span>}
                  <input type="range" min={-60} max={6} step={0.5} defaultValue={a.volumeDb} className="w-28 accent-[var(--accent)]"
                    onMouseUp={(e) => setAudioInput(a.inputName, { volumeDb: +(e.target as HTMLInputElement).value })} aria-label={`Volume de ${a.inputName}`} />
                  <span className="w-14 text-right text-xs tabular-nums">{a.volumeDb.toFixed(1)} dB</span>
                </div>
                <Meter db={a.muted ? -100 : lvl?.peakDb ?? -100} clipping={!!lvl?.clipping} />
                <div className="flex gap-2 text-xs">
                  <button className="text-muted hover:text-accent" onClick={() => updateSettings((s) => ({ ...s, audio: { ...s.audio, mainInput: a.inputName } }))}>definir como principal</button>
                  <button className="text-muted hover:text-accent" onClick={() => updateSettings((s) => ({ ...s, audio: { ...s.audio, backupInput: a.inputName } }))}>definir como reserva</button>
                </div>
              </li>
            );
          })}
          {audio.length === 0 && <li className="text-sm text-muted">Nenhuma fonte de áudio (conecte o OBS).</li>}
        </ul>
        {settings?.audio.mainInput && settings.audio.backupInput && (
          <button className="btn-ghost mt-3" onClick={() => setConfirm({
            title: 'Trocar para o áudio reserva?', message: `"${settings.audio.mainInput}" será silenciado e "${settings.audio.backupInput}" ativado.`,
            action: async () => {
              await api.patch(`/api/v1/obs/audio/${encodeURIComponent(settings.audio.mainInput)}`, { muted: true });
              await api.patch(`/api/v1/obs/audio/${encodeURIComponent(settings.audio.backupInput)}`, { muted: false });
            },
          })}>Usar áudio reserva</button>
        )}
        <p className="mt-3 text-xs text-muted">Nunca reproduza o retorno de áudio no alto-falante do celular que está captando: isso causa microfonia.</p>
      </Section>

      {settings && (
        <Section title="Segurança das trocas e fallback">
          <div className="space-y-2">
            <Toggle checked={settings.obs.autoManageSources} onChange={(v) => updateSettings((s) => ({ ...s, obs: { ...s.obs, autoManageSources: v } }))}
              label="Criar fontes e cenas SacraLink automaticamente" hint="Fontes e cenas criadas por você nunca são alteradas ou apagadas." />
            <Toggle checked={settings.obs.allowDirectSwitch} onChange={(v) => updateSettings((s) => ({ ...s, obs: { ...s.obs, allowDirectSwitch: v } }))}
              label="Permitir trocas diretas no Program sem confirmação (Studio Mode desligado)" hint="Não recomendado: uma troca acidental vai direto ao ar." />
            <div className="grid gap-3 pt-2 sm:grid-cols-3">
              <div>
                <label className="label">Se a câmera NO AR cair</label>
                <select className="input" value={settings.fallback.mode} onChange={(e) => updateSettings((s) => ({ ...s, fallback: { ...s.fallback, mode: e.target.value as Settings['fallback']['mode'] } }))}>
                  <option value="stay">Permanecer na cena (padrão)</option>
                  <option value="backup">Trocar para a câmera reserva</option>
                  <option value="scene">Trocar para uma cena</option>
                </select>
              </div>
              {settings.fallback.mode === 'scene' && (
                <div>
                  <label className="label">Cena (imagem institucional/padrão)</label>
                  <select className="input" value={settings.fallback.sceneName} onChange={(e) => updateSettings((s) => ({ ...s, fallback: { ...s.fallback, sceneName: e.target.value } }))}>
                    <option value="">Escolha…</option>
                    {scenes.map((s) => <option key={s.name}>{s.name}</option>)}
                  </select>
                </div>
              )}
              <div>
                <label className="label">Aguardar antes de trocar (s)</label>
                <input className="input" type="number" min={1} max={60} value={settings.fallback.graceSec}
                  onChange={(e) => updateSettings((s) => ({ ...s, fallback: { ...s.fallback, graceSec: +e.target.value } }))} />
              </div>
            </div>
          </div>
        </Section>
      )}

      <Confirm open={!!confirm} title={confirm?.title ?? ''} message={confirm?.message} danger
        onCancel={() => setConfirm(null)}
        onConfirm={async () => { const c = confirm; setConfirm(null); if (c) await run(c.action); refresh(); }} />
    </div>
  );
}
