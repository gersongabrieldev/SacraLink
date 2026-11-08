import { useEffect, useRef, useState } from 'react';
import {
  BatteryCharging, BatteryFull, BatteryLow, BatteryMedium, Bookmark, Flashlight, Mic, MicOff, MoreHorizontal, RefreshCw,
  SwitchCamera, Thermometer, ZoomIn, ZoomOut,
} from 'lucide-react';
import { ApiError } from '../lib/api';
import { useStore } from '../lib/store';
import { t } from '../lib/i18n';
import { formatBitrate, formatDuration, formatResolution, orientationMismatch, signalBars } from '../lib/format';
import type { CameraView, CommandResult, MediaProfile, Orientation, Preset } from '../lib/types';
import { WhepVideo } from './WhepVideo';
import { Confirm } from './ui';

function Battery({ level, charging }: { level?: number; charging: boolean }) {
  if (level === undefined) return null;
  const pct = Math.round(level * 100);
  const Icon = charging ? BatteryCharging : pct > 60 ? BatteryFull : pct > 25 ? BatteryMedium : BatteryLow;
  return (
    <span className={`inline-flex items-center gap-1 ${pct <= 15 && !charging ? 'text-onair' : ''}`} title="Bateria do celular">
      <Icon size={14} /> {pct}%
    </span>
  );
}

function Bars({ n }: { n: number }) {
  return (
    <span className="inline-flex items-end gap-0.5" title={`Qualidade do sinal: ${n}/4`} aria-label={`Sinal ${n} de 4`}>
      {[1, 2, 3, 4].map((i) => (
        <span key={i} className={`w-1 rounded-sm ${i <= n ? (n <= 1 ? 'bg-onair' : n <= 2 ? 'bg-warn' : 'bg-preview') : 'bg-line'}`} style={{ height: 3 + i * 3 }} />
      ))}
    </span>
  );
}

