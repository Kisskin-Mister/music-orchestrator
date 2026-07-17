import { create } from 'zustand';
import type { Playback, Track } from '@/api/types';

type PlayerStatus='idle'|'resolving'|'playing'|'paused'|'buffering'|'unavailable'|'expired'|'error';
type PlayerState={ currentTrack:Track|null; playback:Playback|null; status:PlayerStatus; queue:Track[]; volume:number; error:string|null; setTrack:(track:Track)=>void; setPlayback:(playback:Playback)=>void; setStatus:(status:PlayerStatus)=>void; setVolume:(volume:number)=>void; setError:(error:string|null)=>void };

export const usePlayerStore=create<PlayerState>((set)=>({currentTrack:null,playback:null,status:'idle',queue:[],volume:.72,error:null,setTrack:(currentTrack)=>set({currentTrack,status:'resolving',error:null}),setPlayback:(playback)=>set({playback,status:playback.playback_type==='unavailable'?'unavailable':'paused'}),setStatus:(status)=>set({status}),setVolume:(volume)=>set({volume}),setError:(error)=>set({error,status:error?'error':'idle'})}));
