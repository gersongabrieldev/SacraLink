// Tipos espelhando os contratos JSON da Admin API (apps/server/internal).

export type ConnState = 'OFFLINE' | 'CONNECTING' | 'CONNECTED' | 'STREAMING' | 'RECONNECTING' | 'ERROR';
export type TallyLabel = 'on_air' | 'preview' | 'none' | 'unknown';

export interface Lens {
  id: string;
  facing: string;
  kind: string;
  label: string;
  minZoom: number;
  maxZoom: number;
  opticalZoomFactors?: number[];
}

export interface Capabilities {
  lenses: Lens[];
  torch: boolean;
  focusPoint: boolean;
  exposurePoint: boolean;
  maxWidth: number;
  maxHeight: number;
  maxFps: number;
  hardwareH264: boolean;
  thermal: boolean;
}

export interface SourceStatus {
  cameraId: string;
  inputName?: string;
  sceneName?: string;
  inputKind?: string;
  state: 'present' | 'missing' | 'none' | 'unknown';
}

export interface CommandView {
  commandId: string;
  name: string;
  status: string;
  message?: string;
  at: number;
}

export interface CameraView {
  id: string;
  slot: number;
  name: string;
  operatorName: string;
  profileId: string;
  ingestMode: 'rtsp' | 'whep';
  isBackup: boolean;
  audioSyncOffsetMs: number;
  mediaPath: string;
  device?: { id: string; name: string; platform: string; model: string; appVersion: string; status: string };
  state: ConnState;
  tally: TallyLabel;
  program: boolean;
  preview: boolean;
  controlConnected: boolean;
  connectedSince?: number;
  streamingSince?: number;
  remoteAddr?: string;
  error?: string;
  bitrateKbps: number;
  width?: number;
  height?: number;
  fps?: number;
  videoCodec?: string;
  hasAudio: boolean;
  packetsLost: number;
  jitterMs?: number;
  rttMs?: number;
  battery?: number;
  charging: boolean;
  thermal?: string;
  network?: string;
  zoom?: number;
  lensId?: string;
  facing?: string;
  micMuted: boolean;
  torch: boolean;
  capabilities?: Capabilities;
  obsSource: SourceStatus;
  lastLocalChange?: { param: string; at: number };
  lastCommand?: CommandView;
}

export interface OBSStatus {
  state: 'disabled' | 'disconnected' | 'connecting' | 'connected' | 'auth_failed';
  error?: string;
  address: string;
  obsVersion?: string;
  wsVersion?: string;
  connectedAt?: number;
  studioMode: boolean;
  programScene?: string;
  previewScene?: string;
  streaming: boolean;
  recording: boolean;
  streamStartedAt?: number;
  recordStartedAt?: number;
  canvasWidth?: number;
  canvasHeight?: number;
  reconnectInSec?: number;
}

export interface Settings {
  serverName: string;
  firstRunComplete: boolean;
  maxStreams: number;
  iceTcp: boolean;
  previewMode: 'all' | 'selected' | 'off';
  language: string;
  theme: 'dark' | 'light';
  obs: { autoManageSources: boolean; allowDirectSwitch: boolean; defaultScene: string; defaultTransition: string };
  fallback: { mode: 'stay' | 'backup' | 'scene'; sceneName: string; graceSec: number };
  remote: { enabled: boolean; publicHost: string; publicPort: number; useStun: boolean };
  audio: { mainInput: string; backupInput: string };
}

export interface MediaStatus {
  state: 'stopped' | 'starting' | 'running' | 'restarting' | 'failed';
  pid?: number;
  restarts: number;
  lastError?: string;
  version?: string;
}

export interface SystemBrief {
  stats: { cpuPercent: number; memTotalBytes: number; memUsedBytes: number; memPercent: number; numCpu: number };
  camerasConnected: number;
  camerasStreaming: number;
  inboundKbps: number;
  media: MediaStatus;
  uptimeSec: number;
  live: boolean;
}

export interface SystemStatus extends SystemBrief {
  version: { version: string; commit: string; date: string };
  os: { name: string; build: number; isWindows10: boolean; warning?: string; smartAppControl?: string; supported: boolean };
  obs: OBSStatus;
  adminUrl: string;
  devicePort: number;
  dataDir: string;
  lanHosts: string[];
  fingerprint: string;
  autostart: boolean;
}

export interface EventItem {
  id: number;
  ts: number;
  level: 'info' | 'warn' | 'error';
  kind: string;
  cameraId?: string;
  message: string;
}

export interface AudioLevel {
  inputName: string;
  peakDb: number;
  magDb: number;
  channelsDb: number[];
  clipping: boolean;
}

export interface Device {
  id: string;
  name: string;
  platform: string;
  model: string;
  osVersion: string;
  appVersion: string;
  status: 'active' | 'revoked' | 'blocked';
  pairedAt: number;
  lastSeenAt: number;
  lastIp: string;
  permissions: string[];
  cameraId: string;
}

export interface SceneInfo {
  name: string;
  program: boolean;
  preview: boolean;
  sacralink: boolean;
  cameraIds: string[] | null;
}

export interface MediaProfile {
  id: string;
  name: string;
  width: number;
  height: number;
  fps: number;
  maxBitrateKbps: number;
  minBitrateKbps: number;
  adaptive: boolean;
  builtin: boolean;
}

export interface Advice {
  level: 'ok' | 'info' | 'warn' | 'error';
  title: string;
  message: string;
}

export interface NetworkReport {
  generatedAt: number;
  primaryIpv4: string;
  lanHosts: string[];
  publicIpv4?: string;
  publicIpv6?: string;
  globalIpv6?: string[];
  internet: boolean;
  cgnat: 'no' | 'likely' | 'yes' | 'unknown';
  remoteEnabled: boolean;
  advice: Advice[];
  interfaces: { name: string; up: boolean; virtual: boolean; addrs: { ip: string; family: number; kind: string }[] }[];
}

export interface CommandResult {
  commandId: string;
  status: 'executed' | 'rejected' | 'unsupported' | 'failed' | 'conflict' | 'expired' | 'timeout' | 'offline';
  message?: string;
  state?: Record<string, unknown>;
}

export interface Preset {
  id: string;
  cameraId: string;
  name: string;
  params: { lensId?: string; zoom?: number; profileId?: string; torch?: boolean; micMuted?: boolean };
  createdAt: number;
}