export function CameraCard({ cam, profiles, previewActive }: { cam: CameraView; profiles: MediaProfile[]; previewActive: boolean }) {
  const { api, state, dispatch, notify, run } = useStore();
  const [confirmProgram, setConfirmProgram] = useState(false);
  const [menu, setMenu] = useState(false);
  const [presets, setPresets] = useState<Preset[]>([]);
  const [presetName, setPresetName] = useState('');
  const [now, setNow] = useState(Date.now());
  const [zoom, setZoom] = useState(cam.zoom ?? 1);
  const zoomTimer = useRef<number>(undefined);
  const obsConnected = state.obs?.state === 'connected';
  const studio = !!state.obs?.studioMode;
  const lens = cam.capabilities?.lenses.find((l) => l.id === cam.lensId) ?? cam.capabilities?.lenses[0];
  const streaming = cam.state === 'STREAMING';
  const hasSource = cam.obsSource.state === 'present';

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);
  useEffect(() => {
    if (cam.zoom !== undefined) setZoom(cam.zoom);
  }, [cam.zoom]);

  const command = async (name: string, params: Record<string, unknown> = {}, force = false) => {
    const r = await run(() => api.post<CommandResult>(`/api/v1/cameras/${cam.id}/commands`, { name, params, force }));
    if (r && r.status !== 'executed') notify(r.status === 'conflict' ? 'warn' : 'error', `${cam.name}: ${t(`cmd.${r.status}`)}${r.message ? ` (${r.message})` : ''}`);
  };

  useEffect(() => {
    if (menu) api.get<Preset[]>(`/api/v1/cameras/${cam.id}/presets`).then(setPresets).catch(() => {});
  }, [menu, api, cam.id]);

  const applyPreset = async (p: Preset) => {
    const r = await run(() => api.post<CommandResult>(`/api/v1/cameras/${cam.id}/presets/${p.id}/apply`));
    if (r && r.status !== 'executed') notify(r.status === 'conflict' ? 'warn' : 'error', `${cam.name}: ${t(`cmd.${r.status}`)}`);
    else if (r) notify('success', `Preset "${p.name}" aplicado em ${cam.name}`);
  };
  const savePreset = async () => {
    const name = presetName.trim();
    if (!name) return;
    const created = await run(() => api.post<Preset>(`/api/v1/cameras/${cam.id}/presets`, { name, fromCurrent: true }), `Preset "${name}" salvo`);
    if (created) {
      setPresets((ps) => [...ps, created]);
      setPresetName('');
    }
  };
  const deletePreset = async (p: Preset) => {
    await run(() => api.del(`/api/v1/cameras/${cam.id}/presets/${p.id}`));
    setPresets((ps) => ps.filter((x) => x.id !== p.id));
  };

  // Orientação: o painel decide; o celular recebe na hora e pede ao operador para girar.
  const setOrientation = (orientation: Orientation) =>
    run(() => api.patch(`/api/v1/cameras/${cam.id}`, { orientation }),
      `${cam.name}: ${orientation === 'portrait' ? 'vertical' : 'horizontal'} — o celular vai pedir para girar`);
  const wrongPosition = streaming && orientationMismatch(cam);

  const sendZoom = (v: number) => {
    setZoom(v);
    window.clearTimeout(zoomTimer.current);
    zoomTimer.current = window.setTimeout(() => command('zoom.set', { value: v }), 150);
  };

  const toPreview = () => run(() => api.post('/api/v1/obs/preview', { cameraId: cam.id }));
  const toProgram = async (confirm = false) => {
    try {
      await api.post('/api/v1/obs/program', { cameraId: cam.id, confirm });
      setConfirmProgram(false);
    } catch (e) {
      if (e instanceof ApiError && e.code === 'confirmation_required') setConfirmProgram(true);
      else notify('error', (e as Error).message);
    }
  };

  const border = cam.program ? 'border-onair ring-2 ring-onair/60' : cam.preview ? 'border-preview ring-2 ring-preview/50' : 'border-line';
  const selected = state.selected === cam.id;

  return (
    <article className={`card relative flex flex-col overflow-hidden border-2 ${border}`} onClick={() => dispatch({ type: 'select', id: cam.id })}>
      <div className="relative">
        <WhepVideo cameraId={cam.id} active={previewActive && streaming} />
        <div className="absolute top-2 left-2 flex gap-1.5">
          {cam.program && <span className="rounded bg-onair px-2 py-0.5 text-xs font-bold text-white">● {t('tally.on_air')}</span>}
          {cam.preview && <span className="rounded bg-preview px-2 py-0.5 text-xs font-bold text-white">{t('tally.preview')}</span>}
          {!streaming && <span className="rounded bg-black/70 px-2 py-0.5 text-xs font-semibold text-white">{t(`state.${cam.state}`)}</span>}
        </div>
        <div className="absolute top-2 right-2 flex gap-1.5">
          {cam.orientation === 'portrait' && <span className="rounded bg-black/70 px-2 py-0.5 text-xs font-semibold text-white" title="Câmera vertical (9:16)">Vertical</span>}
          <span className="rounded bg-black/70 px-2 py-0.5 text-xs font-bold text-white">{String(cam.slot).padStart(2, '0')}</span>
        </div>
        {cam.error ? (
          <div className="absolute inset-x-2 bottom-2 rounded bg-onair/90 px-2 py-1 text-xs text-white">{cam.error}</div>
        ) : wrongPosition && (
          <div className="absolute inset-x-2 bottom-2 rounded bg-warn/90 px-2 py-1 text-xs font-semibold text-black">
            Celular na posição errada: peça para girar na {cam.orientation === 'portrait' ? 'vertical' : 'horizontal'}
          </div>
        )}
      </div>

      <div className="flex flex-1 flex-col gap-2 p-3">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <div className={`truncate font-semibold ${selected ? 'text-accent' : ''}`}>{cam.name}</div>
            <div className="truncate text-xs text-muted">
              {cam.operatorName || cam.device?.name || 'Sem aparelho'}
              {cam.isBackup && ' · reserva'}
            </div>
          </div>
          <div className="flex items-center gap-2 text-xs text-muted">
            {cam.thermal && cam.thermal !== 'nominal' && (
              <span className={cam.thermal === 'critical' || cam.thermal === 'serious' ? 'text-onair' : 'text-warn'} title={`Temperatura: ${cam.thermal}`}>
                <Thermometer size={14} />
              </span>
            )}
            <Battery level={cam.battery} charging={cam.charging} />
            <Bars n={signalBars(cam)} />
          </div>
        </div>

        <dl className="grid grid-cols-4 gap-1 text-[11px]">
          <div><dt className="text-muted">Resolução</dt><dd className="font-medium">{formatResolution(cam.width, cam.height, cam.fps)}</dd></div>
          <div><dt className="text-muted">Bitrate</dt><dd className="font-medium">{formatBitrate(cam.bitrateKbps)}</dd></div>
          <div><dt className="text-muted">Tempo</dt><dd className="font-medium tabular-nums">{formatDuration(cam.streamingSince ?? cam.connectedSince, now)}</dd></div>
          <div>
            <dt className="text-muted">Áudio</dt>
            <dd className="inline-flex items-center gap-1 font-medium">
              {cam.micMuted || !cam.hasAudio ? <MicOff size={12} className="text-warn" /> : <Mic size={12} className="text-preview" />}
              {cam.micMuted ? 'mudo' : cam.hasAudio ? 'ok' : '—'}
            </dd>
          </div>
        </dl>

        <div className="text-[11px] text-muted">
          OBS:{' '}
          {cam.obsSource.state === 'present' ? (
            <span className="text-fg">{cam.obsSource.inputName}</span>
          ) : cam.obsSource.state === 'missing' ? (
            <span className="text-warn">fonte removida no OBS</span>
          ) : (
            'sem fonte'
          )}
        </div>

        {lens && cam.controlConnected && (
          <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
            <button className="text-muted hover:text-fg" onClick={() => sendZoom(Math.max(lens.minZoom, +(zoom - 0.5).toFixed(1)))} aria-label="Afastar">
              <ZoomOut size={16} />
            </button>
            <input
              type="range" min={lens.minZoom} max={lens.maxZoom} step={0.1} value={zoom}
              onChange={(e) => sendZoom(+e.target.value)} className="flex-1 accent-[var(--accent)]" aria-label="Zoom"
            />
            <button className="text-muted hover:text-fg" onClick={() => sendZoom(Math.min(lens.maxZoom, +(zoom + 0.5).toFixed(1)))} aria-label="Aproximar">
              <ZoomIn size={16} />
            </button>
            <span className="w-10 text-right text-xs tabular-nums">{zoom.toFixed(1)}×</span>
          </div>
        )}

        <div className="mt-auto flex flex-wrap items-center gap-1.5" onClick={(e) => e.stopPropagation()}>
          <button className="btn-preview flex-1" disabled={!obsConnected || !hasSource || !studio} onClick={toPreview} title={studio ? 'Enviar para o Preview' : 'Ative o Studio Mode no OBS para usar o Preview'}>
            Preview
          </button>
          <button className="btn-danger flex-1" disabled={!obsConnected || !hasSource} onClick={() => toProgram(false)} title={studio ? 'Levar ao ar com a transição atual' : 'Troca direta no Program'}>
            {studio ? 'Cortar' : 'No ar'}
          </button>
          <div className="relative">
            <button className="btn-ghost px-2" onClick={() => setMenu((m) => !m)} aria-label="Mais ações">
              <MoreHorizontal size={16} />
            </button>
            {menu && (
              <div className="card absolute right-0 bottom-10 z-20 w-56 p-1 text-sm shadow-xl" onMouseLeave={() => setMenu(false)}>
                {cam.capabilities && cam.capabilities.lenses.length > 1 && (
                  <div className="px-2 py-1">
                    <div className="label">Lente</div>
                    <select className="input py-1" value={cam.lensId} onChange={(e) => command('lens.select', { lensId: e.target.value })}>
                      {cam.capabilities.lenses.map((l) => (
                        <option key={l.id} value={l.id}>{l.label}</option>
                      ))}
                    </select>
                  </div>
                )}
                <div className="px-2 py-1">
                  <div className="label">Orientação</div>
                  <select className="input py-1" value={cam.orientation} onChange={(e) => setOrientation(e.target.value as Orientation)}>
                    <option value="landscape">Horizontal (16:9)</option>
                    <option value="portrait">Vertical (9:16)</option>
                  </select>
                </div>
                <div className="px-2 py-1">
                  <div className="label">Qualidade</div>
                  <select className="input py-1" value={cam.profileId} onChange={(e) => command('quality.set', { profileId: e.target.value })}>
                    {profiles.map((p) => (
                      <option key={p.id} value={p.id}>{p.name}</option>
                    ))}
                  </select>
                </div>
                <div className="px-2 py-1">
                  <div className="label">Presets</div>
                  {presets.map((p) => (
                    <div key={p.id} className="flex items-center gap-1 py-0.5">
                      <button
                        className="flex flex-1 items-center gap-1.5 truncate rounded px-1 py-1 text-left hover:bg-surface-2 disabled:opacity-40"
                        disabled={!cam.controlConnected}
                        onClick={() => applyPreset(p)}
                        title={p.params.zoom ? `Aplicar (zoom ${p.params.zoom.toFixed(1)}×)` : 'Aplicar'}
                      >
                        <Bookmark size={13} /> <span className="truncate">{p.name}</span>
                      </button>
                      <button className="px-1 text-muted hover:text-onair" onClick={() => deletePreset(p)} aria-label={`Remover preset ${p.name}`}>
                        ×
                      </button>
                    </div>
                  ))}
                  <div className="mt-1 flex gap-1">
                    <input
                      className="input py-1 text-xs" placeholder="Nome (ex.: plano fechado)" maxLength={40} value={presetName}
                      onChange={(e) => setPresetName(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && savePreset()}
                    />
                    <button className="btn-ghost px-2 text-xs" disabled={!cam.controlConnected || !presetName.trim()} onClick={savePreset} title="Salvar a posição atual da câmera">
                      Salvar
                    </button>
                  </div>
                </div>
                <MenuItem icon={<SwitchCamera size={14} />} label="Alternar frontal/traseira" onClick={() => command('camera.flip')} disabled={!cam.controlConnected} />
                {cam.capabilities?.torch && <MenuItem icon={<Flashlight size={14} />} label={cam.torch ? 'Desligar lanterna' : 'Ligar lanterna'} onClick={() => command('torch.set', { on: !cam.torch })} />}
                <MenuItem icon={cam.micMuted ? <Mic size={14} /> : <MicOff size={14} />} label={cam.micMuted ? 'Ativar microfone' : 'Silenciar microfone'} onClick={() => command('audio.mute', { muted: !cam.micMuted })} disabled={!cam.controlConnected} />
                <MenuItem icon={<RefreshCw size={14} />} label="Pedir reconexão" onClick={() => command('stream.reconnect')} disabled={!cam.controlConnected} />
                {cam.obsSource.state === 'missing' && (
                  <MenuItem icon={<RefreshCw size={14} />} label="Recriar fonte no OBS" onClick={() => run(() => api.post(`/api/v1/obs/cameras/${cam.id}/source`, { recreate: true }), 'Fonte recriada no OBS')} />
                )}
              </div>
            )}
          </div>
        </div>
      </div>

      <Confirm
        open={confirmProgram}
        title="Levar direto ao ar?"
        danger
        confirmLabel="Colocar no ar agora"
        message={<>O <b>Studio Mode</b> do OBS está desligado: a câmera <b>{cam.name}</b> aparecerá imediatamente na transmissão, sem passar pelo Preview.</>}
        onConfirm={() => toProgram(true)}
        onCancel={() => setConfirmProgram(false)}
      />
    </article>
  );
}

function MenuItem({ icon, label, onClick, disabled }: { icon: React.ReactNode; label: string; onClick: () => void; disabled?: boolean }) {
  return (
    <button className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-surface-2 disabled:opacity-40" onClick={onClick} disabled={disabled}>
      {icon} {label}
    </button>
  );
}
