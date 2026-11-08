// Formatação de valores para o painel (pt-BR).

export function formatBitrate(kbps: number): string {
  if (!kbps || kbps < 1) return '0 kbps';
  if (kbps >= 1000) return `${(kbps / 1000).toFixed(1).replace('.', ',')} Mbps`;
  return `${Math.round(kbps)} kbps`;
}

export function formatDuration(sinceMs?: number, now = Date.now()): string {
  if (!sinceMs) return '—';
  const s = Math.max(0, Math.floor((now - sinceMs) / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  const pad = (n: number) => String(n).padStart(2, '0');
  return h > 0 ? `${h}:${pad(m)}:${pad(sec)}` : `${pad(m)}:${pad(sec)}`;
}

export function formatBytes(b: number): string {
  if (b >= 1 << 30) return `${(b / (1 << 30)).toFixed(1).replace('.', ',')} GB`;
  if (b >= 1 << 20) return `${(b / (1 << 20)).toFixed(0)} MB`;
  return `${Math.round(b / 1024)} KB`;
}

export function formatResolution(w?: number, h?: number, fps?: number): string {
  if (!w || !h) return '—';
  const p = `${h}p`;
  return fps ? `${p}${Math.round(fps)}` : p;
}

export function formatTime(ms: number): string {
  return new Date(ms).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

/** Qualidade do sinal (0–4 barras) a partir de perda de pacotes, jitter e RTT. */
export function signalBars(c: { bitrateKbps: number; packetsLost: number; jitterMs?: number; rttMs?: number; state: string }): number {
  if (c.state !== 'STREAMING') return 0;
  let bars = 4;
  if ((c.jitterMs ?? 0) > 30) bars--;
  if ((c.rttMs ?? 0) > 150) bars--;
  if ((c.rttMs ?? 0) > 400) bars--;
  if (c.bitrateKbps < 300) bars--;
  return Math.max(1, bars);
}

/** Layout da grade multicâmera para 1, 2, 4, 6 ou 8 câmeras. */
// As colunas crescem com a largura para que a grade inteira caiba na tela
// de produção (ex.: 1600×1000) sem rolagem.
export function gridColumns(n: number): string {
  if (n <= 1) return 'grid-cols-1 mx-auto w-full max-w-5xl';
  if (n <= 2) return 'grid-cols-1 md:grid-cols-2';
  if (n <= 3) return 'grid-cols-1 md:grid-cols-2 xl:grid-cols-3';
  if (n <= 4) return 'grid-cols-1 md:grid-cols-2 2xl:grid-cols-4';
  if (n <= 6) return 'grid-cols-1 md:grid-cols-2 xl:grid-cols-3';
  return 'grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4';
}

/**
 * A imagem recebida está em orientação diferente da escolhida no painel?
 * (ex.: câmera configurada como vertical e celular deitado). Sem vídeo, não.
 */
export function orientationMismatch(c: { orientation?: 'landscape' | 'portrait'; width?: number; height?: number }): boolean {
  if (!c.width || !c.height || c.width === c.height) return false;
  return (c.orientation === 'portrait') !== c.height > c.width;
}
