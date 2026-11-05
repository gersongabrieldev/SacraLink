import { useEffect, useState } from 'react';
import { useStore } from '../lib/store';
import { formatTime } from '../lib/format';
import { Section } from '../components/ui';

interface Audit { id: number; ts: number; actor: string; action: string; target: string; ip: string; success: boolean; details: string }

export function Events() {
  const { api, state } = useStore();
  const [audit, setAudit] = useState<Audit[]>([]);
  useEffect(() => {
    api.get<Audit[]>('/api/v1/audit?limit=200').then(setAudit).catch(() => {});
  }, [api]);
  const color = { info: 'text-muted', warn: 'text-warn', error: 'text-onair' };
  const cam = (id?: string) => (id ? state.cameras[id]?.name ?? id : '');

  return (
    <div className="grid gap-4 p-4 xl:grid-cols-2">
      <Section title="Eventos operacionais">
        <ul className="max-h-[70vh] space-y-1 overflow-auto text-sm">
          {state.events.map((e) => (
            <li key={e.id} className="flex gap-3">
              <span className="w-16 shrink-0 text-xs text-muted tabular-nums">{formatTime(e.ts)}</span>
              <span className={color[e.level]}>{e.message}</span>
              {e.cameraId && <span className="text-xs text-muted">{cam(e.cameraId)}</span>}
            </li>
          ))}
          {state.events.length === 0 && <li className="text-muted">Sem eventos.</li>}
        </ul>
      </Section>
      <Section title="Auditoria de segurança">
        <ul className="max-h-[70vh] space-y-1 overflow-auto text-xs">
          {audit.map((a) => (
            <li key={a.id} className="flex gap-3">
              <span className="w-16 shrink-0 text-muted tabular-nums">{formatTime(a.ts)}</span>
              <span className={a.success ? '' : 'text-onair'}>{a.action}</span>
              <span className="text-muted">{a.actor} {a.ip && `· ${a.ip}`} {a.details && `· ${a.details}`}</span>
            </li>
          ))}
        </ul>
      </Section>
    </div>
  );
}
