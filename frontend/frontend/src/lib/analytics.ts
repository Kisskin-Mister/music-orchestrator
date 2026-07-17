type EventName='search_submitted'|'provider_filter_changed'|'playback_requested'|'playback_started'|'playback_paused'|'playback_failed'|'playback_unavailable'|'stream_url_expired'|'favorite_added'|'favorite_removed'|'playlist_created'|'playlist_track_added'|'download_requested'|'download_blocked_by_policy'|'download_succeeded'|'download_failed'|'media_file_played'|'api_health_failed'|'api_key_invalid'|'provider_disabled_seen'|'risky_mode_notice_seen';
type SafeProperties={provider_ids?:string[];result_count?:number;latency_bucket?:string;error_code?:string;provider_id?:string;playback_type?:string;duration_bucket?:string};
export interface AnalyticsAdapter{track(name:EventName,properties?:SafeProperties):void}
const devAdapter:AnalyticsAdapter={track:(name,properties)=>{if(import.meta.env.DEV)console.info('[analytics]',name,properties??{})}};
export const analytics:AnalyticsAdapter=import.meta.env.VITE_ANALYTICS_ENABLED==='true'?devAdapter:{track:()=>{}};
