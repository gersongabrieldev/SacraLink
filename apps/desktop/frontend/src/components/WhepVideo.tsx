import { useEffect, useRef, useState } from 'react';
import { useStore } from '../lib/store';

/**
 * Prévia da câmera via WHEP (WebRTC) direto do MediaMTX local, decodificada
 * por hardware no WebView2. Sempre mutada: o áudio é monitorado pelo OBS,
 * evitando microfonia. Só conecta quando `active` (modo de prévia).
 */
export function WhepVideo({ cameraId, active }: { cameraId: string; active: boolean }) {
  const { api } = useStore();
  const ref = useRef<HTMLVideoElement>(null);
  const [status, setStatus] = useState<'idle' | 'connecting' | 'playing' | 'error'>('idle');

  useEffect(() => {
    if (!active) {
      setStatus('idle');
      return;
    }
    let pc: RTCPeerConnection | null = null;
    let location: string | null = null;
    let cancelled = false;
    let retry: number | undefined;

    const start = async () => {
      setStatus('connecting');
      pc = new RTCPeerConnection();
      pc.addTransceiver('video', { direction: 'recvonly' });
      pc.addTransceiver('audio', { direction: 'recvonly' });
      pc.ontrack = (e) => {
        if (ref.current && ref.current.srcObject !== e.streams[0]) ref.current.srcObject = e.streams[0];
      };
      pc.onconnectionstatechange = () => {
        if (!pc) return;
        if (pc.connectionState === 'connected') setStatus('playing');
        if (pc.connectionState === 'failed' || pc.connectionState === 'disconnected') schedule();
      };
      try {
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);
        await new Promise<void>((res) => {
          if (!pc || pc.iceGatheringState === 'complete') return res();
          const t = window.setTimeout(res, 800);
          pc.addEventListener('icegatheringstatechange', () => {
            if (pc?.iceGatheringState === 'complete') {
              window.clearTimeout(t);
              res();
            }
          });
        });
        const resp = await fetch(`${api.base}/whep/${encodeURIComponent(cameraId)}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/sdp', Authorization: `Bearer ${api.token}` },
          body: pc.localDescription?.sdp,
        });
        if (resp.status !== 201) throw new Error(String(resp.status));
        const loc = resp.headers.get('Location');
        location = loc ? api.base + loc : null;
        const answer = await resp.text();
        if (cancelled) return;
        await pc.setRemoteDescription({ type: 'answer', sdp: answer });
      } catch {
        schedule();
      }
    };

    const stop = () => {
      pc?.close();
      pc = null;
      if (location) {
        fetch(location, { method: 'DELETE', headers: { Authorization: `Bearer ${api.token}` } }).catch(() => {});
        location = null;
      }
    };

    const schedule = () => {
      if (cancelled) return;
      setStatus('error');
      stop();
      window.clearTimeout(retry);
      retry = window.setTimeout(start, 2500);
    };

    start();
    return () => {
      cancelled = true;
      window.clearTimeout(retry);
      stop();
    };
  }, [api, cameraId, active]);

  return (
    <div className="relative aspect-video w-full overflow-hidden rounded-lg bg-black">
      {active && <video ref={ref} className="h-full w-full object-contain" autoPlay playsInline muted />}
      {status !== 'playing' && (
        <div className="absolute inset-0 flex items-center justify-center text-xs text-white/60">
          {!active ? 'Prévia desativada' : status === 'error' ? 'Aguardando vídeo…' : 'Conectando prévia…'}
        </div>
      )}
    </div>
  );
}
