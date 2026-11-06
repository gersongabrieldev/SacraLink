import { useEffect, useRef, useState } from 'react';
import { AlertTriangle, CheckCircle2, PlayCircle, XCircle } from 'lucide-react';
import { useStore } from '../lib/store';

interface Check {
  name: string;
  status: 'ok' | 'warn' | 'error';
  message: string;
}

/** Teste de funcionamento (assistente §4.1, etapa 13): somente leitura. */
export function SelfTest({ autoRun = false }: { autoRun?: boolean }) {
  const { api, run } = useStore();
  const [checks, setChecks] = useState<Check[] | null>(null);
  const [busy, setBusy] = useState(false);
  const autoStarted = useRef(false);

  const execute = async () => {
    setBusy(true);
    const r = await run(() => api.post<Check[]>('/api/v1/system/selftest'));
    if (r) setChecks(r);
    setBusy(false);
  };
  useEffect(() => {
    // Executa uma única vez ao abrir (o ref evita repetir no StrictMode).
    if (autoRun && !autoStarted.current) {
      autoStarted.current = true;
      void execute();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoRun]);

  const icon = { ok: CheckCircle2, warn: AlertTriangle, error: XCircle };
  const color = { ok: 'text-preview', warn: 'text-warn', error: 'text-onair' };
  const errors = checks?.filter((c) => c.status === 'error').length ?? 0;

  return (
    <div className="space-y-2">
      <button className="btn-ghost" onClick={execute} disabled={busy}>
        <PlayCircle size={16} /> {busy ? 'Testando…' : 'Executar teste de funcionamento'}
      </button>
      {checks && (
        <>
          <ul className="space-y-1.5">
            {checks.map((c) => {
              const Icon = icon[c.status];
              return (
                <li key={c.name} className="flex gap-2 text-sm">
                  <Icon size={17} className={`mt-0.5 shrink-0 ${color[c.status]}`} />
                  <span><b>{c.name}:</b> <span className="text-muted">{c.message}</span></span>
                </li>
              );
            })}
          </ul>
          <p className={`text-xs ${errors ? 'text-onair' : 'text-preview'}`}>
            {errors ? `${errors} problema(s) impedem o funcionamento.` : 'Componentes essenciais funcionando.'}
          </p>
        </>
      )}
    </div>
  );
}
