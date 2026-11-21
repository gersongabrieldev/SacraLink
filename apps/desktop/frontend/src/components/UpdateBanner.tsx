import { useEffect, useState } from 'react';
import { Download, X } from 'lucide-react';
import { useStore } from '../lib/store';
import { openReleaseURL } from '../lib/api';
import type { UpdateStatus } from '../lib/types';

/** Faixa "Nova versão disponível": verifica ao abrir o painel e a cada 30 min.
 *  O servidor consulta o GitHub (no máximo uma vez por minuto) e nada é
 *  baixado ou instalado sem o operador clicar. */
export function UpdateBanner() {
  const { api } = useStore();
  const [st, setSt] = useState<UpdateStatus | null>(null);
  const [hidden, setHidden] = useState(false);

  useEffect(() => {
    let alive = true;
    const apply = (s: UpdateStatus) => { if (alive) setSt(s); };
    api.post<UpdateStatus>('/api/v1/update/check').then(apply).catch(() => {});
    const id = window.setInterval(() => api.get<UpdateStatus>('/api/v1/update').then(apply).catch(() => {}), 30 * 60 * 1000);
    return () => { alive = false; window.clearInterval(id); };
  }, [api]);

  const latest = st?.latest;
  if (hidden || !st?.available || !latest) return null;
  return (
    <div role="status" className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-info/40 bg-info/10 px-4 py-2 text-sm">
      <Download size={16} className="shrink-0 text-info" />
      <span>
        <b>Nova versão do SacraLink: {latest.version}</b>
        <span className="text-muted"> · você está usando a {st.current}{latest.prerelease ? ' · pré-lançamento' : ''}</span>
      </span>
      <div className="flex flex-wrap items-center gap-2">
        {latest.installerUrl && (
          <button className="btn-primary px-3 py-1 text-xs" onClick={() => openReleaseURL(latest.installerUrl!)}>Baixar instalador</button>
        )}
        <button className="btn-ghost px-3 py-1 text-xs" onClick={() => openReleaseURL(latest.url)}>O que mudou</button>
      </div>
      <button className="ml-auto flex items-center gap-1 text-xs text-muted hover:text-fg" onClick={() => setHidden(true)} title="Esconder até a próxima vez que o painel for aberto">
        <X size={14} /> Agora não
      </button>
    </div>
  );
}
