// Cliente da Admin API do SacraLink (REST + WebSocket).

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

declare global {
  interface Window {
    go?: { main?: { App?: { ServerURL?: () => Promise<string> } } };
  }
}

/** Descobre a URL do servidor: Wails (binding), variável do Vite ou mesma origem. */
export async function resolveServerURL(): Promise<string> {
  try {
    const fromWails = await window.go?.main?.App?.ServerURL?.();
    if (fromWails) return fromWails;
  } catch {
    /* fora do Wails */
  }
  const env = import.meta.env.VITE_SERVER_URL as string | undefined;
  if (env) return env;
  // Painel servido pelo próprio sacralink-server (navegador local).
  const local = ['127.0.0.1', 'localhost', '[::1]'].includes(location.hostname);
  if (location.protocol === 'http:' && local && location.port !== '5173') return location.origin;
  return 'http://127.0.0.1:7380';
}

export class Api {
  token: string | null = null;
  onUnauthorized: () => void = () => {};

  constructor(public base: string) {}

  async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = {};
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (this.token) headers['Authorization'] = `Bearer ${this.token}`;
    let resp: Response;
    try {
      resp = await fetch(this.base + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
    } catch {
      throw new ApiError(0, 'server_unreachable', 'Servidor SacraLink indisponível. Verifique se ele está em execução (ícone na bandeja).');
    }
    if (resp.status === 204) return undefined as T;
    const text = await resp.text();
    const data = text ? JSON.parse(text) : undefined;
    if (!resp.ok) {
      const err = data?.error ?? {};
      if (resp.status === 401 && path !== '/api/v1/auth/login') this.onUnauthorized();
      throw new ApiError(resp.status, err.code ?? 'error', err.message ?? `Erro HTTP ${resp.status}`);
    }
    return data as T;
  }

  get = <T>(path: string) => this.request<T>('GET', path);
  post = <T>(path: string, body: unknown = {}) => this.request<T>('POST', path, body);
  put = <T>(path: string, body: unknown) => this.request<T>('PUT', path, body);
  patch = <T>(path: string, body: unknown) => this.request<T>('PATCH', path, body);
  del = <T>(path: string) => this.request<T>('DELETE', path);

  /** Abre o WebSocket de eventos com ticket de uso único. */
  async openEvents(onMessage: (type: string, data: unknown) => void, onClose: () => void): Promise<WebSocket> {
    const { ticket } = await this.post<{ ticket: string }>('/api/v1/auth/ws-ticket');
    const ws = new WebSocket(this.base.replace(/^http/, 'ws') + '/api/v1/ws?ticket=' + encodeURIComponent(ticket));
    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data as string);
        onMessage(msg.type, msg.data);
      } catch {
        /* mensagem inválida */
      }
    };
    ws.onclose = onClose;
    return ws;
  }
}

/** Mensagem amigável de erro para exibir ao operador. */
export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return String(e);
}
