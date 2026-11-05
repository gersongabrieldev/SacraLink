import { createContext, useCallback, useContext, useEffect, useMemo, useReducer, useRef, useState, type ReactNode } from 'react';
import { Api, errorMessage, resolveServerURL } from './api';
import { initialState, messageToAction, reducer, type Action, type State } from './state';
import type { EventItem } from './types';

interface Ctx {
  api: Api;
  state: State;
  dispatch: (a: Action) => void;
  authed: boolean;
  setToken: (t: string | null) => void;
  notify: (level: 'info' | 'success' | 'warn' | 'error', message: string) => void;
  /** Executa uma ação e mostra o erro como notificação. */
  run: <T>(fn: () => Promise<T>, success?: string) => Promise<T | undefined>;
}

const StoreContext = createContext<Ctx | null>(null);

const TOKEN_KEY = 'sacralink.token';

function loadToken(): string | null {
  try {
    return sessionStorage.getItem(TOKEN_KEY);
  } catch {
    return null;
  }
}

function saveToken(t: string | null) {
  try {
    if (t) sessionStorage.setItem(TOKEN_KEY, t);
    else sessionStorage.removeItem(TOKEN_KEY);
  } catch {
    /* armazenamento indisponível */
  }
}

export function StoreProvider({ api, children }: { api: Api; children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initialState);
  const [token, setTokenState] = useState<string | null>(() => loadToken());
  api.token = token;

  const setToken = useCallback(
    (t: string | null) => {
      api.token = t;
      saveToken(t);
      setTokenState(t);
      if (!t) dispatch({ type: 'reset' });
    },
    [api],
  );

  useEffect(() => {
    api.onUnauthorized = () => setToken(null);
  }, [api, setToken]);

  // WebSocket com reconexão exponencial enquanto houver sessão.
  const retry = useRef(1000);
  useEffect(() => {
    if (!token) return;
    let ws: WebSocket | null = null;
    let stopped = false;
    let timer: number | undefined;
    const connect = async () => {
      try {
        ws = await api.openEvents(
          (type, data) => {
            const a = messageToAction(type, data);
            if (a) dispatch(a);
          },
          () => {
            dispatch({ type: 'ws', live: false });
            if (!stopped) timer = window.setTimeout(connect, (retry.current = Math.min(retry.current * 2, 10000)));
          },
        );
        ws.onopen = () => {
          retry.current = 1000;
          dispatch({ type: 'ws', live: true });
          api.get<EventItem[]>('/api/v1/events?limit=100').then((ev) => dispatch({ type: 'events', data: ev })).catch(() => {});
        };
      } catch {
        dispatch({ type: 'ws', live: false });
        if (!stopped) timer = window.setTimeout(connect, (retry.current = Math.min(retry.current * 2, 10000)));
      }
    };
    connect();
    return () => {
      stopped = true;
      window.clearTimeout(timer);
      ws?.close();
    };
  }, [api, token]);

  const notify = useCallback((level: 'info' | 'success' | 'warn' | 'error', message: string) => {
    dispatch({ type: 'toast', toast: { level, message } });
  }, []);

  const run = useCallback(
    async <T,>(fn: () => Promise<T>, success?: string) => {
      try {
        const r = await fn();
        if (success) notify('success', success);
        return r;
      } catch (e) {
        notify('error', errorMessage(e));
        return undefined;
      }
    },
    [notify],
  );

  const value = useMemo(() => ({ api, state, dispatch, authed: !!token, setToken, notify, run }), [api, state, token, setToken, notify, run]);
  return <StoreContext.Provider value={value}>{children}</StoreContext.Provider>;
}

export function useStore(): Ctx {
  const c = useContext(StoreContext);
  if (!c) throw new Error('useStore fora do StoreProvider');
  return c;
}

export { resolveServerURL };
