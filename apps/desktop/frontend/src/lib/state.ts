// Estado do painel alimentado pelo WebSocket da Admin API. O reducer é puro
// (testado em state.test.ts).
import type { AudioLevel, CameraView, EventItem, MediaStatus, OBSStatus, Settings, SystemBrief } from './types';

export interface Toast {
  id: number;
  level: 'info' | 'success' | 'warn' | 'error';
  message: string;
}

export interface State {
  live: boolean; // WebSocket conectado
  loaded: boolean;
  cameras: Record<string, CameraView>;
  obs: OBSStatus | null;
  settings: Settings | null;
  system: SystemBrief | null;
  media: MediaStatus | null;
  mediaUp: boolean;
  events: EventItem[];
  audio: Record<string, AudioLevel>;
  toasts: Toast[];
  selected: string | null;
}

export const initialState: State = {
  live: false,
  loaded: false,
  cameras: {},
  obs: null,
  settings: null,
  system: null,
  media: null,
  mediaUp: false,
  events: [],
  audio: {},
  toasts: [],
  selected: null,
};

export type Action =
  | { type: 'ws'; live: boolean }
  | { type: 'snapshot'; data: { cameras: CameraView[]; obs: OBSStatus; mediaUp: boolean; settings: Settings } }
  | { type: 'camera'; data: CameraView }
  | { type: 'obs'; data: OBSStatus }
  | { type: 'settings'; data: Settings }
  | { type: 'system'; data: SystemBrief }
  | { type: 'media'; data: MediaStatus }
  | { type: 'event'; data: EventItem }
  | { type: 'events'; data: EventItem[] }
  | { type: 'audio'; data: AudioLevel[] }
  | { type: 'toast'; toast: Omit<Toast, 'id'> }
  | { type: 'dismiss'; id: number }
  | { type: 'select'; id: string | null }
  | { type: 'reset' };

let toastSeq = 0;

export function reducer(s: State, a: Action): State {
  switch (a.type) {
    case 'ws':
      return { ...s, live: a.live };
    case 'snapshot': {
      const cameras: Record<string, CameraView> = {};
      for (const c of a.data.cameras ?? []) cameras[c.id] = c;
      return { ...s, loaded: true, cameras, obs: a.data.obs, mediaUp: a.data.mediaUp, settings: a.data.settings };
    }
    case 'camera':
      return { ...s, cameras: { ...s.cameras, [a.data.id]: a.data } };
    case 'obs':
      return { ...s, obs: a.data };
    case 'settings':
      return { ...s, settings: a.data };
    case 'system':
      return { ...s, system: a.data, media: a.data.media, mediaUp: a.data.media?.state === 'running' };
    case 'media':
      return { ...s, media: a.data, mediaUp: a.data.state === 'running' };
    case 'event': {
      const events = [a.data, ...s.events.filter((e) => e.id !== a.data.id)].slice(0, 200);
      const toasts =
        a.data.level === 'error' ? [...s.toasts, { id: ++toastSeq, level: 'error' as const, message: a.data.message }].slice(-5) : s.toasts;
      return { ...s, events, toasts };
    }
    case 'events':
      return { ...s, events: a.data };
    case 'audio': {
      const audio: Record<string, AudioLevel> = {};
      for (const l of a.data) audio[l.inputName] = l;
      return { ...s, audio };
    }
    case 'toast':
      return { ...s, toasts: [...s.toasts, { ...a.toast, id: ++toastSeq }].slice(-5) };
    case 'dismiss':
      return { ...s, toasts: s.toasts.filter((t) => t.id !== a.id) };
    case 'select':
      return { ...s, selected: a.id };
    case 'reset':
      return initialState;
  }
}

/** Converte mensagens do WebSocket em ações. */
export function messageToAction(type: string, data: unknown): Action | null {
  switch (type) {
    case 'snapshot':
    case 'camera':
    case 'obs':
    case 'settings':
    case 'system':
    case 'media':
    case 'event':
    case 'audio':
      return { type, data } as Action;
    case 'device_paired':
      return { type: 'toast', toast: { level: 'success', message: `Novo aparelho pareado: ${(data as { name?: string })?.name ?? ''}` } };
  }
  return null;
}

/** Câmeras ordenadas por slot. */
export function sortedCameras(s: State): CameraView[] {
  return Object.values(s.cameras).sort((a, b) => a.slot - b.slot);
}
