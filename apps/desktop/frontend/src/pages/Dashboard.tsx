import { useEffect, useState } from 'react';
import { ArrowRightLeft, QrCode, Smartphone } from 'lucide-react';
import { useStore } from '../lib/store';
import { sortedCameras } from '../lib/state';
import { gridColumns } from '../lib/format';
import type { MediaProfile } from '../lib/types';
import { CameraCard } from '../components/CameraCard';
import { Empty } from '../components/ui';
import { PairingModal } from './Devices';

export function Dashboard() {
  const { api, state, run } = useStore();
  const [profiles, setProfiles] = useState<MediaProfile[]>([]);
  const [pairing, setPairing] = useState(false);
  const cams = sortedCameras(state);
  const obs = state.obs;
  const mode = state.settings?.previewMode ?? 'all';

  useEffect(() => {
    api.get<MediaProfile[]>('/api/v1/profiles').then(setProfiles).catch(() => {});
  }, [api]);

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <div className="pill border-onair/50"><span className="font-bold text-onair">PROGRAM</span> {obs?.programScene ?? '—'}</div>
        {obs?.studioMode && <div className="pill border-preview/50"><span className="font-bold text-preview">PREVIEW</span> {obs.previewScene ?? '—'}</div>}
        {obs?.studioMode && (
          <button className="btn-primary" disabled={obs.state !== 'connected'} onClick={() => run(() => api.post('/api/v1/obs/transition'))}>
            <ArrowRightLeft size={16} /> Transição
          </button>
        )}
        {obs && !obs.studioMode && obs.state === 'connected' && (
          <span className="text-xs text-warn">Studio Mode desligado: as trocas vão direto ao ar e exigem confirmação.</span>
        )}
        <div className="ml-auto flex items-center gap-2 text-xs text-muted">
          Prévia: {mode === 'all' ? 'todas as câmeras' : mode === 'selected' ? 'somente a selecionada' : 'desligada'}
          <button className="btn-ghost" onClick={() => setPairing(true)}>
            <QrCode size={16} /> Parear celular
          </button>
        </div>
      </div>

      {cams.length === 0 ? (
        <div className="card flex-1">
          <Empty icon={<Smartphone size={40} />} title="Nenhuma câmera pareada">
            Clique em <b>Parear celular</b> e aponte a câmera do celular para o QR Code: a câmera abre no navegador, sem instalar aplicativo.
          </Empty>
        </div>
      ) : (
        <div className={`grid flex-1 auto-rows-min gap-3 overflow-auto ${gridColumns(cams.length)}`}>
          {cams.map((c) => (
            <CameraCard key={c.id} cam={c} profiles={profiles} previewActive={mode === 'all' || (mode === 'selected' && state.selected === c.id)} />
          ))}
        </div>
      )}
      <PairingModal open={pairing} onClose={() => setPairing(false)} />
    </div>
  );
}
