import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, Info, RefreshCw, XCircle } from 'lucide-react';
import { useStore } from '../lib/store';
import type { NetworkReport, Settings } from '../lib/types';
import { Section, Toggle } from '../components/ui';

export function Network() {
  const { api, state, run } = useStore();
  const [rep, setRep] = useState<NetworkReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [remote, setRemote] = useState<Settings['remote'] | null>(state.settings?.remote ?? null);

  const diagnose = async () => {
    setBusy(true);
    const r = await run(() => api.post<NetworkReport>('/api/v1/network/diagnose'));
    if (r) setRep(r);
    setBusy(false);
  };
  useEffect(() => {
    api.get<NetworkReport>('/api/v1/network/status').then(setRep).catch(() => {});
  }, [api]);
  useEffect(() => setRemote(state.settings?.remote ?? null), [state.settings]);

  const icons = { ok: CheckCircle2, info: Info, warn: AlertTriangle, error: XCircle };
  const colors = { ok: 'text-preview', info: 'text-info', warn: 'text-warn', error: 'text-onair' };

  return (
    <div className="grid gap-4 p-4 xl:grid-cols-2">
      <Section title="Diagnóstico de rede" actions={<button className="btn-ghost" onClick={diagnose} disabled={busy}><RefreshCw size={14} className={busy ? 'animate-spin' : ''} /> Diagnosticar</button>}>
        {rep && (
          <div className="space-y-4">
            <dl className="grid grid-cols-2 gap-2 text-sm">
              <div><dt className="label">IP local</dt><dd className="text-lg font-bold text-accent">{rep.primaryIpv4 || '—'}</dd></div>
              <div><dt className="label">IP público (STUN)</dt><dd>{rep.publicIpv4 || 'não verificado'}</dd></div>
              <div><dt className="label">IPv6 global</dt><dd>{rep.globalIpv6?.length ? rep.globalIpv6[0] : 'não'}</dd></div>
              <div><dt className="label">CGNAT</dt><dd>{{ no: 'não', yes: 'sim', likely: 'provável', unknown: 'não confirmado' }[rep.cgnat]}</dd></div>
            </dl>
            <ul className="space-y-2">
              {rep.advice.map((a) => {
                const Icon = icons[a.level];
                return (
                  <li key={a.title} className="flex gap-2 text-sm">
                    <Icon size={18} className={`mt-0.5 shrink-0 ${colors[a.level]}`} />
                    <span><b>{a.title}.</b> <span className="text-muted">{a.message}</span></span>
                  </li>
                );
              })}
            </ul>
          </div>
        )}
      </Section>

      {remote && state.settings && (
        <Section title="Acesso externo (4G/5G)">
          <div className="space-y-3 text-sm">
            <p className="text-muted">
              Sem VPS ou relay: o celular precisa alcançar diretamente este computador. Isso exige IP público com encaminhamento de portas
              no roteador, ou IPv6 global. Atrás de CGNAT sem IPv6, a conexão externa é inviável.
            </p>
            {state.settings && <Toggle checked={remote.enabled} onChange={(v) => setRemote({ ...remote, enabled: v })} label="Habilitar acesso externo" hint="Desligado por padrão. Exponha apenas as portas do SacraLink, nunca o OBS." />}
            <div className="grid gap-3 sm:grid-cols-[1fr_120px]">
              <div><label className="label">IP público ou DNS dinâmico</label><input className="input" placeholder="ex.: igreja.duckdns.org" value={remote.publicHost} onChange={(e) => setRemote({ ...remote, publicHost: e.target.value })} /></div>
              <div><label className="label">Porta pública</label><input className="input" type="number" value={remote.publicPort} onChange={(e) => setRemote({ ...remote, publicPort: +e.target.value })} /></div>
            </div>
            <Toggle checked={remote.useStun} onChange={(v) => setRemote({ ...remote, useStun: v })} label="Descobrir IP público via STUN" hint="Somente descoberta de endereço; nenhum vídeo passa por servidores externos." />
            {state.system && <p className="text-xs text-warn">Alterar o acesso externo reinicia o servidor de mídia: as câmeras reconectam em alguns segundos.</p>}
            <button className="btn-primary" onClick={() => run(() => api.put('/api/v1/settings', { ...state.settings!, remote }), 'Acesso externo atualizado')}>Salvar</button>
            <p className="text-xs text-muted">Depois de salvar, gere um novo QR Code: ele passa a incluir o endereço externo. Teste no app com o Wi-Fi desligado.</p>
          </div>
        </Section>
      )}
    </div>
  );
}
