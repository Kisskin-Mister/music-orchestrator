import { create } from 'zustand';
import type { Playback, Track } from '@/api/types';

// Насколько media duration должен превысить provider metadata, чтобы считать
// его мусорным: YouTube CDN отдаёт Content-Length всего video+audio контейнера,
// и audio element рапортует примерно двойную длину.
export const OVERREPORTED_DURATION_RATIO = 1.3;

// Реальная длительность из audio element важнее provider metadata:
// поисковая выдача yt-dlp иногда отдаёт duration другого ролика.
// Исключение — случай выше: там завышен именно audio element, и провайдер прав.
export function correctedDurationSeconds(providerSeconds: number | undefined, mediaSeconds: number | undefined): number | undefined {
  const provider = typeof providerSeconds === 'number' && Number.isFinite(providerSeconds) && providerSeconds > 0 ? Math.round(providerSeconds) : undefined;
  if (typeof mediaSeconds === 'number' && Number.isFinite(mediaSeconds) && mediaSeconds > 0) {
    const media = Math.round(mediaSeconds);
    if (provider === undefined) return media;
    if (media > provider * OVERREPORTED_DURATION_RATIO) return provider;
    if (Math.abs(provider - media) > 1) return media;
    return provider;
  }
  return provider;
}

function localPlayback(track: Track): Playback | null {
  if (!track.download_media_url) return null;
  return {
    track_id: track.id,
    provider_id: track.provider_id,
    playback_type: 'local_cached_stream',
    stream_url: track.download_media_url,
    embed_url: null,
    expires_in_seconds: null,
    attribution: 'saved copy',
    capabilities: track.capabilities,
    policy: track.policy,
  };
}

type PlayerStatus='idle'|'resolving'|'playing'|'paused'|'buffering'|'unavailable'|'expired'|'error';
type RepeatMode='off'|'all'|'one';
type PlayerState={
  currentTrack:Track|null;
  playback:Playback|null;
  status:PlayerStatus;
  queue:Track[];
  queueIndex:number;
  volume:number;
  error:string|null;
  shuffle:boolean;
  repeat:RepeatMode;
  setTrack:(track:Track)=>void;
  playQueue:(tracks:Track[],index:number)=>void;
  toggleTrack:(track:Track,queue?:Track[],index?:number)=>void;
  playNext:(track:Track)=>void;
  addToQueue:(track:Track)=>void;
  next:()=>void;
  previous:()=>void;
  setPlayback:(playback:Playback)=>void;
  updateDuration:(seconds:number)=>void;
  setStatus:(status:PlayerStatus)=>void;
  setVolume:(volume:number)=>void;
  setError:(error:string|null)=>void;
  toggleShuffle:()=>void;
  cycleRepeat:()=>void;
};

function startTrack(track:Track){
  const cachedPlayback = localPlayback(track);
  return {currentTrack:track,playback:cachedPlayback,status:cachedPlayback?'playing' as const:'resolving' as const,error:null};
}

export const usePlayerStore=create<PlayerState>((set,get)=>({
  currentTrack:null,
  playback:null,
  status:'idle',
  queue:[],
  queueIndex:-1,
  volume:.72,
  error:null,
  shuffle:false,
  repeat:'off',
  setTrack:(currentTrack)=>set({...startTrack(currentTrack),queue:[currentTrack],queueIndex:0}),
  playQueue:(tracks,index)=>{
    const safeIndex=Math.max(0,Math.min(index,tracks.length-1));
    const track=tracks[safeIndex];
    if(!track) return;
    set({...startTrack(track),queue:tracks,queueIndex:safeIndex});
  },
  toggleTrack:(track,queue,index)=>{
    const { currentTrack, status, playback } = get();
    if (currentTrack?.id === track.id) {
      if (status === 'playing') set({status:'paused'});
      else set({status:'playing', playback: playback ?? localPlayback(track), error:null});
      return;
    }
    const nextQueue = queue?.length ? queue : get().queue.length ? get().queue : [track];
    const nextIndex = typeof index === 'number' ? index : Math.max(0,nextQueue.findIndex((item)=>item.id===track.id));
    set({...startTrack(track),queue:nextQueue,queueIndex:nextIndex});
  },
  playNext:(track)=>{
    const { queue, currentTrack } = get();
    if (!currentTrack || !queue.length) { set({...startTrack(track),queue:[track],queueIndex:0}); return; }
    const filtered = queue.filter((item)=>item.id !== track.id);
    const currentIndex = Math.max(0, filtered.findIndex((item)=>item.id === currentTrack.id));
    filtered.splice(currentIndex + 1, 0, track);
    set({queue:filtered, queueIndex:currentIndex});
  },
  addToQueue:(track)=>{
    const { queue } = get();
    if (!queue.length) { set({...startTrack(track),queue:[track],queueIndex:0}); return; }
    if (queue.some((item)=>item.id === track.id)) return;
    set({queue:[...queue, track]});
  },
  next:()=>{
    const { queue, queueIndex, shuffle, repeat, currentTrack } = get();
    if (!queue.length) return;
    if (repeat === 'one' && currentTrack) { set({...startTrack(currentTrack)}); return; }
    const nextIndex = shuffle ? Math.floor(Math.random()*queue.length) : queueIndex + 1;
    if (nextIndex >= queue.length) {
      if (repeat !== 'all') { set({status:'paused'}); return; }
      set({...startTrack(queue[0]),queueIndex:0});
      return;
    }
    set({...startTrack(queue[nextIndex]),queueIndex:nextIndex});
  },
  previous:()=>{
    const { queue, queueIndex } = get();
    if (!queue.length) return;
    const previousIndex = queueIndex <= 0 ? queue.length-1 : queueIndex-1;
    set({...startTrack(queue[previousIndex]),queueIndex:previousIndex});
  },
  setPlayback:(playback)=>set({playback,status:playback.playback_type==='unavailable'?'unavailable':'playing'}),
  updateDuration:(seconds)=>{
    const { currentTrack, queue } = get();
    if (!currentTrack) return;
    const corrected = correctedDurationSeconds(currentTrack.duration_seconds, seconds);
    if (!corrected || corrected === currentTrack.duration_seconds) return;
    set({
      currentTrack: { ...currentTrack, duration_seconds: corrected },
      queue: queue.map((item) => item.id === currentTrack.id ? { ...item, duration_seconds: corrected } : item),
    });
  },
  setStatus:(status)=>set({status}),
  setVolume:(volume)=>set({volume}),
  setError:(error)=>set({error,status:error?'error':'idle'}),
  toggleShuffle:()=>set((s)=>({shuffle:!s.shuffle})),
  cycleRepeat:()=>set((s)=>({repeat:s.repeat==='off'?'all':s.repeat==='all'?'one':'off'})),
}));
