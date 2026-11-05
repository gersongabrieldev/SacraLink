import { useEffect, type ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, Info, X, XCircle } from 'lucide-react';
import { useStore } from '../lib/store';

export function Modal({ open, title, onClose, children, wide }: { open: boolean; title: string; onClose: () => void; children: ReactNode; wide?: boolean }) {
  useEffect(() => {
    if (!open) return;
    const h = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('keydown', h);
    return () => window.removeEventListener('keydown', h);
  }, [open, onClose]);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onMouseDown={onClose} role="dialog" aria-modal="true" aria-label={title}>
      <div className={`card w-full ${wide ? 'max-w-2xl' : 'max-w-md'} p-5 shadow-2xl`} onMouseDown={(e) => e.stopPropagation()}>
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold">{title}</h2>
          <button className="text-muted hover:text-fg" onClick={onClose} aria-label="Fechar">
            <X size={18} />
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

/** Diálogo de confirmação para ações que afetam a transmissão. */
export function Confirm({
  open, title, message, confirmLabel = 'Confirmar', danger, onConfirm, onCancel,
}: { open: boolean; title: string; message: ReactNode; confirmLabel?: string; danger?: boolean; onConfirm: () => void; onCancel: () => void }) {
  return (
    <Modal open={open} title={title} onClose={onCancel}>
      <div className="mb-5 text-sm text-muted">{message}</div>
      <div className="flex justify-end gap-2">
        <button className="btn-ghost" onClick={onCancel}>Cancelar</button>
        <button className={danger ? 'btn-danger' : 'btn-primary'} onClick={onConfirm} autoFocus>{confirmLabel}</button>
      </div>
    </Modal>
  );
}

export function Toasts() {
  const { state, dispatch } = useStore();
  useEffect(() => {
    if (state.toasts.length === 0) return;
    const t = state.toasts[0];
    const id = window.setTimeout(() => dispatch({ type: 'dismiss', id: t.id }), t.level === 'error' ? 9000 : 4000);
    return () => window.clearTimeout(id);
  }, [state.toasts, dispatch]);
  const icons = { info: Info, success: CheckCircle2, warn: AlertTriangle, error: XCircle };
  const colors = { info: 'text-info', success: 'text-preview', warn: 'text-warn', error: 'text-onair' };
  return (
    <div className="fixed right-4 bottom-4 z-50 flex w-96 max-w-[calc(100vw-2rem)] flex-col gap-2" aria-live="polite">
      {state.toasts.map((t) => {
        const Icon = icons[t.level];
        return (
          <div key={t.id} className="card flex items-start gap-2 p-3 text-sm shadow-xl">
            <Icon size={18} className={`mt-0.5 shrink-0 ${colors[t.level]}`} />
            <span className="flex-1">{t.message}</span>
            <button className="text-muted hover:text-fg" onClick={() => dispatch({ type: 'dismiss', id: t.id })} aria-label="Fechar">
              <X size={14} />
            </button>
          </div>
        );
      })}
    </div>
  );
}

export function Dot({ color, pulse }: { color: string; pulse?: boolean }) {
  return <span className={`inline-block h-2 w-2 shrink-0 rounded-full ${color} ${pulse ? 'pulse-dot' : ''}`} />;
}

export function Section({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <section className="card p-5">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-base font-semibold">{title}</h2>
        {actions}
      </div>
      {children}
    </section>
  );
}

export function Toggle({ checked, onChange, label, hint }: { checked: boolean; onChange: (v: boolean) => void; label: string; hint?: string }) {
  return (
    <label className="flex cursor-pointer items-start gap-3 py-1.5">
      <input type="checkbox" className="mt-0.5 h-4 w-4 accent-[var(--accent)]" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span>
        <span className="text-sm font-medium">{label}</span>
        {hint && <span className="block text-xs text-muted">{hint}</span>}
      </span>
    </label>
  );
}

export function Empty({ icon, title, children }: { icon: ReactNode; title: string; children?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-12 text-center text-muted">
      <div className="text-accent">{icon}</div>
      <div className="text-base font-semibold text-fg">{title}</div>
      <div className="max-w-md text-sm">{children}</div>
    </div>
  );
}
