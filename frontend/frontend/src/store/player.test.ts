import { beforeEach, describe, expect, it } from 'vitest';
import { correctedDurationSeconds, usePlayerStore } from './player';
import type { Track } from '@/api/types';

function track(id: string, duration?: number): Track {
  return {
    id,
    provider_id: 'youtube_stream',
    provider_track_id: id,
    title: `Track ${id}`,
    duration_seconds: duration,
    downloaded: false,
    capabilities: { search: true, metadata: true, embed_playback: false, official_stream_url: false, raw_audio_stream: true, persistent_cache: true, offline_playback: true, public_deployment_safe: false },
    policy: { download_allowed: true, cache_allowed: true, requires_attribution: false, notes: [] },
  };
}

describe('correctedDurationSeconds', () => {
  it('prefers real media duration over wrong provider metadata', () => {
    expect(correctedDurationSeconds(201, 101)).toBe(101);
  });

  it('keeps provider duration when it matches media within a second', () => {
    expect(correctedDurationSeconds(101, 101.4)).toBe(101);
  });

  it('falls back to media duration when provider has none', () => {
    expect(correctedDurationSeconds(undefined, 95)).toBe(95);
    expect(correctedDurationSeconds(0, 95)).toBe(95);
  });

  it('keeps provider duration when the audio element overreports the container length', () => {
    // YouTube CDN Content-Length covers video+audio, so the element claims ~2x.
    expect(correctedDurationSeconds(200, 400)).toBe(200);
    expect(correctedDurationSeconds(200, 261)).toBe(200);
    // Just under the threshold is still treated as a real audio-element reading.
    expect(correctedDurationSeconds(200, 259)).toBe(259);
  });

  it('ignores invalid media durations', () => {
    expect(correctedDurationSeconds(201, 0)).toBe(201);
    expect(correctedDurationSeconds(201, Number.NaN)).toBe(201);
    expect(correctedDurationSeconds(201, Number.POSITIVE_INFINITY)).toBe(201);
    expect(correctedDurationSeconds(undefined, 0)).toBeUndefined();
  });
});

describe('player store updateDuration', () => {
  beforeEach(() => {
    usePlayerStore.setState({ currentTrack: null, playback: null, status: 'idle', queue: [], queueIndex: -1, error: null });
  });

  it('corrects current track and queue entry from real audio duration', () => {
    usePlayerStore.getState().playQueue([track('a', 201), track('b', 60)], 0);
    usePlayerStore.getState().updateDuration(101);
    const state = usePlayerStore.getState();
    expect(state.currentTrack?.duration_seconds).toBe(101);
    expect(state.queue[0].duration_seconds).toBe(101);
    expect(state.queue[1].duration_seconds).toBe(60);
  });

  it('does nothing for invalid durations or missing track', () => {
    usePlayerStore.getState().setTrack(track('a', 201));
    usePlayerStore.getState().updateDuration(0);
    expect(usePlayerStore.getState().currentTrack?.duration_seconds).toBe(201);
    usePlayerStore.setState({ currentTrack: null, queue: [] });
    usePlayerStore.getState().updateDuration(101);
    expect(usePlayerStore.getState().currentTrack).toBeNull();
  });
});
