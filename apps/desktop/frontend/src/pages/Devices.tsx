import { useCallback, useEffect, useState } from 'react';
import { Ban, Pencil, QrCode, Trash2, Unplug, Undo2, UserX } from 'lucide-react';
import { useStore } from '../lib/store';
import { sortedCameras } from '../lib/state';
import { formatTime } from '../lib/format';
import { t } from '../lib/i18n';
import type { CameraView, Device, MediaProfile, Orientation } from '../lib/types';
import { Confirm, Modal, Section, Toggle } from '../components/ui';

/** QR Code de pareamento com contagem regressiva (token de uso único, 5 min). */
export function PairingModal({ open, onClose, cameraId }: { open: boolean; onClose: () => void; cameraId?: string }) {
  const { api, notify } = useStore();
  const [qr, setQr] = useState<{ qrPng: string; expiresAt: number; uri: string } | null>(null);
  const [now, setNow] = useState(Date.now());

  const generate = useCallback(async () => {
    try {
      setQr(await api.post('/api/v1/devices/pair', { cameraId: cameraId ?? '' }));
    } catch (e) {
      notify('error', (e as Error).message);
      onClose();
    }
  }, [api, cameraId, notify, onClose]);

  useEffect(() => {
    if (!open) return setQr(null);
    generate();
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [open, generate]);

  const left = qr ? Math.max(0, Math.floor((qr.expiresAt - now) / 1000)) : 0;
  return (
    <Modal open={open} title={cameraId ? 'Substituir aparelho da câmera' : 'Parear novo celular'} onClose={onClose}>
      <div className="flex flex-col items-center gap-3 text-center">
        {qr && left > 0 ? (
          <>
            <img src={qr.qrPng} alt="QR Code de pareamento" className="h-64 w-64 rounded-lg bg-white p-2" />
            <p className="text-sm">
              Aponte a <b>câmera do celular</b> para o QR Code e abra o link: a câmera funciona no navegador, sem instalar nada
              (Chrome no Android, Safari no iPhone).
            </p>
            <p className="text-xs text-muted">
              Na primeira vez, o navegador avisa que a conexão não é particular (certificado deste computador): toque em
              <b> Avançado → Continuar</b>. O app SacraLink Camera também lê este QR Code.
            </p>
            <p className="text-xs text-muted">Código de uso único · expira em {Math.floor(left / 60)}:{String(left % 60).padStart(2, '0')}</p>
          </>
        ) : (
          <>
            <p className="text-sm text-muted">O código expirou.</p>
            <button className="btn-primary" onClick={generate}>Gerar novo código</button>
          </>
        )}
        <p className="text-xs text-muted">O QR Code não contém a senha do administrador. Ele autoriza apenas um aparelho.</p>
      </div>
    </Modal>
  );
}

function EditCamera({ cam, profiles, onClose }: { cam: CameraView; profiles: MediaProfile[]; onClose: () => void }) {
  const { api, run } = useStore();
  const [name, setName] = useState(cam.name);
  const [operatorName, setOperator] = useState(cam.operatorName);
  const [profileId, setProfile] = useState(cam.profileId);
  const [ingestMode, setIngest] = useState(cam.ingestMode);
  const [isBackup, setBackup] = useState(cam.isBackup);
  const [audioSyncOffsetMs, setOffset] = useState(cam.audioSyncOffsetMs);
  const [orientation, setOrientation] = useState<Orientation>(cam.orientation ?? 'landscape');
  const save = async () => {
    const r = await run(() => api.patch(`/api/v1/cameras/${cam.id}`, { name, operatorName, profileId, ingestMode, isBackup, audioSyncOffsetMs, orientation }), 'Câmera atualizada');
    if (r !== undefined) {
      if (ingestMode !== cam.ingestMode && cam.obsSource.state === 'present') {
        await run(() => api.post(`/api/v1/obs/cameras/${cam.id}/source`, { recreate: true }), 'Fonte do OBS recriada no novo modo');
      }
      onClose();
    }
  };
  return (
    <Modal open title={`Editar ${cam.name}`} onClose={onClose}>
      <div className="space-y-3">
        <div><label className="label">Nome da câmera</label><input className="input" value={name} onChange={(e) => setName(e.target.value)} maxLength={60} /></div>
        <div><label className="label">Operador</label><input className="input" value={operatorName} onChange={(e) => setOperator(e.target.value)} placeholder="Opcional" /></div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="label">Perfil de qualidade</label>
            <select className="input" value={profileId} onChange={(e) => setProfile(e.target.value)}>
              {profiles.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
          </div>
          <div>
            <label className="label">Ingestão no OBS</label>
            <select className="input" value={ingestMode} onChange={(e) => setIngest(e.target.value as 'rtsp' | 'whep')}>
              <option value="rtsp">RTSP — Media Source (padrão)</option>
              <option value="whep">WebRTC — Browser Source (experimental)</option>
            </select>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="label">Orientação da imagem</label>
            <select className="input" value={orientation} onChange={(e) => setOrientation(e.target.value as Orientation)}>
              <option value="landscape">Horizontal (16:9)</option>
              <option value="portrait">Vertical (9:16)</option>
            </select>
          </div>
          <div>
            <label className="label">Atraso do áudio (ms)</label>
            <input className="input" type="number" min={-950} max={20000} value={audioSyncOffsetMs} onChange={(e) => setOffset(+e.target.value)} />
          </div>
        </div>
        <p className="text-xs text-muted">
          O celular segue a orientação escolhida aqui: a tela dele pede para girar e trava na posição certa (quando o navegador permite).
        </p>
        <Toggle checked={isBackup} onChange={setBackup} label="Câmera reserva" hint="Usada pelo fallback automático quando a câmera no ar perde o sinal (se habilitado)." />
        {ingestMode !== cam.ingestMode && cam.obsSource.state === 'present' && (
          <p className="text-xs text-warn">Trocar o modo recria a fonte desta câmera no OBS (somente a fonte criada pelo SacraLink).</p>
        )}
        <div className="flex justify-end gap-2">
          <button className="btn-ghost" onClick={onClose}>Cancelar</button>
          <button className="btn-primary" onClick={save}>Salvar</button>
        </div>
      </div>
    </Modal>
  );
}

export function Devices() {
  const { api, state, run } = useStore();
  const [devices, setDevices] = useState<Device[]>([]);
  const [profiles, setProfiles] = useState<MediaProfile[]>([]);
  const [pairing, setPairing] = useState<{ cameraId?: string } | null>(null);
  const [editing, setEditing] = useState<CameraView | null>(null);
  const [confirm, setConfirm] = useState<{ title: string; message: string; action: () => Promise<unknown> } | null>(null);
  const cams = sortedCameras(state);

  const load = useCallback(() => {
    api.get<Device[]>('/api/v1/devices').then(setDevices).catch(() => {});
  }, [api]);
  useEffect(() => {
    load();
    api.get<MediaProfile[]>('/api/v1/profiles').then(setProfiles).catch(() => {});
  }, [api, load, state.cameras]);

  const act = (d: Device, action: string, title: string, message: string) =>
    setConfirm({ title, message, action: () => api.post(`/api/v1/devices/${d.id}/${action}`) });

  return (
    <div className="space-y-4 p-4">
      <Section title="Câmeras" actions={<button className="btn-primary" onClick={() => setPairing({})}><QrCode size={16} /> Parear celular</button>}>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-muted uppercase">
              <tr><th className="py-2">Nº</th><th>Câmera</th><th>Aparelho</th><th>Estado</th><th>Fonte no OBS</th><th /></tr>
            </thead>
            <tbody>
              {cams.map((c) => (
                <tr key={c.id} className="border-t border-line">
                  <td className="py-2 font-bold">{String(c.slot).padStart(2, '0')}</td>
                  <td>{c.name}{c.isBackup && <span className="ml-1 text-xs text-muted">(reserva)</span>}<div className="text-xs text-muted">{c.operatorName}</div></td>
                  <td>{c.device ? `${c.device.name} · ${c.device.platform}` : <span className="text-muted">nenhum</span>}</td>
                  <td>{t(`state.${c.state}`)}</td>
                  <td className="text-xs">{c.obsSource.inputName ?? '—'}</td>
                  <td className="text-right whitespace-nowrap">
                    <button className="btn-ghost px-2" title="Editar" onClick={() => setEditing(c)}><Pencil size={14} /></button>{' '}
                    <button className="btn-ghost px-2" title="Parear outro aparelho nesta câmera" onClick={() => setPairing({ cameraId: c.id })}><QrCode size={14} /></button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Section>

      <Section title="Aparelhos pareados">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-muted uppercase">
              <tr><th className="py-2">Aparelho</th><th>Modelo</th><th>App</th><th>Situação</th><th>Último contato</th><th /></tr>
            </thead>
            <tbody>
              {devices.map((d) => (
                <tr key={d.id} className="border-t border-line">
                  <td className="py-2">{d.name}<div className="text-xs text-muted">{d.platform} · {d.lastIp}</div></td>
                  <td>{d.model || '—'}</td>
                  <td>{d.appVersion || '—'}</td>
                  <td className={d.status === 'active' ? 'text-preview' : 'text-onair'}>{d.status === 'active' ? 'autorizado' : d.status === 'revoked' ? 'revogado' : 'bloqueado'}</td>
                  <td>{d.lastSeenAt ? formatTime(d.lastSeenAt) : '—'}</td>
                  <td className="text-right whitespace-nowrap">
                    <button className="btn-ghost px-2" title="Desconectar agora" onClick={() => act(d, 'disconnect', 'Desconectar aparelho?', `${d.name} será desconectado; ele pode reconectar em seguida.`)}><Unplug size={14} /></button>{' '}
                    {d.status === 'active' ? (
                      <>
                        <button className="btn-ghost px-2" title="Revogar autorização" onClick={() => act(d, 'revoke', 'Revogar autorização?', `${d.name} precisará ser pareado novamente.`)}><UserX size={14} /></button>{' '}
                        <button className="btn-ghost px-2" title="Bloquear" onClick={() => act(d, 'block', 'Bloquear aparelho?', `${d.name} não poderá mais conectar até ser desbloqueado.`)}><Ban size={14} /></button>
                      </>
                    ) : (
                      <button className="btn-ghost px-2" title="Reautorizar" onClick={() => act(d, 'unblock', 'Reautorizar aparelho?', `${d.name} poderá conectar novamente.`)}><Undo2 size={14} /></button>
                    )}{' '}
                    <button className="btn-ghost px-2 text-onair" title="Remover" onClick={() => setConfirm({ title: 'Remover aparelho?', message: `${d.name} será removido. A câmera e sua fonte no OBS permanecem.`, action: () => api.del(`/api/v1/devices/${d.id}?confirm=true`) })}><Trash2 size={14} /></button>
                  </td>
                </tr>
              ))}
              {devices.length === 0 && <tr><td colSpan={6} className="py-6 text-center text-muted">Nenhum aparelho pareado.</td></tr>}
            </tbody>
          </table>
        </div>
      </Section>

      <PairingModal open={!!pairing} cameraId={pairing?.cameraId} onClose={() => { setPairing(null); load(); }} />
      {editing && <EditCamera cam={editing} profiles={profiles} onClose={() => setEditing(null)} />}
      <Confirm
        open={!!confirm} title={confirm?.title ?? ''} message={confirm?.message} danger
        onCancel={() => setConfirm(null)}
        onConfirm={async () => { const c = confirm; setConfirm(null); if (c) await run(c.action, 'Feito'); load(); }}
      />
    </div>
  );
}
