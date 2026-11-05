import { useEffect, useState } from 'react';
import { Activity, Camera, LayoutGrid, LogOut, Network as NetworkIcon, Settings as SettingsIcon, Video } from 'lucide-react';
import { useStore } from './lib/store';
import { t } from './lib/i18n';
import type { SystemStatus } from './lib/types';
import { Header } from './components/Header';
import { Toasts } from './components/ui';
import { Login } from './pages/Login';
import { Setup } from './pages/Setup';
import { Dashboard } from './pages/Dashboard';
import { Devices } from './pages/Devices';
import { OBSPage } from './pages/OBSPage';
import { Network } from './pages/Network';
import { SettingsPage } from './pages/SettingsPage';
import { Events } from './pages/Events';

type Page = 'dashboard' | 'devices' | 'obs' | 'network' | 'settings' | 'events';
const nav: { id: Page; icon: typeof LayoutGrid }[] = [
  { id: 'dashboard', icon: LayoutGrid },
  { id: 'devices', icon: Camera },
  { id: 'obs', icon: Video },
  { id: 'network', icon: NetworkIcon },
  { id: 'settings', icon: SettingsIcon },
  { id: 'events', icon: Activity },
];

interface SetupStatus { adminConfigured: boolean; firstRunComplete: boolean; serverName: string }

export function App() {
  const { api, state, authed, setToken } = useStore();
  const [setup, setSetup] = useState<SetupStatus | null>(null);
  const [unreachable, setUnreachable] = useState(false);
  const [page, setPage] = useState<Page>('dashboard');
  const [lanIp, setLanIp] = useState<string>();

  useEffect(() => {
    let timer: number;
    const check = () =>
      api.get<SetupStatus>('/api/v1/setup/status').then((s) => { setSetup(s); setUnreachable(false); })
        .catch(() => { setUnreachable(true); timer = window.setTimeout(check, 2000); });
    check();
    return () => window.clearTimeout(timer);
  }, [api, authed]);

  useEffect(() => {
    if (authed) api.get<SystemStatus>('/api/v1/system/status').then((s) => setLanIp(s.lanHosts?.[0])).catch(() => {});
  }, [api, authed, state.live]);

  useEffect(() => {
    document.documentElement.classList.toggle('dark', (state.settings?.theme ?? 'dark') === 'dark');
  }, [state.settings?.theme]);

  if (unreachable && !setup) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center">
        <img src="/logo.svg" alt="" className="h-12 w-12" />
        <p className="font-semibold">Aguardando o servidor SacraLink…</p>
        <p className="max-w-md text-sm text-muted">O servidor roda em segundo plano (ícone na bandeja). Se ele não iniciar, reinstale o SacraLink ou consulte os logs em %LOCALAPPDATA%\SacraLink\logs.</p>
      </div>
    );
  }
  if (!setup) return null;
  if (!setup.adminConfigured || (authed && !setup.firstRunComplete)) {
    return (
      <>
        <Setup adminConfigured={setup.adminConfigured} onDone={() => setSetup({ ...setup, adminConfigured: true, firstRunComplete: true })} />
        <Toasts />
      </>
    );
  }
  if (!authed) return (<><Login /><Toasts /></>);

  return (
    <div className="flex h-full flex-col">
      <Header lanIp={lanIp} />
      <div className="flex min-h-0 flex-1">
        <nav className="flex w-14 shrink-0 flex-col items-center gap-1 border-r border-line bg-surface py-3 lg:w-52 lg:items-stretch lg:px-2" aria-label="Navegação">
          {nav.map(({ id, icon: Icon }) => (
            <button key={id} onClick={() => setPage(id)} title={t(`nav.${id}`)}
              className={`flex items-center gap-3 rounded-lg px-3 py-2 text-sm ${page === id ? 'bg-accent/15 font-semibold text-accent' : 'text-muted hover:bg-surface-2 hover:text-fg'}`}>
              <Icon size={18} /> <span className="hidden lg:inline">{t(`nav.${id}`)}</span>
            </button>
          ))}
          <button className="mt-auto flex items-center gap-3 rounded-lg px-3 py-2 text-sm text-muted hover:text-fg" title="Sair"
            onClick={() => { api.post('/api/v1/auth/logout').catch(() => {}); setToken(null); }}>
            <LogOut size={18} /> <span className="hidden lg:inline">Sair</span>
          </button>
        </nav>
        <main className="min-w-0 flex-1 overflow-auto">
          {!state.live && <div className="bg-warn/15 px-4 py-1.5 text-center text-xs text-warn">Reconectando ao servidor… os dados podem estar desatualizados.</div>}
          {page === 'dashboard' && <Dashboard />}
          {page === 'devices' && <Devices />}
          {page === 'obs' && <OBSPage />}
          {page === 'network' && <Network />}
          {page === 'settings' && <SettingsPage />}
          {page === 'events' && <Events />}
        </main>
      </div>
      <Toasts />
    </div>
  );
}
