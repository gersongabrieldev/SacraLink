import { useState } from 'react';
import { LogIn } from 'lucide-react';
import { useStore } from '../lib/store';
import { errorMessage } from '../lib/api';

export function Login() {
  const { api, setToken } = useStore();
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const r = await api.post<{ token: string }>('/api/v1/auth/login', { password });
      setToken(r.token);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex h-full items-center justify-center p-4">
      <form onSubmit={submit} className="card w-full max-w-sm p-6">
        <div className="mb-6 flex items-center gap-3">
          <img src="/logo.svg" alt="" className="h-10 w-10" />
          <div>
            <h1 className="text-xl font-bold">SacraLink</h1>
            <p className="text-sm text-muted">Acesso do administrador</p>
          </div>
        </div>
        <label className="label" htmlFor="pw">Senha</label>
        <input id="pw" type="password" className="input mb-3" value={password} onChange={(e) => setPassword(e.target.value)} autoFocus autoComplete="current-password" />
        {error && <p className="mb-3 text-sm text-onair">{error}</p>}
        <button className="btn-primary w-full" disabled={busy || !password}>
          <LogIn size={16} /> Entrar
        </button>
        <p className="mt-4 text-xs text-muted">
          Esqueceu a senha? No computador do SacraLink, execute <code>sacralink-server admin reset --confirm</code> e defina uma nova.
        </p>
      </form>
    </div>
  );
}
