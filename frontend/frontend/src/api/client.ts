import type { APIError, Favorite, Job, Playback, Playlist, Provider, SearchResponse, SessionInfo, User, ServerSettings, ServerSettingsPatch } from './types';

const ENV_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '';
const API_KEY = import.meta.env.VITE_API_KEY ?? '';
const BACKEND_URL_STORAGE_KEY = 'music.backendUrl';

export function getBackendBaseURL() {
  if (typeof window === 'undefined') return ENV_BASE_URL;
  return window.localStorage.getItem(BACKEND_URL_STORAGE_KEY) ?? ENV_BASE_URL;
}

export function setBackendBaseURL(value: string) {
  const next = value.trim().replace(/\/+$/, '');
  if (typeof window === 'undefined') return;
  if (!next) window.localStorage.removeItem(BACKEND_URL_STORAGE_KEY);
  else window.localStorage.setItem(BACKEND_URL_STORAGE_KEY, next);
}

export function getDefaultBackendBaseURL() { return ENV_BASE_URL; }

/** Route cover art through the backend (see artwork.go): works on networks where
 *  the CDN is unreachable, and keeps the user's IP off third-party hosts. */
export function artworkURL(raw?: string | null) {
  if (!raw) return undefined;
  if (raw.startsWith('/')) return `${getBackendBaseURL()}${raw}`;
  return `${getBackendBaseURL()}/v1/artwork?url=${encodeURIComponent(raw)}`;
}
const DEFAULT_TIMEOUT_MS = 12_000;
const SEARCH_TIMEOUT_MS = 45_000;

export class MusicAPIError extends Error { constructor(public code:string, message:string, public details?:unknown){ super(message); } }

function timeoutSignal(parent?: AbortSignal, timeoutMs = DEFAULT_TIMEOUT_MS) {
  const controller = new AbortController();
  const timer = globalThis.setTimeout(() => controller.abort(new DOMException('Request timed out', 'TimeoutError')), timeoutMs);
  const abort = () => controller.abort(parent?.reason ?? new DOMException('Request cancelled', 'AbortError'));
  if (parent?.aborted) abort();
  else parent?.addEventListener('abort', abort, { once: true });
  return { signal: controller.signal, cleanup: () => { globalThis.clearTimeout(timer); parent?.removeEventListener('abort', abort); } };
}

async function request<T>(path:string, init:RequestInit = {}, protectedEndpoint=false, timeoutMs = DEFAULT_TIMEOUT_MS):Promise<T>{
  const headers = new Headers(init.headers);
  headers.set('Accept','application/json');
  if(init.body) headers.set('Content-Type','application/json');
  if(API_KEY) headers.set('X-API-Key',API_KEY);
  const { signal, cleanup } = timeoutSignal(init.signal ?? undefined, timeoutMs);
  try {
    const response = await fetch(`${getBackendBaseURL()}${path}`,{...init,headers,signal,credentials:'include'});
    if(!response.ok){ const body = await response.json().catch(()=>null) as APIError|null; throw new MusicAPIError(body?.error.code ?? `http_${response.status}`,body?.error.message ?? 'Backend request failed',body?.error.details); }
    if(response.status===204) return undefined as T;
    const contentType = response.headers.get('Content-Type') ?? '';
    if (!contentType.includes('application/json')) throw new MusicAPIError('bad_response', 'Backend returned non-JSON response');
    return response.json() as Promise<T>;
  } catch (error) {
    if (error instanceof MusicAPIError) throw error;
    if (error instanceof DOMException && error.name === 'AbortError') throw new MusicAPIError('cancelled', 'Request cancelled');
    if (error instanceof DOMException && error.name === 'TimeoutError') throw new MusicAPIError('timeout', 'Backend request timed out');
    throw error;
  } finally {
    cleanup();
  }
}

