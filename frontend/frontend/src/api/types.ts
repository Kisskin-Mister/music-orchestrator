export type ProviderId = 'local' | 'youtube_official' | 'soundcloud_official' | 'youtube_stream' | 'soundcloud_stream';
export type PlaybackType = 'local_stream' | 'local_cached_stream' | 'embed' | 'official_stream' | 'extractor_stream' | 'unavailable';
export type JobStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'blocked_by_policy';
export type Capabilities = { search:boolean; metadata:boolean; embed_playback:boolean; official_stream_url:boolean; raw_audio_stream:boolean; persistent_cache:boolean; offline_playback:boolean; public_deployment_safe:boolean };
export type Policy = { download_allowed:boolean; cache_allowed:boolean; requires_attribution:boolean; notes:string[] };
export type Provider = { id:ProviderId; name:string; kind:string; configured:boolean; enabled:boolean; risk_level:string; capabilities:Capabilities; policy:Policy };
export type Track = { id:string; provider_id:ProviderId; provider_track_id:string; title:string; artist?:string; album?:string; duration_seconds?:number; artwork_url?:string; attribution?:string; official?:boolean; downloaded:boolean; download_media_url?:string; capabilities:Capabilities; policy:Policy };
export type SearchResponse = { query:string; limit:number; offset:number; total:number; items:Track[] };
export type Playback = { track_id:string; provider_id:ProviderId; playback_type:PlaybackType; stream_url?:string|null; embed_url?:string|null; expires_in_seconds?:number|null; attribution?:string; capabilities:Capabilities; policy:Policy };
export type Job = { id:string; type:string; status:JobStatus; track_id?:string; payload:Record<string,unknown>&{track?:Track}; result?:{media_url?:string; track?:Track}; error?:string; created_at:string; updated_at:string };
export type Favorite = { track_id:string; track?:Track; created_at:string };
export type PlaylistTrack = { id:string; track_id:string; track?:Track; position:number; added_at:string };
export type Playlist = { id:string; name:string; description?:string; cover_url?:string; track_count:number; duration_seconds:number; tracks:PlaylistTrack[]; created_at:string; updated_at:string };
export type APIError = { error:{ code:string; message:string; details?:unknown } };
export type User = { id:string; username:string; role:'user'|'admin'; created_at:string; updated_at:string };
export type SessionInfo = { authenticated:boolean; user_id?:string; username?:string; auth_type?:string; role?:'user'|'admin'; setup_required:boolean; totp_required:boolean; totp_enabled:boolean; login_enabled:boolean };
export type LoginResult = SessionInfo;

/** Server settings an admin can review and change from the UI.
 *  Secrets are never sent back — only whether they are set. Fields under
 *  `read_only` are fixed at boot or deliberately env-only (see settings.go). */
export type ServerSettings = {
  enable_risky_extractors: boolean;
  extractor_timeout_seconds: number;
  download_timeout_seconds: number;
  session_ttl_hours: number;
  secure_cookies: boolean;
  public_media_base_url: string;
  cors_origins: string[];
  youtube_api_key_set: boolean;
  soundcloud_client_id_set: boolean;
  navidrome_token_set: boolean;
  navidrome_base_url: string;
  navidrome_username: string;
  read_only: {
    addr: string;
    environment: string;
    store_path: string;
    media_root: string;
    web_root: string;
    yt_dlp_binary: string;
    reason: string;
  };
};

export type ServerSettingsPatch = Partial<{
  enable_risky_extractors: boolean;
  extractor_timeout_seconds: number;
  download_timeout_seconds: number;
  session_ttl_hours: number;
  secure_cookies: boolean;
  public_media_base_url: string;
  cors_origins: string[];
  youtube_api_key: string;
  soundcloud_client_id: string;
  navidrome_base_url: string;
  navidrome_username: string;
  navidrome_token: string;
}>;

/** Итог сканирования папки с музыкой (см. import.go). */
export type ImportResult = {
  scanned: number;
  imported: number;
  duplicate: number;
  skipped?: { path: string; reason: string }[];
  elapsed: string;
  truncated?: boolean;
  counts?: Record<string, number>;
};

/** Страница медиатеки: треки плюс счётчики для фасетов (см. library в main.go). */
export type LibraryPage = {
  tracks: Track[];
  total: number;
  offset: number;
  sources: Record<string, number>;
};
export type ArtistSummary = { name: string; tracks: number; albums: number };
export type AlbumSummary = { name: string; artist: string; tracks: number; cover?: string };
