import { useEffect, useState } from 'react';
import { AlertTriangle } from 'lucide-react';
import { useStore } from '../lib/store';
import type { Settings, SystemStatus } from '../lib/types';
import { Section, Toggle } from '../components/ui';

export function SettingsPage() {
  const { api, state, run, setToken } = useStore();
  const [s, setS] = useState<Settings | null>(state.settings);
  const [sys, setSys] = useState<SystemStatus | null>(null);
  const [pw, setPw] = useState({ current: '', next: '' });

  useEffect(() => setS(state.settings), [state.settings]);
  useEffect(() => {
    api.get<SystemStatus>('/api/v1/system/status').then(setSys).catch(() => {});
  }, [api]);

  if (!s) return null;
  const save = () => run(() => api.put('/api/v1/settings', s), 'Configurações salvas');

  return (
    <div className="grid gap-4 p-4 xl:grid-cols-2">
      <Section title="Geral">
        <div className="space-y-3">
          <div><label className="label">Nome da igreja/servidor (aparece no celular)</label><input className="input" value={s.serverName} maxLength={60} onChange={(e) => setS({ ...s, serverName: e.target.value })} /></div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="label">Prévia no painel</label>
              <select className="input" value={s.previewMode} onChange={(e) => setS({ ...s, previewMode: e.target.value as Settings['previewMode'] })}>
                <option value="all">Todas as câmeras</option>
                <option value="selected">Somente a selecionada (economiza CPU/GPU)</option>
                <option value="off">Desligada</option>
              </select>
            </div>
            <div>
              <label className="label">Transmissões simultâneas</label>
              <input className="input" type="number" min={1} max={8} value={s.maxStreams} onChange={(e) => setS({ ...s, maxStreams: +e.target.value })} />
            </div>
          </div>
          <div>
            <label className="label">Tema</label>
            <select className="input" value={s.theme} onChange={(e) => setS({ ...s, theme: e.target.value as Settings['theme'] })}>
              <option value="dark">Escuro</option>
              <option value="light">Claro</option>
            </select>
          </div>
          <Toggle checked={s.iceTcp} onChange={(v) => setS({ ...s, iceTcp: v })} label="Permitir mídia WebRTC sobre TCP" hint="Ajuda em redes que bloqueiam UDP; reinicia o servidor de mídia." />
          <button className="btn-primary" onClick={save}>Salvar</button>
        </div>
      </Section>

      <Section title="Sistema">
        {sys && (
          <div className="space-y-3 text-sm">
            {sys.os.warning && <div className="flex gap-2 rounded-lg border border-warn/50 bg-warn/10 p-3 text-warn"><AlertTriangle size={18} className="shrink-0" /> {sys.os.warning}</div>}
            {sys.os.smartAppControl === 'on' && (
              <div className="rounded-lg border border-info/40 bg-info/10 p-3 text-xs">O Smart App Control está ativo neste Windows: use somente instaladores assinados do SacraLink.</div>
            )}
            <dl className="grid grid-cols-2 gap-2 text-xs">
              <div><dt className="text-muted">Versão</dt><dd>{sys.version.version} ({sys.version.commit})</dd></div>
              <div><dt className="text-muted">Windows</dt><dd>{sys.os.name} · build {sys.os.build}</dd></div>
              <div><dt className="text-muted">Servidor de mídia</dt><dd>{sys.media.state} {sys.media.version} · reinícios: {sys.media.restarts}</dd></div>
              <div><dt className="text-muted">Pasta de dados</dt><dd className="break-all">{sys.dataDir}</dd></div>
              <div className="col-span-2"><dt className="text-muted">Impressão digital do certificado</dt><dd className="font-mono break-all">{sys.fingerprint}</dd></div>
            </dl>
            <Toggle checked={sys.autostart} onChange={(v) => run(async () => { await api.post('/api/v1/system/autostart', { enabled: v }); setSys({ ...sys, autostart: v }); }, v ? 'Início automático ativado' : 'Início automático desativado')}
              label="Iniciar o SacraLink com o Windows" hint="O servidor fica na bandeja e continua recebendo câmeras com o painel fechado." />
            <div className="flex flex-wrap gap-2">
              <button className="btn-ghost" onClick={() => run(() => api.post('/api/v1/system/media/restart'), 'Servidor de mídia reiniciando')}>Reiniciar servidor de mídia</button>
              <button className="btn-ghost" onClick={() => run(() => api.post('/api/v1/system/firewall', { confirm: true }), 'Regras de firewall atualizadas')}>Reconfigurar firewall</button>
            </div>
          </div>
        )}
      </Section>

      <Section title="Senha do administrador">
        <div className="grid gap-3 sm:grid-cols-2">
          <div><label className="label">Senha atual</label><input className="input" type="password" value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} /></div>
          <div><label className="label">Nova senha</label><input className="input" type="password" value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} /></div>
        </div>
        <button className="btn-primary mt-3" disabled={pw.next.length < 8} onClick={async () => {
          const r = await run(() => api.post<{ token: string }>('/api/v1/auth/password', { current: pw.current, new: pw.next }), 'Senha alterada; outras sessões foram encerradas');
          if (r) { setToken(r.token); setPw({ current: '', next: '' }); }
        }}>Alterar senha</button>
      </Section>
    </div>
  );
}
