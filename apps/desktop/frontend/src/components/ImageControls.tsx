import { useEffect, useRef, useState } from 'react';
import { Copy } from 'lucide-react';
import { useStore } from '../lib/store';
import type { CameraView, CommandResult } from '../lib/types';

/** Temperaturas de cor comuns em igrejas (só as que a câmera aceita aparecem). */
const TEMPERATURES: { k: number; label: string }[] = [
  { k: 2700, label: '2700 K · velas, lâmpada amarela' },
  { k: 3200, label: '3200 K · halógena, refletor de palco' },
  { k: 4000, label: '4000 K · fluorescente, LED neutro' },
  { k: 5600, label: '5600 K · luz do dia' },
  { k: 6500, label: '6500 K · dia nublado, LED frio' },
];

export function hasImageControls(cam: CameraView): boolean {
  const c = cam.capabilities;
  return !!(c && (c.exposureLock || c.exposureCompensation || c.whiteBalanceLock));
}

const formatEv = (v: number) => `${v > 0 ? '+' : ''}${v.toFixed(1).replace('.', ',')}`;

/** Exposição e cor de uma câmera: travar impede o celular de "puxar" brilho e
 *  cor sozinho (vitrais, velas, projetor); copiar para as outras câmeras faz
 *  as imagens combinarem no corte. */
export function ImageControls({ cam, command }: { cam: CameraView; command: (name: string, params?: Record<string, unknown>) => Promise<void> }) {
  const { api, state, notify } = useStore();
  const caps = cam.capabilities!;
  const disabled = !cam.controlConnected;
  const [ev, setEv] = useState(cam.exposureCompensation ?? 0);
  const evTimer = useRef<number>(undefined);
  useEffect(() => setEv(cam.exposureCompensation ?? 0), [cam.exposureCompensation]);

  const range = caps.colorTemperature;
  const temps = range ? TEMPERATURES.filter((t) => t.k >= range.min && t.k <= range.max) : [];
  const k = cam.colorTemperature ?? 0;
  const wbValue = !cam.whiteBalanceLocked ? 'auto' : k > 0 ? String(k) : 'lock';

  const onWb = (v: string) => {
    if (v === 'auto') command('whitebalance.lock', { locked: false });
    else if (v === 'lock') command('whitebalance.lock', { locked: true });
    else command('whitebalance.temperature', { kelvin: Number(v) });
  };

  const onEv = (v: number) => {
    setEv(v);
    window.clearTimeout(evTimer.current);
    evTimer.current = window.setTimeout(() => command('exposure.compensation', { value: v }), 200);
  };

  // Copia brilho, trava de exposição e temperatura de cor para as outras
  // câmeras conectadas que aceitam cada ajuste.
  const copyToOthers = async () => {
    const others = Object.values(state.cameras).filter((c) => c.id !== cam.id && c.controlConnected && hasImageControls(c));
    let applied = 0;
    let failed = 0;
    const send = async (id: string, name: string, params: Record<string, unknown>) => {
      try {
        const r = await api.post<CommandResult>(`/api/v1/cameras/${id}/commands`, { name, params, force: false });
        if (r.status === 'executed') applied++;
        else failed++;
      } catch {
        failed++;
      }
    };
    for (const o of others) {
      const oc = o.capabilities!;
      if (caps.exposureCompensation && oc.exposureCompensation) await send(o.id, 'exposure.compensation', { value: cam.exposureCompensation });
      if (caps.exposureLock && oc.exposureLock) await send(o.id, 'exposure.lock', { locked: cam.exposureLocked });
      if (k > 0 && oc.colorTemperature) await send(o.id, 'whitebalance.temperature', { kelvin: k });
      else if (!cam.whiteBalanceLocked && oc.whiteBalanceLock) await send(o.id, 'whitebalance.lock', { locked: false });
    }
    if (others.length === 0) notify('warn', 'Nenhuma outra câmera conectada aceita esses ajustes.');
    else if (failed) notify('warn', `Ajustes copiados com ${failed} falha(s): confira cada câmera.`);
    else if (applied) notify('success', `Exposição e cor copiadas para ${others.length} câmera(s).`);
  };

  return (
    <div className="px-2 py-1">
      <div className="label">Exposição e cor</div>
      {caps.exposureLock && (
        <label className="flex items-center justify-between gap-2 py-1 text-xs">
          <span title="Impede o celular de clarear/escurecer sozinho quando alguém passa na frente de uma janela ou do projetor">Travar exposição</span>
          <input type="checkbox" checked={cam.exposureLocked} disabled={disabled} onChange={(e) => command('exposure.lock', { locked: e.target.checked })} />
        </label>
      )}
      {caps.exposureCompensation && (
        <div className="py-1">
          <div className="flex justify-between text-xs"><span>Brilho</span><span className="tabular-nums">{formatEv(ev)} EV</span></div>
          <input
            type="range" className="w-full" aria-label="Compensação de exposição"
            min={caps.exposureCompensation.min} max={caps.exposureCompensation.max} step={caps.exposureCompensation.step}
            value={ev} disabled={disabled} onChange={(e) => onEv(Number(e.target.value))}
          />
        </div>
      )}
      {caps.whiteBalanceLock && (
        <div className="py-1">
          <div className="mb-0.5 text-xs" title="Balanço de branco: com a mesma temperatura em todas as câmeras, as cores combinam no corte">Cor</div>
          <select className="input py-1 text-xs" value={wbValue} disabled={disabled} onChange={(e) => onWb(e.target.value)}>
            <option value="auto">Automática</option>
            <option value="lock">Travada na cor atual</option>
            {temps.map((t) => <option key={t.k} value={String(t.k)}>{t.label}</option>)}
            {k > 0 && !temps.some((t) => t.k === k) && <option value={String(k)}>{k} K</option>}
          </select>
        </div>
      )}
      <button className="mt-1 flex w-full items-center gap-1.5 rounded px-1 py-1 text-left text-xs hover:bg-surface-2 disabled:opacity-40" disabled={disabled} onClick={copyToOthers}
        title="Aplica o mesmo brilho, a mesma trava de exposição e a mesma temperatura de cor nas outras câmeras conectadas">
        <Copy size={13} /> Copiar para as outras câmeras
      </button>
    </div>
  );
}
