import type { APIError, Job, Playback, Provider, SearchResponse } from './types';

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://127.0.0.1:8080';
const API_KEY = import.meta.env.VITE_API_KEY ?? '';

export class MusicAPIError extends Error { constructor(public code:string, message:string, public details?:unknown){ super(message); } }

async function request<T>(path:string, init:RequestInit = {}, protectedEndpoint=false):Promise<T>{
  const headers = new Headers(init.headers);
  headers.set('Accept','application/json');
  if(init.body) headers.set('Content-Type','application/json');
  if(protectedEndpoint && API_KEY) headers.set('X-API-Key',API_KEY);
  const response = await fetch(`${BASE_URL}${path}`,{...init,headers});
  if(!response.ok){ const body = await response.json().catch(()=>null) as APIError|null; throw new MusicAPIError(body?.error.code ?? `http_${response.status}`,body?.error.message ?? 'Backend request failed',body?.error.details); }
  return response.json() as Promise<T>;
}

export const api = {
  health:()=>request<{status:string}>('/health'),
  providers:()=>request<Provider[]>('/v1/providers'),
  search:(q:string,providers:string[]=[],limit=20,offset=0)=>{ const p=new URLSearchParams({q,limit:String(limit),offset:String(offset)}); if(providers.length)p.set('providers',providers.join(',')); return request<SearchResponse>(`/v1/search?${p}`); },
  playback:(trackId:string)=>request<Playback>(`/v1/playback/${encodeURIComponent(trackId)}`),
  createDownload:(trackId:string,format='mp3')=>request<Job>('/v1/downloads',{method:'POST',body:JSON.stringify({track_id:trackId,format})},true),
};
