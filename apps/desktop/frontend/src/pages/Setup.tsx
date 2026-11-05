import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, ChevronRight, MonitorCheck, Shield, Video, Wifi, XCircle } from 'lucide-react';
import { useStore } from '../lib/store';
import { errorMessage } from '../lib/api';
import type { NetworkReport, OBSStatus, Settings, SystemStatus } from '../lib/types';
import { Toggle } from '../components/ui';

type Step = 'welcome' | 'password' | 'obs' | 'firewall' | 'network' | 'done';
const steps: { id: Step; label: string }[] = [
  { id: 'welcome', label: 'Sistema' },
  { id: 'password', label: 'Senha' },
  { id: 'obs', label: 'OBS' },
  { id: 'firewall', label: 'Firewall' },
  { id: 'network', label: 'Rede' },
  { id: 'done', label: 'Pronto' },
];

interface OBSDetect {
  found: boolean;
  path?: string;
  version?: string;
  running: boolean;
  supported: boolean;
  websocket?: { enabled: boolean; port: number; authRequired: boolean };
}

/** Assistente da primeira execução (especificação §4.1). Nenhuma alteração no
 * firewall ou no OBS é feita sem a confirmação explícita do operador. */
export function Setup({ adminConfigured, onDone }: { adminConfigured: boolean; onDone: () => void }) {
  const { api, authed, setToken, run } = useStore();
  const [step, setStep] = useState<Step>(adminConfigured ? 'obs' : 'welcome');
  const [sys, setSys] = useState<SystemStatus | null>(null);

  useEffect(() => {
    if (authed) api.get<SystemStatus>('/api/v1/system/status').then(setSys).catch(() => {});
  }, [api, authed, step]);

  const idx = steps.findIndex((s) => s.id === step);
  const next = () => setStep(steps[Math.min(idx + 1, steps.length - 1)].id);

  return (
    <div className="flex h-full items-center justify-center overflow-auto p-4">
      <div className="card w-full max-w-2xl p-6">
        <div className="mb-6 flex items-center gap-3">
          <img src="/logo.svg" alt="" className="h-10 w-10" />
          <div>
            <h1 className="text-xl font-bold">Configuração inicial do SacraLink</h1>
            <p className="text-sm text-muted">Leva poucos minutos. Você pode alterar tudo depois em Configurações.</p>
          </div>
        </div>
        <ol className="mb-6 flex flex-wrap gap-1 text-xs">
          {steps.map((s, i) => (
            <li key={s.id} className={`rounded-full px-2.5 py-1 ${i === idx ? 'bg-accent font-semibold text-black' : i < idx ? 'bg-surface-2 text-fg' : 'bg-surface-2 text-muted'}`}>
              {i + 1}. {s.label}
            </li>
          ))}
        </ol>

        {step === 'welcome' && <Welcome onNext={next} />}
        {step === 'password' && <Password onDone={(tok) => { setToken(tok); next(); }} />}
        {step === 'obs' && <OBSStep onNext={next} />}
        {step === 'firewall' && <FirewallStep onNext={next} />}
        {step === 'network' && <NetworkStep sys={sys} onNext={next} />}
        {step === 'done' && (
          <div className="space-y-4">
            <p className="flex items-center gap-2 text-preview"><CheckCircle2 /> Tudo pronto!</p>
            <p className="text-sm text-muted">
              Para conectar um celular, abra <b>Câmeras e aparelhos → Parear celular</b> e leia o QR Code com o app SacraLink Camera.
            </p>
            <button className="btn-primary" onClick={() => run(async () => { await api.post('/api/v1/setup/complete'); onDone(); })}>
              Abrir o painel <ChevronRight size={16} />
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

function Welcome({ onNext }: { onNext: () => void }) {
  return (
    <div className="space-y-4 text-sm">
      <p>O SacraLink transforma celulares em câmeras sem fio para o OBS. Antes de começar, verifique:</p>
      <ul className="space-y-2">
        <li className="flex gap-2"><MonitorCheck size={18} className="shrink-0 text-accent" /> Este computador roda o OBS Studio 28 ou mais recente.</li>
        <li className="flex gap-2"><Wifi size={18} className="shrink-0 text-accent" /> Os celulares estarão no mesmo Wi-Fi (de preferência 5 GHz) — o computador de preferência no cabo.</li>
        <li className="flex gap-2"><Shield size={18} className="shrink-0 text-accent" /> Nada será alterado no OBS ou no firewall sem a sua confirmação.</li>
      </ul>
      <button className="btn-primary" onClick={onNext}>Começar <ChevronRight size={16} /></button>
    </div>
  );
}

function Password({ onDone }: { onDone: (token: string) => void }) {
  const { api } = useStore();
  const [name, setName] = useState('');
  const [pw, setPw] = useState('');
  const [pw2, setPw2] = useState('');
  const [error, setError] = useState('');
  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (pw !== pw2) return setError('As senhas não conferem.');
    try {
      const r = await api.post<{ token: string }>('/api/v1/setup/admin', { password: pw, serverName: name });
      onDone(r.token);
    } catch (err) {
      setError(errorMessage(err));
    }
  };
  return (
    <form onSubmit={submit} className="space-y-3">
      <div>
        <label className="label" htmlFor="sn">Nome da igreja ou do servidor</label>
        <input id="sn" className="input" placeholder="Ex.: Paróquia São José" value={name} onChange={(e) => setName(e.target.value)} maxLength={60} />
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <label className="label" htmlFor="p1">Senha do administrador</label>
          <input id="p1" type="password" className="input" value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" />
        </div>
        <div>
          <label className="label" htmlFor="p2">Confirmar senha</label>
          <input id="p2" type="password" className="input" value={pw2} onChange={(e) => setPw2(e.target.value)} autoComplete="new-password" />
        </div>
      </div>
      <p className="text-xs text-muted">Mínimo de 8 caracteres. A senha protege o painel; ela nunca vai para o QR Code dos celulares.</p>
      {error && <p className="text-sm text-onair">{error}</p>}
      <button className="btn-primary" disabled={pw.length < 8}>Salvar senha <ChevronRight size={16} /></button>
    </form>
  );
}

function OBSStep({ onNext }: { onNext: () => void }) {
  const { api, state, notify } = useStore();
  const [detect, setDetect] = useState<OBSDetect | null>(null);
  const [host, setHost] = useState('127.0.0.1');
  const [port, setPort] = useState(4455);
  const [password, setPassword] = useState('');
  const [autoManage, setAutoManage] = useState(true);
  const [busy, setBusy] = useState(false);
  const obs: OBSStatus | null = state.obs;

  useEffect(() => {
    api.get<OBSDetect>('/api/v1/obs/detect').then((d) => {
      setDetect(d);
      if (d.websocket?.port) setPort(d.websocket.port);
    }).catch(() => {});
  }, [api]);

  const importLocal = async () => {
    setBusy(true);
    try {
      await api.post('/api/v1/obs/config/import');
      notify('success', 'Configuração do obs-websocket importada deste computador.');
    } catch (e) {
      notify('error', errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const save = async () => {
    setBusy(true);
    try {
      await api.put('/api/v1/obs/config', { host, port, password, autoConnect: true });
    } catch (e) {
      notify('error', errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const finish = async () => {
    const s = await api.get<Settings>('/api/v1/settings');
    await api.put('/api/v1/settings', { ...s, obs: { ...s.obs, autoManageSources: autoManage } });
    onNext();
  };

  return (
    <div className="space-y-4 text-sm">
      <div className="card bg-surface-2 p-3">
        {detect === null ? 'Procurando o OBS Studio…' : detect.found ? (
          <div className="space-y-1">
            <div className="flex items-center gap-2">{detect.supported ? <CheckCircle2 size={16} className="text-preview" /> : <XCircle size={16} className="text-onair" />}
              OBS Studio {detect.version} encontrado {detect.running ? '(aberto)' : '(fechado)'}</div>
            {!detect.supported && <div className="text-onair">Versão não suportada: atualize para o OBS 28 ou mais recente.</div>}
            {detect.websocket && !detect.websocket.enabled && (
              <div className="text-warn">O servidor WebSocket do OBS está desligado. No OBS: Ferramentas → Configurações do WebSocket → Habilitar.</div>
            )}
          </div>
        ) : (
          <div className="flex items-center gap-2 text-warn"><AlertTriangle size={16} /> OBS Studio não encontrado neste computador.</div>
        )}
      </div>

      {detect?.websocket?.enabled && (
        <div className="flex flex-wrap items-center gap-2">
          <button className="btn-ghost" onClick={importLocal} disabled={busy}>Importar configuração do OBS deste computador</button>
          <span className="text-xs text-muted">Lê o endereço, a porta e a senha do obs-websocket (somente leitura).</span>
        </div>
      )}

      <div className="grid gap-3 sm:grid-cols-[1fr_100px_1fr]">
        <div><label className="label">Endereço</label><input className="input" value={host} onChange={(e) => setHost(e.target.value)} /></div>
        <div><label className="label">Porta</label><input className="input" type="number" value={port} onChange={(e) => setPort(+e.target.value)} /></div>
        <div><label className="label">Senha do WebSocket</label><input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} /></div>
      </div>
      <button className="btn-ghost" onClick={save} disabled={busy}>Conectar ao OBS</button>
      <div className={`text-sm ${obs?.state === 'connected' ? 'text-preview' : obs?.state === 'auth_failed' ? 'text-onair' : 'text-muted'}`}>
        Estado: {obs?.state === 'connected' ? `conectado ao OBS ${obs.obsVersion} (WebSocket ${obs.wsVersion})` : obs?.state === 'auth_failed' ? 'senha incorreta' : obs?.error || obs?.state || '—'}
      </div>

      <Toggle
        checked={autoManage}
        onChange={setAutoManage}
        label="Permitir que o SacraLink crie fontes e cenas no OBS"
        hint="Cria uma fonte e uma cena “SacraLink — Câmera NN” por celular. Fontes e cenas suas nunca são alteradas ou apagadas."
      />
      <div className="flex gap-2">
        <button className="btn-primary" onClick={finish}>Continuar <ChevronRight size={16} /></button>
        <button className="btn-ghost" onClick={onNext}>Configurar depois</button>
      </div>
    </div>
  );
}

interface FirewallStatus { rules: { name: string; protocol: string; port: number; exists: boolean }[]; elevated: boolean }

function FirewallStep({ onNext }: { onNext: () => void }) {
  const { api, notify } = useStore();
  const [fw, setFw] = useState<FirewallStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const load = () => api.get<FirewallStatus>('/api/v1/system/firewall').then(setFw).catch(() => {});
  useEffect(() => { load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  const apply = async () => {
    setBusy(true);
    try {
      setFw(await api.post<FirewallStatus>('/api/v1/system/firewall', { confirm: true }));
      notify('success', 'Regras de firewall criadas.');
    } catch (e) {
      notify('error', errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const allOk = fw?.rules.every((r) => r.exists);
  return (
    <div className="space-y-4 text-sm">
      <p>Para os celulares alcançarem este computador, o Firewall do Windows precisa permitir a entrada nas portas abaixo (somente redes <b>privadas</b> e de domínio):</p>
      <ul className="space-y-1">
        {fw?.rules.map((r) => (
          <li key={r.name} className="flex items-center gap-2">
            {r.exists ? <CheckCircle2 size={16} className="text-preview" /> : <AlertTriangle size={16} className="text-warn" />} {r.name}
          </li>
        ))}
      </ul>
      <p className="text-xs text-muted">O Windows pedirá permissão de administrador. Nenhuma outra porta é aberta; o painel e o OBS continuam acessíveis só neste computador.</p>
      <div className="flex gap-2">
        {!allOk && <button className="btn-primary" onClick={apply} disabled={busy}>Criar regras (requer administrador)</button>}
        <button className={allOk ? 'btn-primary' : 'btn-ghost'} onClick={onNext}>{allOk ? 'Continuar' : 'Pular'} <ChevronRight size={16} /></button>
      </div>
    </div>
  );
}

function NetworkStep({ sys, onNext }: { sys: SystemStatus | null; onNext: () => void }) {
  const { api } = useStore();
  const [rep, setRep] = useState<NetworkReport | null>(null);
  useEffect(() => { api.get<NetworkReport>('/api/v1/network/status').then(setRep).catch(() => {}); }, [api]);
  return (
    <div className="space-y-4 text-sm">
      {sys?.os.warning && (
        <div className="flex gap-2 rounded-lg border border-warn/50 bg-warn/10 p-3 text-warn"><AlertTriangle size={18} className="shrink-0" /> {sys.os.warning}</div>
      )}
      <p>Endereço deste computador na rede local (usado pelos celulares):</p>
      <div className="text-2xl font-bold tracking-wide text-accent">{rep?.lanHosts?.[0] ?? sys?.lanHosts?.[0] ?? '—'}</div>
      <ul className="space-y-1">
        {rep?.advice.map((a) => (
          <li key={a.title} className="text-xs"><b>{a.title}:</b> <span className="text-muted">{a.message}</span></li>
        ))}
      </ul>
      <button className="btn-primary" onClick={onNext}>Concluir <ChevronRight size={16} /></button>
      <div className="flex items-center gap-2 text-xs text-muted"><Video size={14} /> O acesso por 4G/5G fica desligado até você habilitá-lo em Rede.</div>
    </div>
  );
}
