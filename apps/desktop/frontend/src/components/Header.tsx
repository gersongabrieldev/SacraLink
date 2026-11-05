import { Cpu, MemoryStick, Network, Radio, Server, Smartphone, Video, Wifi, Globe } from 'lucide-react';
import { useStore } from '../lib/store';
import { t } from '../lib/i18n';
import { formatBitrate } from '../lib/format';
import { Dot } from './ui';

export function Header({ lanIp }: { lanIp?: string }) {
  const { state } = useStore();
  const sys = state.system;
  const obs = state.obs;
  const media = state.media;
  const live = !!obs && obs.state === 'connected' && (obs.streaming || obs.recording);
  const obsColor = obs?.state === 'connected' ? 'bg-preview' : obs?.state === 'connecting' ? 'bg-warn' : 'bg-onair';
  const mediaColor = media?.state === 'running' ? 'bg-preview' : media?.state === 'failed' ? 'bg-onair' : 'bg-warn';
  const remote = state.settings?.remote.enabled;

  return (
    <header className="flex flex-wrap items-center gap-2 border-b border-line bg-surface px-4 py-2.5">
      <div className="mr-2 flex items-center gap-2">
        <img src="/logo.svg" alt="" className="h-7 w-7" />
        <div className="leading-tight">
          <div className="text-sm font-bold">SacraLink</div>
          <div className="max-w-40 truncate text-[11px] text-muted">{state.settings?.serverName}</div>
        </div>
      </div>

      <span className="pill" title={media?.lastError || t(`media.${media?.state ?? 'stopped'}`)}>
        <Server size={13} /> <Dot color={state.live ? mediaColor : 'bg-onair'} /> {state.live ? t(`media.${media?.state ?? 'starting'}`) : 'Servidor sem conexão'}
      </span>
      <span className="pill" title={obs?.error || obs?.address}>
        <Video size={13} /> <Dot color={obsColor} /> {t(`obs.${obs?.state ?? 'disabled'}`)}
        {obs?.studioMode && <span className="text-muted">· Studio</span>}
      </span>
      <span className="pill">
        <Smartphone size={13} /> {sys?.camerasConnected ?? 0} conectadas · {sys?.camerasStreaming ?? 0} transmitindo
      </span>
      <span className="pill" title="Endereço para os celulares na rede local">
        <Wifi size={13} /> {lanIp ?? '—'}
      </span>
      <span className="pill" title="Acesso externo (4G/5G)">
        <Globe size={13} /> {remote ? 'Externo habilitado' : 'Somente rede local'}
      </span>

      <div className="ml-auto flex flex-wrap items-center gap-2">
        <span className="pill" title="Processador"><Cpu size={13} /> {Math.round(sys?.stats.cpuPercent ?? 0)}%</span>
        <span className="pill" title="Memória"><MemoryStick size={13} /> {Math.round(sys?.stats.memPercent ?? 0)}%</span>
        <span className="pill" title="Entrada de vídeo das câmeras"><Network size={13} /> {formatBitrate(sys?.inboundKbps ?? 0)}</span>
        <span
          className={`pill font-bold ${live ? 'border-onair bg-onair text-white' : 'text-muted'}`}
          title={obs?.state !== 'connected' ? 'Estado do OBS desconhecido' : live ? 'OBS transmitindo/gravando' : 'OBS não está transmitindo'}
        >
          <Radio size={13} className={live ? 'pulse-dot' : ''} />
          {obs?.state !== 'connected' ? 'AO VIVO ?' : live ? (obs.streaming ? 'AO VIVO' : 'GRAVANDO') : 'FORA DO AR'}
        </span>
      </div>
    </header>
  );
}