export const api = {
  health:()=>request<{status:string}>('/health'),
  providers:()=>request<Provider[]>('/v1/providers'),
  session:()=>request<SessionInfo>('/v1/auth/session'),
  register:(username:string,password:string,totpSecret='')=>request<SessionInfo>('/v1/auth/register',{method:'POST',body:JSON.stringify({username,password,totp_secret:totpSecret})}),
  login:(username:string,password:string)=>request<SessionInfo>('/v1/auth/login',{method:'POST',body:JSON.stringify({username,password})}),
  verify:(code:string)=>request<SessionInfo>('/v1/auth/verify',{method:'POST',body:JSON.stringify({code})}),
  logout:()=>request<void>('/v1/auth/logout',{method:'POST'}),
  updateAccount:(patch:{username:string;password?:string;totp_secret?:string})=>request<SessionInfo>('/v1/account',{method:'PATCH',body:JSON.stringify(patch)},true),
  settings:()=>request<ServerSettings>('/v1/settings',{},true),
  updateSettings:(patch:ServerSettingsPatch)=>request<ServerSettings>('/v1/settings',{method:'PATCH',body:JSON.stringify(patch)},true),
  users:()=>request<User[]>('/v1/users',{},true),
  createUser:(username:string,password:string)=>request<User>('/v1/users',{method:'POST',body:JSON.stringify({username,password})},true),
  updateUser:(userId:string,patch:{username?:string;password?:string})=>request<User>(`/v1/users/${encodeURIComponent(userId)}`,{method:'PATCH',body:JSON.stringify(patch)},true),
  deleteUser:(userId:string)=>request<void>(`/v1/users/${encodeURIComponent(userId)}`,{method:'DELETE'},true),
  search:(q:string,providers:string[]=[],limit=20,offset=0,signal?:AbortSignal)=>{ const p=new URLSearchParams({q,limit:String(limit),offset:String(offset)}); if(providers.length)p.set('providers',providers.join(',')); return request<SearchResponse>(`/v1/search?${p}`,{signal},false,SEARCH_TIMEOUT_MS); },
  playback:(trackId:string)=>request<Playback>(`/v1/playback/${encodeURIComponent(trackId)}`),
  createDownload:(trackId:string,format='mp3')=>request<Job>('/v1/downloads',{method:'POST',body:JSON.stringify({track_id:trackId,format})},true),
  downloads:()=>request<Job[]>('/v1/downloads',{},true),
  deleteDownload:(trackId:string)=>request<void>(`/v1/downloads/${encodeURIComponent(trackId)}`,{method:'DELETE'},true),
  favorites:()=>request<Favorite[]>('/v1/favorites',{},true),
  addFavorite:(trackId:string,track?:unknown)=>request<Favorite>('/v1/favorites',{method:'POST',body:JSON.stringify({track_id:trackId,track})},true),
  deleteFavorite:(trackId:string)=>request<void>(`/v1/favorites/${encodeURIComponent(trackId)}`,{method:'DELETE'},true),
  playlists:()=>request<Playlist[]>('/v1/playlists',{},true),
  playlist:(playlistId:string)=>request<Playlist>(`/v1/playlists/${encodeURIComponent(playlistId)}`,{},true),
  createPlaylist:(name:string,description='')=>request<Playlist>('/v1/playlists',{method:'POST',body:JSON.stringify({name,description})},true),
  updatePlaylist:(playlistId:string,patch:{name?:string;description?:string;cover_url?:string})=>request<Playlist>(`/v1/playlists/${encodeURIComponent(playlistId)}`,{method:'PATCH',body:JSON.stringify(patch)},true),
  deletePlaylist:(playlistId:string)=>request<void>(`/v1/playlists/${encodeURIComponent(playlistId)}`,{method:'DELETE'},true),
  addPlaylistTrack:(playlistId:string,trackId:string)=>request<Playlist>(`/v1/playlists/${encodeURIComponent(playlistId)}/tracks`,{method:'POST',body:JSON.stringify({track_id:trackId})},true),
  removePlaylistTrack:(playlistId:string,trackId:string)=>request<Playlist>(`/v1/playlists/${encodeURIComponent(playlistId)}/tracks/${encodeURIComponent(trackId)}`,{method:'DELETE'},true),
};
