import { describe, expect, it } from 'vitest';
import { initialState, messageToAction, reducer, sortedCameras } from './state';
import { formatBitrate, formatDuration, gridColumns, signalBars } from './format';
import type { CameraView } from './types';

const cam = (id: string, slot: number, extra: Partial<CameraView> = {}): CameraView =>
  ({
    id, slot, name: `Câmera ${slot}`, operatorName: '', profileId: 'auto', ingestMode: 'rtsp', isBackup: false,
    audioSyncOffsetMs: 0, mediaPath: `sl/${id}`, state: 'STREAMING', tally: 'none', program: false, preview: false,
    controlConnected: true, bitrateKbps: 2000, hasAudio: true, packetsLost: 0, charging: false, micMuted: false, torch: false,
    obsSource: { cameraId: id, state: 'present' }, ...extra,
  }) as CameraView;

describe('reducer', () => {
  it('aplica snapshot e atualizações incrementais', () => {
    let s = reducer(initialState, messageToAction('snapshot', { cameras: [cam('b', 2), cam('a', 1)], obs: null, mediaUp: true, settings: null })!);
    expect(sortedCameras(s).map((c) => c.id)).toEqual(['a', 'b']);
    s = reducer(s, messageToAction('camera', cam('a', 1, { tally: 'on_air', program: true }))!);
    expect(s.cameras.a.tally).toBe('on_air');
    expect(s.loaded).toBe(true);
  });

  it('eventos de erro geram notificação', () => {
    const s = reducer(initialState, { type: 'event', data: { id: 1, ts: 0, level: 'error', kind: 'camera_lost_on_air', message: 'caiu' } });
    expect(s.toasts).toHaveLength(1);
    expect(s.events[0].message).toBe('caiu');
  });

  it('ignora mensagens desconhecidas', () => {
    expect(messageToAction('xyz', {})).toBeNull();
  });
});

describe('formatação', () => {
  it('bitrate', () => {
    expect(formatBitrate(850)).toBe('850 kbps');
    expect(formatBitrate(2824)).toBe('2,8 Mbps');
  });
  it('duração', () => {
    expect(formatDuration(1000, 1000 + 65_000)).toBe('01:05');
    expect(formatDuration(0)).toBe('—');
    expect(formatDuration(1, 1 + 3_725_000)).toBe('1:02:05');
  });
  it('grade para 1, 2, 4, 6 e 8 câmeras', () => {
    expect(gridColumns(1)).toContain('grid-cols-1');
    expect(gridColumns(3)).toContain('xl:grid-cols-3');
    expect(gridColumns(8)).toContain('xl:grid-cols-4');
  });
  it('barras de sinal', () => {
    expect(signalBars({ state: 'OFFLINE', bitrateKbps: 0, packetsLost: 0 })).toBe(0);
    expect(signalBars({ state: 'STREAMING', bitrateKbps: 2500, packetsLost: 0, jitterMs: 5, rttMs: 40 })).toBe(4);
    expect(signalBars({ state: 'STREAMING', bitrateKbps: 200, packetsLost: 10, jitterMs: 80, rttMs: 500 })).toBe(1);
  });
});
