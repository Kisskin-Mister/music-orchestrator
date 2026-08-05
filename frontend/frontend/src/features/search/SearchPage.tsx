import { CSSProperties, FormEvent, MouseEvent, ReactNode, TouchEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Check, Cloud, Download, Folder, Heart, Library, ListMusic, Loader2, LogOut, MoreHorizontal, Music2, Pause, Pencil, Play, Plus, Repeat, Repeat1, Search, SearchX, Settings, ShieldCheck, Shuffle, SkipBack, SkipForward, Trash2, X, Youtube } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { useAddFavorite, useAddPlaylistTrack, useCreateDownload, useCreatePlaylist, useDeleteDownload, useDeleteFavorite, useDeletePlaylist, useDownloads, useFavorites, usePlayback, usePlaylists, useProviders, useRemovePlaylistTrack, useSearch, useSession, useUpdateAccount, useCreateUser, useDeleteUser, useUpdatePlaylist, useUpdateUser, useUsers } from '@/api/queries';
import { OVERREPORTED_DURATION_RATIO, usePlayerStore } from '@/store/player';
import { analytics } from '@/lib/analytics';
import { artworkURL, getBackendBaseURL, getDefaultBackendBaseURL, setBackendBaseURL } from '@/api/client';
import { ACCENT_PRESETS, applyAccent, getStoredAccent } from '@/lib/theme';
import { useSettings, useUpdateSettings } from '@/api/queries';
import type { Job, Playlist, Provider, ProviderId, ServerSettingsPatch, Track, User } from '@/api/types';

const DEFAULT_PROVIDERS: ProviderId[] = ['youtube_stream', 'soundcloud_stream'];
const PAGE_SIZE = 20;
const NAV = [
  { id: 'library', label: 'Медиатека', icon: Library },
  { id: 'search', label: 'Поиск', icon: Search },
  { id: 'playlists', label: 'Плейлисты', icon: ListMusic },
  { id: 'downloads', label: 'Загрузки', icon: Download },
  { id: 'settings', label: 'Настройки', icon: Settings },
];

type TrackSurfaceProps = {
  playlists: Playlist[];
  favoriteIDs: Set<string>;
  downloadedByTrack: Map<string | undefined, Job>;
  onLike: (track: Track) => void;
  onPlay: (track: Track, list: Track[], index: number) => void;
};

/* Brand colours make the source readable at a glance once the filter chips
   lost their text labels — same palette as the Flutter client. */
function sourceColor(id: string) {
  if (id.includes('youtube')) return '#FF0033';
  if (id.includes('soundcloud')) return '#FF7700';
  if (id.includes('local')) return '#8BC34A';
  return undefined;
}
function SourceIcon({ id, className = '', colored = false }: { id: string; className?: string; colored?: boolean }) {
  const style = colored ? { color: sourceColor(id) } : undefined;
  if (id.includes('youtube')) return <Youtube className={className} style={style} aria-label="YouTube" />;
  if (id.includes('soundcloud')) return <Cloud className={className} style={style} aria-label="SoundCloud" />;
  if (id.includes('local')) return <Folder className={className} style={style} aria-label="Local" />;
  return <Music2 className={className} style={style} aria-label="Source" />;
}
function sourceName(id: string) { if (id.includes('youtube')) return 'YouTube'; if (id.includes('soundcloud')) return 'SoundCloud'; if (id === 'local') return 'Local'; return id; }
// Deterministic two-tone gradient per track id, so tracks without artwork get a distinct,
// intentional-looking cover instead of a flat grey box — the same trick Spotify/SoundCloud use.
function artHues(seed: string) { let h = 0; for (let i = 0; i < seed.length; i += 1) h = (h * 31 + seed.charCodeAt(i)) >>> 0; return { a: h % 360, b: (h % 360 + 46) % 360 }; }
function TrackArt({ track, className = '' }: { track: Track; className?: string }) {
  const src = artworkURL(track.artwork_url);
  const { a, b } = artHues(track.id);
  const [failedSrc, setFailedSrc] = useState<string | null>(null);
  const failed = Boolean(src && failedSrc === src);
  return <span aria-hidden="true" className={`relative block overflow-hidden ${className}`} style={{ background: `linear-gradient(135deg, hsl(${a} 58% 26%), hsl(${b} 46% 15%))` }}>
    {src && !failed && <img alt="" src={src} loading="lazy" onError={() => setFailedSrc(src)} className="absolute inset-0 block h-full w-full object-cover" />}
  </span>;
}
function providerTone(id: string, risky?: boolean) { if (id === 'local') return 'border-lime-300/25 bg-lime-300/10 text-lime-100'; if (risky) return 'border-amber-300/25 bg-amber-300/10 text-amber-100'; return 'border-sky-300/20 bg-sky-300/10 text-sky-100'; }
function providerName(provider?: Provider) { return provider ? sourceName(provider.id) : 'Источник'; }
function getMediaURL(job?: Job | null) { return typeof job?.result?.media_url === 'string' ? job.result.media_url : null; }
function trackFromJob(job: Job): Track | null { return job.result?.track ?? job.payload?.track ?? null; }
function formatDuration(seconds?: number) { if (!seconds) return '—'; const m = Math.floor(seconds / 60); const s = Math.floor(seconds % 60).toString().padStart(2, '0'); return `${m}:${s}`; }
function filterTracks(tracks: Track[], filter: string) { const q = filter.toLowerCase().trim(); return q ? tracks.filter((t) => `${t.title} ${t.artist ?? ''}`.toLowerCase().includes(q)) : tracks; }

export function SearchPage({ onLogout, localMode = false, onLeaveLocalMode }: { onLogout?: () => void; localMode?: boolean; onLeaveLocalMode?: () => void } = {}) {
  const [draft, setDraft] = useState('');
  const [query, setQuery] = useState('');
  const [searchLimit, setSearchLimit] = useState(PAGE_SIZE);
  const [libraryFilter, setLibraryFilter] = useState('');
  const [downloadsFilter, setDownloadsFilter] = useState('');
  const [activeView, setActiveView] = useState('library');
  // Lives here rather than inside PlaylistsView so the floating button can sit
  // outside the animated view wrapper — a transform there would make it the
  // containing block and the button would slide with the page on every load.
  const [creatingPlaylist, setCreatingPlaylist] = useState(false);
  const [selectedProviders, setSelectedProviders] = useState<ProviderId[]>(() => {
    try {
      const raw = localStorage.getItem('mo_enabled_search_providers');
      const parsed = raw ? JSON.parse(raw) : null;
      return Array.isArray(parsed) && parsed.length ? parsed as ProviderId[] : DEFAULT_PROVIDERS;
    } catch {
      return DEFAULT_PROVIDERS;
    }
  });
  const providers = useProviders();
  const result = useSearch(query, selectedProviders, searchLimit, 0);
  const downloads = useDownloads();
  const favorites = useFavorites();
  const playlists = usePlaylists();
  const addFavorite = useAddFavorite();
  const deleteFavorite = useDeleteFavorite();
  const toggleTrack = usePlayerStore((s) => s.toggleTrack);

  useEffect(() => {
    localStorage.setItem('mo_enabled_search_providers', JSON.stringify(selectedProviders));
  }, [selectedProviders]);

  // Dynamic search removed — search only triggers on explicit form submit.

  useEffect(() => {
    const youtube = providers.data?.find((p) => p.id === 'youtube_stream' && p.enabled);
    if (youtube && selectedProviders.length === 0) setSelectedProviders(['youtube_stream']);
  }, [providers.data, selectedProviders.length]);

  const favoriteIDs = useMemo(() => new Set(favorites.data?.map((f) => f.track_id) ?? []), [favorites.data]);
  const downloadedJobs = downloads.data ?? [];
  const downloadedByTrack = useMemo(() => new Map(downloadedJobs.map((j) => [j.track_id, j])), [downloadedJobs]);
  const favoriteTracks = favorites.data?.map((f) => f.track).filter(Boolean) as Track[] | undefined;
  const libraryTracks = useMemo(() => {
    const byID = new Map<string, Track>();
    downloadedJobs.forEach((job) => { const track = trackFromJob(job); if (track) byID.set(track.id, { ...track, downloaded: true, download_media_url: getMediaURL(job) ?? track.download_media_url }); });
    favoriteTracks?.forEach((track) => byID.set(track.id, byID.get(track.id) ?? track));
    return [...byID.values()];
  }, [downloadedJobs, favoriteTracks]);
  const filteredLibrary = useMemo(() => filterTracks(libraryTracks, libraryFilter), [libraryTracks, libraryFilter]);
  const riskyEnabled = providers.data?.some((p) => p.risk_level === 'risky' && p.enabled) ?? false;
  const shared: TrackSurfaceProps = {
    playlists: playlists.data ?? [], favoriteIDs, downloadedByTrack,
    onLike: (track) => favoriteIDs.has(track.id) ? deleteFavorite.mutate(track.id) : addFavorite.mutate(track),
    onPlay: (track, list, index) => toggleTrack(track, list, index),
  };
  const submit = (e: FormEvent) => { e.preventDefault(); const next = draft.trim(); setSearchLimit(PAGE_SIZE); setQuery(next); setActiveView('search'); if (next) analytics.track('search_submitted', { provider_ids: selectedProviders }); };
  const clearSearch = () => { setDraft(''); setQuery(''); setSearchLimit(PAGE_SIZE); };
  // Stable identity: the infinite-scroll observer keys off this, and a fresh
  // function each render would tear down and refire the observer in a loop.
  const loadMore = useCallback(() => setSearchLimit((n) => n + PAGE_SIZE), []);
  const toggleProvider = (id: ProviderId) => setSelectedProviders((current) => current.includes(id) ? current.filter((p) => p !== id) : [...current, id]);

  return <main className="mx-auto grid min-h-screen max-w-[1600px] gap-4 px-3 py-4 pb-[calc(10rem+env(safe-area-inset-bottom,0px))] lg:grid-cols-[76px_minmax(0,1fr)] lg:px-5 lg:pb-24 xl:grid-cols-[240px_minmax(0,1fr)] xl:gap-8 xl:px-8">
    <aside className="fixed inset-x-3 bottom-[calc(0.5rem+env(safe-area-inset-bottom,0px))] z-30 rounded-2xl border border-white/10 bg-surface-2/95 p-2 shadow-[0_16px_48px_rgba(0,0,0,.4)] backdrop-blur lg:sticky lg:top-5 lg:inset-x-auto lg:bottom-auto lg:block lg:h-fit lg:min-h-[calc(100vh-2.5rem-5.5rem)] lg:p-3 xl:sticky xl:top-5 xl:p-4">
      <div className="hidden items-center gap-2.5 px-1.5 pb-5 lg:flex">
        <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-lime-300 text-black"><Music2 size={17} /></span>
        <span className="hidden truncate font-display text-[13px] font-bold leading-tight tracking-[-.02em] xl:inline">Orchestrator</span>
      </div>
      <nav className="grid grid-cols-5 gap-1 lg:grid-cols-1 lg:gap-1">{NAV.map(({ id, label, icon: Icon }) => <button key={id} type="button" onClick={() => setActiveView(id)} className={`flex flex-col items-center justify-center gap-1 rounded-control px-2 py-2 transition active:scale-[.96] lg:py-3 xl:flex-row xl:justify-start xl:gap-3 xl:px-3 xl:py-2.5 ${activeView === id ? 'bg-lime-300 text-black' : 'text-[#9aa0ad] hover:bg-white/8 hover:text-white'}`} aria-label={label} aria-current={activeView === id ? 'page' : undefined} title={label}><Icon size={20} className="shrink-0" /><span className="text-[11px] leading-none lg:hidden xl:inline xl:text-sm xl:font-medium xl:leading-none">{label}</span></button>)}</nav>
    </aside>
    <section className="min-w-0"><div key={activeView} className="view-enter">
      {activeView === 'library' && <LibraryView tracks={filteredLibrary} filter={libraryFilter} setFilter={setLibraryFilter} {...shared} />}
      {activeView === 'search' && <SearchView draft={draft} setDraft={setDraft} submit={submit} clearSearch={clearSearch} query={query} resultQuery={result.data?.query ?? ''} providers={providers.data ?? []} selectedProviders={selectedProviders} toggleProvider={toggleProvider} isLoading={result.isLoading} isFetching={result.isFetching} isError={result.isError} total={result.data?.total ?? 0} limit={searchLimit} more={loadMore} tracks={result.data?.items ?? []} {...shared} />}
      {activeView === 'playlists' && <PlaylistsView libraryTracks={libraryTracks} creating={creatingPlaylist} setCreating={setCreatingPlaylist} {...shared} />}
      {activeView === 'downloads' && <DownloadsView jobs={downloadedJobs.filter((j) => j.track_id)} filter={downloadsFilter} setFilter={setDownloadsFilter} {...shared} />}
      {activeView === 'settings' && <SettingsView providers={providers.data ?? []} riskyEnabled={riskyEnabled} selectedProviders={selectedProviders} toggleProvider={toggleProvider} onLogout={onLogout} localMode={localMode} onLeaveLocalMode={onLeaveLocalMode} />}
    </div></section>
    {activeView === 'playlists' && !creatingPlaylist && <button type="button" onClick={() => setCreatingPlaylist(true)} aria-label="Создать плейлист" title="Создать плейлист" className="fixed bottom-[calc(9rem+env(safe-area-inset-bottom,0px))] right-5 z-40 grid h-14 w-14 place-items-center rounded-full bg-lime-300 text-black shadow-[0_10px_30px_rgba(0,0,0,.45)] transition hover:scale-105 active:scale-[.96] lg:bottom-28 lg:right-8"><Plus size={26} /></button>}
    <PlayerBar favoriteIDs={favoriteIDs} downloadedByTrack={downloadedByTrack} onLike={shared.onLike} />
  </main>;
}

/* Eyebrow → title → subtitle, with the same 6/8/16px rhythm every Flutter
   screen opens with (see mobile/lib/screens/*.dart). `subtitle` is optional
   because Search and Playlists go straight from the title to their controls. */
function SectionHeader({ eyebrow, title, subtitle, children }: { eyebrow: string; title: string; subtitle?: string; children?: ReactNode }) { return <header className="mb-4"><p className="t-eyebrow m-0 text-lime-300">{eyebrow}</p><h1 className="t-headline-lg m-0 mt-1.5 text-[40px] text-balance">{title}</h1>{subtitle && <p className="t-body-sm m-0 mt-2 max-w-2xl text-pretty">{subtitle}</p>}{children && <div className="mt-4">{children}</div>}</header>; }
function InlineFilter({ value, onChange, placeholder }: { value: string; onChange: (value: string) => void; placeholder: string }) { return <div className="flex h-12 items-center gap-3 rounded-control border border-white/15 bg-surface px-4 transition focus-within:border-lime-300/60"><Search size={18} className="text-muted" /><input value={value} onChange={(e) => onChange(e.target.value)} className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-subtle" placeholder={placeholder} /></div>; }
function SpinnerLine({ label }: { label: string }) { return <div className="flex items-center gap-2 rounded-xl bg-surface-2 p-4 text-[#9aa0ad]"><Loader2 className="animate-spin text-lime-300" size={18} /> {label}</div>; }
/* Skeletons beat a spinner where the result shape is known: the layout stops
   jumping when data lands, and the wait reads as "this list is filling in". */
function SkeletonBar({ className = '' }: { className?: string }) { return <span className={`block animate-pulse rounded bg-white/[0.07] ${className}`} />; }
function TrackRowSkeleton() { return <div className="flex items-center gap-3 px-1 py-2"><SkeletonBar className="h-13 w-13 shrink-0 rounded-control" /><div className="min-w-0 flex-1"><SkeletonBar className="h-4 w-[62%]" /><SkeletonBar className="mt-2 h-3 w-[38%]" /></div></div>; }
function TrackListSkeleton({ rows = 6 }: { rows?: number }) { return <div aria-hidden="true">{Array.from({ length: rows }, (unused, i) => <TrackRowSkeleton key={i} />)}</div>; }
/* Flutter's empty screens are a bare centred column — 40px muted icon, title,
   hint — with no card or dashed frame around them. */
function EmptyState({ icon: Icon, title, hint }: { icon: LucideIcon; title: string; hint: string }) { return <div className="grid place-items-center px-8 py-14 text-center"><Icon size={40} className="text-muted" /><h2 className="t-title-lg m-0 mt-4 text-[22px]">{title}</h2><p className="t-body m-0 mt-1.5 max-w-sm text-pretty">{hint}</p></div>; }

function SearchView({ draft, setDraft, submit, clearSearch, query, resultQuery, providers, selectedProviders, toggleProvider, isLoading, isFetching, isError, total, limit, more, tracks, playlists, favoriteIDs, downloadedByTrack, onLike, onPlay }: { draft:string; setDraft:(v:string)=>void; submit:(e:FormEvent)=>void; clearSearch:()=>void; query:string; resultQuery:string; providers:Provider[]; selectedProviders:ProviderId[]; toggleProvider:(id:ProviderId)=>void; isLoading:boolean; isFetching:boolean; isError:boolean; total:number; limit:number; more:()=>void; tracks: Track[] } & TrackSurfaceProps) {
  const trimmedDraft = draft.trim();
  const waitingForDebounce = Boolean(trimmedDraft) && trimmedDraft !== query;
  /* `tracks` still holds the previous query's items while the next request is in
     flight, so matching on resultQuery alone let another query's results render
     under the loading skeleton. Anything still being typed or fetched counts as
     not-fresh, and the skeleton shows on its own. */
  const staleInFlight = isFetching && resultQuery !== query;
  const hasFreshResult = Boolean(query) && resultQuery === query && !waitingForDebounce && !staleInFlight;
  const visibleTracks = hasFreshResult ? tracks : [];
  const visibleTotal = hasFreshResult ? total : 0;
  const waitingForFreshResult = Boolean(query) && !hasFreshResult && !isError;
  const showInitialSpinner = Boolean(query) && (isLoading || isFetching || waitingForFreshResult) && !visibleTracks.length;
  const canShowEmpty = Boolean(query) && hasFreshResult && !waitingForDebounce && !isFetching && !isLoading && !isError && !visibleTracks.length;
  const canLoadMore = visibleTracks.length > 0 && visibleTracks.length < visibleTotal;
  /* An observed sentinel replaces the old "Ещё 20" button: the next page is
     requested as soon as the end of the list comes into view, one rootMargin
     screen early so the spinner is rarely visible at all. */
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel || !canLoadMore || isFetching) return;
    if (typeof IntersectionObserver === 'undefined') return;
    const observer = new IntersectionObserver((entries) => { if (entries.some((entry) => entry.isIntersecting)) more(); }, { rootMargin: '400px 0px' });
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [canLoadMore, isFetching, more]);
  return <>
    <SectionHeader eyebrow="Поиск" title="Что послушаем?">
      <form onSubmit={submit} className="flex h-13 items-center gap-3 rounded-control border border-white/15 bg-surface px-4 transition focus-within:border-lime-300/60">
        <Search size={19} className="text-muted" />
        <input value={draft} onChange={(e) => setDraft(e.target.value)} className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-subtle" placeholder="Название, исполнитель или альбом" />
        {draft && <button type="button" onClick={clearSearch} aria-label="Очистить поиск" className="rounded-control p-2 text-muted hover:bg-white/8"><X size={17} /></button>}
        <button type="submit" aria-label="Найти" className="rounded-control bg-lime-300 px-3 py-2 text-sm font-medium text-black disabled:opacity-50" disabled={!draft.trim()}>{isFetching ? <Loader2 className="animate-spin" size={16} /> : 'Найти'}</button>
      </form>
      <p className="t-eyebrow m-0 mt-3">Источники</p>
      <div className="mt-2 flex flex-wrap gap-2.5">{providers.filter((p) => p.enabled).map((provider) => { const selected = selectedProviders.includes(provider.id); return <button key={provider.id} type="button" onClick={() => toggleProvider(provider.id)} aria-pressed={selected} className={`grid h-11 w-14 place-items-center rounded-card border transition ${selected ? 'border-lime-300/60 bg-lime-300/[0.18] ring-1 ring-lime-300/40' : 'border-white/10 bg-surface-2 opacity-45 hover:opacity-80'}`} title={providerName(provider)} aria-label={providerName(provider)}><SourceIcon id={provider.id} colored className="h-[22px] w-[22px]" /></button>; })}</div>
    </SectionHeader>
    {!query && <EmptyState icon={Search} title="Найди свой трек" hint="Введи название песни, исполнителя или альбома — соберём результаты из подключённых источников." />}
    {(waitingForDebounce || showInitialSpinner) && <><span className="sr-only" role="status">Ищу треки…</span><TrackListSkeleton /></>}
    {query && isError && !waitingForDebounce && <section role="alert" className="rounded-xl border border-red-300/25 bg-red-300/[0.06] p-5"><h2 className="m-0 text-lg">Поиск не удался</h2><p className="mb-0 text-[#b9bec9]">Источник не ответил вовремя. Попробуй ещё раз или сократи запрос.</p></section>}
    {canShowEmpty && <EmptyState icon={SearchX} title={`По запросу «${query}» ничего нет`} hint="Попробуй написать короче или другими словами — или выбери другой источник выше." />}
    {Boolean(visibleTracks.length) && <><TrackList tracks={visibleTracks} playlists={playlists} favoriteIDs={favoriteIDs} downloadedByTrack={downloadedByTrack} onLike={onLike} onPlay={onPlay} />{canLoadMore && <div ref={sentinelRef} className="mt-3 min-h-[1px]">{isFetching && <SpinnerLine label="Подгружаю…" />}</div>}<p className="mt-3 text-xs text-[#777]">Показано {Math.min(limit, visibleTracks.length)} из {visibleTotal}</p></>}
  </>;
}

function TrackRow({ track, playlists, liked, cachedJob, onLike, onPlay, onRemoveFromPlaylist }: { track: Track; playlists: Playlist[]; liked: boolean; cachedJob?: Job; onLike: () => void; onPlay: () => void; onRemoveFromPlaylist?: () => void }) {
  const createDownload = useCreateDownload();
  const deleteDownload = useDeleteDownload();
  const createPlaylist = useCreatePlaylist();
  const addPlaylistTrack = useAddPlaylistTrack();
  const [job, setJob] = useState<Job | null>(cachedJob ?? null);
  const [menuOpen, setMenuOpen] = useState(false);
  const [playlistPicker, setPlaylistPicker] = useState(false);
  const [newPlaylistName, setNewPlaylistName] = useState('');
  useEffect(() => { if (cachedJob) setJob(cachedJob); }, [cachedJob]);
  const mediaURL = track.download_media_url ?? getMediaURL(job);
  const downloaded = track.downloaded || Boolean(mediaURL);
  const download = async () => { try { const next = await createDownload.mutateAsync({ trackId: track.id, format: 'mp3' }); setJob(next); setMenuOpen(false); analytics.track('download_requested', { provider_id: track.provider_id }); } catch (error) { setJob({ id: 'local-error', type: 'download', status: 'failed', payload: {}, error: error instanceof Error ? error.message : 'Download failed', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }); } };
  const remove = async () => { await deleteDownload.mutateAsync(track.id); setJob(null); setMenuOpen(false); };
  const addToPlaylist = async (playlistId: string) => { await addPlaylistTrack.mutateAsync({ playlistId, trackId: track.id }); setPlaylistPicker(false); setMenuOpen(false); };
  const createAndAdd = async () => { const name = newPlaylistName.trim(); if (!name) return; const playlist = await createPlaylist.mutateAsync({ name }); await addPlaylistTrack.mutateAsync({ playlistId: playlist.id, trackId: track.id }); setNewPlaylistName(''); setPlaylistPicker(false); setMenuOpen(false); };
  const currentId = usePlayerStore((state) => state.currentTrack?.id);
  const liveDuration = usePlayerStore((state) => state.currentTrack?.id === track.id ? state.currentTrack.duration_seconds : undefined);
  const playerStatus = usePlayerStore((state) => state.status);
  const playNext = usePlayerStore((state) => state.playNext);
  const addToQueue = usePlayerStore((state) => state.addToQueue);
  const isCurrent = currentId === track.id;
  return <article aria-current={isCurrent ? 'true' : undefined} className={`grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-control px-1 py-2 transition-colors ${isCurrent ? 'bg-lime-300/[0.08]' : 'hover:bg-white/[0.03]'}`}>
    <button type="button" onClick={onPlay} className="flex min-w-0 items-center gap-3 text-left outline-none ring-lime-300/70 focus-visible:ring-2">
      <span className="group relative grid h-13 w-13 shrink-0 place-items-center overflow-hidden rounded-control transition hover:ring-2"><TrackArt track={track} className="h-full w-full transition group-hover:scale-105" /><span className={`absolute inset-0 grid place-items-center text-white transition ${isCurrent ? 'bg-black/45 opacity-100' : 'bg-black/0 opacity-0 group-hover:bg-black/35 group-hover:opacity-100'}`}>{isCurrent ? <span className={`eq ${playerStatus === 'playing' ? '' : 'eq--paused'}`} aria-label="Сейчас играет"><span /><span /><span /></span> : <Play size={18} />}</span></span>
      <span className="min-w-0"><span className="flex min-w-0 items-center gap-2"><strong className="truncate font-semibold">{track.title}</strong>{track.official && <ShieldCheck size={15} className="shrink-0 text-sky-300" aria-label="Official Topic" />}{downloaded && <Folder size={15} className="shrink-0 text-lime-300" aria-label="Локально" />}</span><span className="t-body-sm m-0 mt-[3px] flex min-w-0 items-center gap-[5px]"><SourceIcon id={track.provider_id} className="h-[15px] w-[15px] shrink-0" /><span className="truncate">{track.artist || 'Исполнитель не указан'}</span><span className="shrink-0 font-mono text-xs tabular-nums">· {formatDuration(liveDuration ?? track.duration_seconds)}</span></span>{job?.error && <span className="m-0 mt-1 block text-xs text-red-200">{job.error}</span>}</span>
    </button>
    <div className="relative flex gap-1">
      <button aria-label={liked ? 'Убрать лайк' : 'Поставить лайк'} onClick={onLike} className={`grid h-11 w-11 place-items-center rounded-full transition hover:bg-white/8 ${liked ? 'text-danger' : 'text-muted'}`}><Heart size={20} fill={liked ? 'currentColor' : 'none'} /></button>
      <button aria-label="Действия" onClick={() => setMenuOpen((v) => !v)} aria-haspopup="menu" aria-expanded={menuOpen} className="grid h-11 w-11 place-items-center rounded-full text-muted transition hover:bg-white/8"><MoreHorizontal size={20} /></button>
      {menuOpen && <><button type="button" aria-label="Закрыть меню" tabIndex={-1} onClick={() => setMenuOpen(false)} className="fixed inset-0 z-30 cursor-default" /><div role="menu" onKeyDown={(event) => { if (event.key === 'Escape') setMenuOpen(false); }} className="absolute right-0 top-11 z-40 w-64 overflow-hidden rounded-xl border border-white/10 bg-surface-3 p-1 shadow-2xl shadow-black/50">
        {/* Order mirrors the mobile sheet: queue actions, then collection, then
            downloads, with anything destructive last and coloured — so a
            removal never sits where a routine action was a moment ago. */}
        <button role="menuitem" onClick={() => { playNext(track); setMenuOpen(false); }} className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm text-[#a6abb7] hover:bg-white/8"><SkipForward size={16} /> Слушать следующим</button>
        <button role="menuitem" onClick={() => { addToQueue(track); setMenuOpen(false); }} className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm text-[#a6abb7] hover:bg-white/8"><ListMusic size={16} /> В конец очереди</button>
        <button role="menuitem" onClick={() => { onLike(); setMenuOpen(false); }} className={`flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm hover:bg-white/8 ${liked ? 'text-red-300' : 'text-[#a6abb7]'}`}><Heart size={16} fill={liked ? 'currentColor' : 'none'} /> {liked ? 'Убрать из медиатеки' : 'В медиатеку'}</button>
        <button role="menuitem" onClick={() => setPlaylistPicker((v) => !v)} aria-expanded={playlistPicker} className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm text-[#a6abb7] hover:bg-white/8"><Plus size={16} /> Добавить в плейлист</button>
        {onRemoveFromPlaylist && <button role="menuitem" onClick={() => { onRemoveFromPlaylist(); setMenuOpen(false); }} className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm text-red-200 hover:bg-red-300/10"><X size={16} /> Убрать из плейлиста</button>}
        {track.policy.download_allowed && <><div className="my-1 border-t border-white/10" />
        <button role="menuitem" onClick={download} disabled={createDownload.isPending || downloaded} className="flex w-full items-start gap-2 rounded-lg px-3 py-2 text-left text-sm hover:bg-white/8 disabled:opacity-50">{createDownload.isPending ? <Loader2 className="mt-0.5 animate-spin" size={16} /> : <Download size={16} className="mt-0.5" />}<span><span className="block">{downloaded ? 'Уже на сервере' : 'Скачать на сервер'}</span><small className="block text-[#8c919e]">{downloaded ? 'Не зависит от YouTube' : 'Чтобы не зависеть от YouTube'}</small></span></button></>}
        {downloaded && <button role="menuitem" onClick={remove} disabled={deleteDownload.isPending} className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm text-red-200 hover:bg-red-300/10"><Trash2 size={16} /> Удалить с сервера</button>}
        {playlistPicker && <div className="mt-1 border-t border-white/10 p-2">
          <div className="grid gap-1">{playlists.map((playlist) => <button key={playlist.id} onClick={() => addToPlaylist(playlist.id)} className="flex w-full items-center justify-between rounded-xl px-2 py-2 text-left text-sm hover:bg-white/8"><span className="truncate">{playlist.name}</span><small className="text-[#8c919e]">{playlist.track_count}</small></button>)}</div>
          <p className="mt-2 mb-1 px-1 font-mono text-[10px] uppercase tracking-[.12em] text-[#8c919e]">Новый плейлист</p>
          <div className="flex gap-2"><input value={newPlaylistName} onChange={(e) => setNewPlaylistName(e.target.value)} placeholder="Название" aria-label="Название нового плейлиста" className="min-w-0 flex-1 rounded-xl border border-white/10 bg-black/25 px-3 py-2 text-sm outline-none" /><button onClick={createAndAdd} disabled={!newPlaylistName.trim() || createPlaylist.isPending || addPlaylistTrack.isPending} className="shrink-0 rounded-xl bg-lime-300 px-3 py-2 text-sm font-medium text-black disabled:opacity-50">{createPlaylist.isPending || addPlaylistTrack.isPending ? <Loader2 className="animate-spin" size={15} /> : 'Создать'}</button></div>
        </div>}
      </div></>}
    </div>
  </article>;
}

/* Rows sit straight on the page background with no card or separators, the way
   Flutter's SliverList does — the rounded current-track highlight only reads as
   a pill when nothing is drawn between rows. */
function TrackList({ tracks, playlists, favoriteIDs, downloadedByTrack, onLike, onPlay }: { tracks: Track[] } & TrackSurfaceProps) { return <div>{tracks.map((track, index) => <div key={track.id} className="stagger-in" style={{ '--stagger-index': Math.min(index, 10) } as CSSProperties}><TrackRow track={track} playlists={playlists} liked={favoriteIDs.has(track.id)} cachedJob={downloadedByTrack.get(track.id)} onLike={() => onLike(track)} onPlay={() => onPlay(track, tracks, index)} /></div>)}</div>; }
function CoverStrip({ eyebrow, tracks, onPlay }: { eyebrow: string; tracks: Track[]; onPlay: (track: Track, list: Track[], index: number) => void }) {
  if (!tracks.length) return null;
  return <section className="mb-4">
    <p className="t-eyebrow m-0 mb-2.5 pl-1">{eyebrow}</p>
    <div className="no-scrollbar -mx-1 flex gap-[14px] overflow-x-auto px-1 pb-1">
      {tracks.slice(0, 12).map((track, index) => <button key={track.id} type="button" onClick={() => onPlay(track, tracks, index)} className="stagger-in group w-[140px] shrink-0 text-left" style={{ '--stagger-index': index } as CSSProperties}>
        <span className="relative block h-[140px] w-[140px] overflow-hidden rounded-card shadow-[0_14px_34px_rgba(0,0,0,.4)] transition group-hover:-translate-y-1">
          <TrackArt track={track} className="h-full w-full transition group-hover:scale-105" />
          <span className="absolute inset-0 grid place-items-center bg-black/0 opacity-0 transition group-hover:bg-black/35 group-hover:opacity-100"><span className="grid h-11 w-11 place-items-center rounded-full bg-lime-300 text-black"><Play size={18} /></span></span>
        </span>
        <strong className="mt-2 block truncate text-[13px] font-semibold">{track.title}</strong>
        <span className="mt-0.5 flex items-center gap-1 text-xs text-muted"><SourceIcon id={track.provider_id} className="h-[13px] w-[13px] shrink-0" /><span className="truncate">{track.artist || 'Исполнитель не указан'}</span></span>
      </button>)}
    </div>
  </section>;
}
function LibraryView({ tracks, filter, setFilter, playlists, favoriteIDs, downloadedByTrack, onLike, onPlay }: { tracks: Track[]; filter:string; setFilter:(value:string)=>void } & TrackSurfaceProps) { return <><SectionHeader eyebrow="Коллекция" title="Медиатека" subtitle="Всё, что ты лайкнул или скачал, — в одном месте."><InlineFilter value={filter} onChange={setFilter} placeholder="Искать в медиатеке" /></SectionHeader>{tracks.length ? <><CoverStrip eyebrow="Слушай снова" tracks={tracks} onPlay={onPlay} /><TrackList tracks={tracks} playlists={playlists} favoriteIDs={favoriteIDs} downloadedByTrack={downloadedByTrack} onLike={onLike} onPlay={onPlay} /></> : <EmptyState icon={Heart} title="Пока пусто" hint="Лайкни трек или скачай его — и он появится здесь." />}</>; }
function DownloadsView({ jobs, filter, setFilter, playlists, favoriteIDs, downloadedByTrack, onLike, onPlay }: { jobs: Job[]; filter:string; setFilter:(value:string)=>void } & TrackSurfaceProps) { const tracks = filterTracks(jobs.map((job) => { const track = trackFromJob(job); return track ? { ...track, downloaded: true, download_media_url: getMediaURL(job) ?? track.download_media_url } : null; }).filter(Boolean) as Track[], filter); return <><SectionHeader eyebrow="Offline" title="Загрузки" subtitle="Лежат на сервере — нужен доступ к нему, чтобы слушать."><InlineFilter value={filter} onChange={setFilter} placeholder="Искать в загрузках" /></SectionHeader>{tracks.length ? <TrackList tracks={tracks} playlists={playlists} favoriteIDs={favoriteIDs} downloadedByTrack={downloadedByTrack} onLike={onLike} onPlay={onPlay} /> : <EmptyState icon={Download} title="На сервере ничего нет" hint="Скачай трек на сервер, чтобы не зависеть от YouTube и SoundCloud." />}</>; }

function PlaylistsView({ playlists, libraryTracks, favoriteIDs, downloadedByTrack, onLike, onPlay, creating, setCreating }: TrackSurfaceProps & { libraryTracks: Track[]; creating: boolean; setCreating: (v: boolean) => void }) {
  const createPlaylist = useCreatePlaylist();
  const addPlaylistTrack = useAddPlaylistTrack();
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [selectedTrackIDs, setSelectedTrackIDs] = useState<Set<string>>(new Set());
  const selectedTracks = libraryTracks.filter((track) => selectedTrackIDs.has(track.id));
  const toggleSelectedTrack = (trackID: string) => setSelectedTrackIDs((current) => { const next = new Set(current); if (next.has(trackID)) next.delete(trackID); else next.add(trackID); return next; });
  const resetCreateForm = () => { setName(''); setDescription(''); setSelectedTrackIDs(new Set()); setCreating(false); };
  const create = async (event: FormEvent) => {
    event.preventDefault();
    const next = name.trim();
    if (!next || createPlaylist.isPending || addPlaylistTrack.isPending) return;
    const playlist = await createPlaylist.mutateAsync({ name: next, description: description.trim() || undefined });
    for (const track of selectedTracks) await addPlaylistTrack.mutateAsync({ playlistId: playlist.id, trackId: track.id });
    resetCreateForm();
  };
  return <>
    <SectionHeader eyebrow="Коллекции" title="Плейлисты" />
    {creating && <form onSubmit={create} className="mb-8 overflow-hidden rounded-2xl border border-white/10 bg-surface">
      <div className="border-b border-white/8 p-5">
        <div className="flex items-start justify-between gap-3"><div><p className="m-0 font-mono text-[11px] uppercase tracking-[.12em] text-lime-300">Новый плейлист</p><h2 className="m-0 mt-1 font-display text-2xl font-bold tracking-[-.01em]">Создать подборку</h2><p className="m-0 mt-1 text-sm text-[#a6abb7]">Сначала опиши вайб, потом выбери треки из медиатеки.</p></div><button type="button" onClick={resetCreateForm} aria-label="Закрыть создание плейлиста" className="rounded-lg p-2 text-[#a6abb7] hover:bg-white/8"><X size={19} /></button></div>
        <div className="mt-5 grid gap-3 md:grid-cols-2">
          <label className="grid gap-2 text-sm text-[#c6cad3]">Название плейлиста<input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Например: Ночной вайб" className="rounded-xl border border-white/10 bg-surface-2 px-4 py-3 text-white outline-none placeholder:text-[#626875]" /></label>
          <label className="grid gap-2 text-sm text-[#c6cad3]">Описание плейлиста<input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Коротко: настроение, жанр, для чего" className="rounded-xl border border-white/10 bg-surface-2 px-4 py-3 text-white outline-none placeholder:text-[#626875]" /></label>
        </div>
      </div>
      <div className="grid gap-4 p-5 lg:grid-cols-[minmax(0,1fr)_260px]">
        <section>
          <div className="mb-3 flex items-center justify-between gap-3"><div><h3 className="m-0 text-base font-semibold">Добавить треки из медиатеки</h3><p className="m-0 mt-1 text-xs text-[#8c919e]">Доступны лайкнутые и скачанные треки.</p></div><span className="rounded-full border border-white/10 px-3 py-1 text-xs text-[#a6abb7]">{selectedTrackIDs.size} выбрано</span></div>
          {libraryTracks.length ? <div className="grid max-h-80 gap-2 overflow-y-auto overscroll-contain pr-1">{libraryTracks.map((track) => {
            const checked = selectedTrackIDs.has(track.id);
            return <label key={track.id} className={`grid cursor-pointer grid-cols-[auto_42px_minmax(0,1fr)] items-center gap-3 rounded-xl border px-3 py-2 transition ${checked ? 'border-lime-300/40 bg-lime-300/[0.10]' : 'border-white/10 bg-surface-2 hover:bg-white/8'}`}>
              <input type="checkbox" checked={checked} onChange={() => toggleSelectedTrack(track.id)} aria-label={`Добавить ${track.title}`} className="h-5 w-5 accent-lime-300" />
              <span className="grid h-10 w-10 shrink-0 place-items-center overflow-hidden rounded-lg"><TrackArt track={track} className="h-full w-full" /></span>
              <span className="min-w-0"><strong className="block truncate text-sm">{track.title}</strong><small className="block truncate text-[#8c919e]">{track.artist ?? 'Исполнитель не указан'}</small></span>
            </label>;
          })}</div> : <div className="rounded-xl border border-white/10 bg-surface-2 p-5 text-sm text-[#a6abb7]">Медиатека пустая. Лайкни или скачай треки — потом их можно будет добавить сюда при создании.</div>}
        </section>
        <aside className="rounded-xl border border-white/10 bg-surface-2 p-4">
          <h3 className="m-0 text-base font-semibold">Превью</h3>
          <p className="m-0 mt-1 text-sm text-[#8c919e]">{name.trim() || 'Без названия'}</p>
          <p className="m-0 mt-3 text-xs text-[#8c919e]">{description.trim() || 'Описание можно оставить пустым.'}</p>
          <div className="mt-4 grid gap-2">{selectedTracks.slice(0, 4).map((track) => <div key={track.id} className="truncate rounded-lg bg-surface-3 px-3 py-2 text-sm">{track.title}</div>)}{selectedTracks.length > 4 && <div className="text-xs text-[#8c919e]">+ ещё {selectedTracks.length - 4}</div>}</div>
          <button type="submit" disabled={!name.trim() || createPlaylist.isPending || addPlaylistTrack.isPending} className="mt-5 w-full rounded-xl bg-lime-300 px-4 py-3 font-medium text-black disabled:opacity-50">{createPlaylist.isPending || addPlaylistTrack.isPending ? 'Создаю…' : 'Создать'}</button>
        </aside>
      </div>
    </form>}
    {playlists.length ? <div className="grid gap-4">{playlists.map((playlist) => <PlaylistCard key={playlist.id} playlist={playlist} playlists={playlists} favoriteIDs={favoriteIDs} downloadedByTrack={downloadedByTrack} onLike={onLike} onPlay={onPlay} />)}</div> : !creating && <EmptyState icon={ListMusic} title="Плейлистов пока нет" hint="Нажми «+» в правом нижнем углу, задай название и выбери треки из медиатеки." />}
    {/* Creating a playlist is the one primary action here, so it gets a fixed
        corner target instead of a button that scrolls away with the header.
        Sits above the player dock so it never covers playback controls. */}

  </>;
}

function PlaylistCard({ playlist, playlists, favoriteIDs, downloadedByTrack, onLike, onPlay }: { playlist: Playlist } & TrackSurfaceProps) {
  void playlists; void downloadedByTrack;
  const updatePlaylist = useUpdatePlaylist();
  const deletePlaylist = useDeletePlaylist();
  const removePlaylistTrack = useRemovePlaylistTrack();
  const [editing, setEditing] = useState(false);
  const [addingTrack, setAddingTrack] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [armedTrackID, setArmedTrackID] = useState<string | null>(null);
  const [name, setName] = useState(playlist.name);
  const [description, setDescription] = useState(playlist.description ?? '');
  const tracks = playlist.tracks.map((item) => item.track).filter(Boolean) as Track[];
  const currentTrackID = usePlayerStore((state) => state.currentTrack?.id);
  useEffect(() => { setName(playlist.name); setDescription(playlist.description ?? ''); }, [playlist.name, playlist.description]);
  const save = async (event: FormEvent) => {
    event.preventDefault();
    const next = name.trim();
    if (!next) return;
    await updatePlaylist.mutateAsync({ playlistId: playlist.id, patch: { name: next, description: description.trim() } });
    setEditing(false);
  };
  const removeTrack = async (track: Track) => {
    if (armedTrackID !== track.id) {
      setArmedTrackID(track.id);
      window.setTimeout(() => setArmedTrackID((current) => current === track.id ? null : current), 3000);
      return;
    }
    await removePlaylistTrack.mutateAsync({ playlistId: playlist.id, trackId: track.id });
    setArmedTrackID(null);
  };
  const removePlaylist = async () => {
    await deletePlaylist.mutateAsync(playlist.id);
    setConfirmDelete(false);
  };
  return <section className="overflow-hidden rounded-2xl border border-white/8 bg-surface">
    <div className="grid gap-4 border-b border-white/8 p-4 sm:grid-cols-[auto_minmax(0,1fr)_auto] sm:items-center">
      <button type="button" onClick={() => tracks.length && onPlay(tracks[0], tracks, 0)} disabled={!tracks.length} aria-label="Воспроизвести плейлист" className="group grid grid-cols-[64px_minmax(0,1fr)] items-center gap-3 text-left sm:contents disabled:cursor-default">
        <span className="relative grid h-16 w-16 shrink-0 place-items-center overflow-hidden rounded-xl bg-lime-300/15 text-lime-100 sm:row-span-2">{playlist.cover_url ? <img alt="" src={playlist.cover_url} className="h-full w-full object-cover" /> : <ListMusic size={24} />}<span className="absolute inset-0 grid place-items-center bg-black/35 opacity-0 transition group-enabled:group-hover:opacity-100"><Play size={20} /></span></span>
        {!editing && <span className="min-w-0 sm:hidden"><strong className="block truncate text-lg">{playlist.name}</strong><small className="mt-1 block truncate text-[#8c919e]">{playlist.track_count} · {formatDuration(playlist.duration_seconds)}</small></span>}
      </button>
      {editing ? <form onSubmit={save} className="min-w-0 sm:col-span-2">
        <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]"><input value={name} onChange={(e) => setName(e.target.value)} className="rounded-xl border border-white/10 bg-surface-2 px-4 py-3 outline-none" placeholder="Название плейлиста" /><input value={description} onChange={(e) => setDescription(e.target.value)} className="rounded-xl border border-white/10 bg-surface-2 px-4 py-3 outline-none" placeholder="Описание" /></div>
        <div className="mt-3 flex flex-wrap gap-2"><button type="submit" disabled={!name.trim() || updatePlaylist.isPending} className="rounded-xl bg-lime-300 px-4 py-2 text-sm font-medium text-black disabled:opacity-50">Сохранить</button><button type="button" onClick={() => { setEditing(false); setName(playlist.name); setDescription(playlist.description ?? ''); }} className="rounded-xl border border-white/10 px-4 py-2 text-sm text-[#a6abb7] hover:bg-white/8">Отмена</button></div>
      </form> : <div className="hidden min-w-0 sm:block"><h3 className="m-0 truncate font-display text-xl font-bold tracking-[-.01em]">{playlist.name}</h3>{playlist.description && <p className="m-0 mt-1 line-clamp-2 text-sm text-[#a6abb7]">{playlist.description}</p>}<p className="m-0 mt-1 text-sm text-[#8c919e]">{playlist.track_count} треков · {formatDuration(playlist.duration_seconds)}</p></div>}
      {!editing && <div className="flex items-center justify-end gap-2">
        {tracks.length > 0 && <button type="button" onClick={() => onPlay(tracks[0], tracks, 0)} className="grid h-12 w-12 place-items-center rounded-full bg-lime-300 text-black transition hover:scale-[1.03] active:scale-[.96]" aria-label="Воспроизвести плейлист"><Play size={20} /></button>}
        <button type="button" onClick={() => setEditing(true)} className="grid h-11 w-11 place-items-center rounded-xl border border-white/10 text-[#a6abb7] transition hover:bg-white/8" aria-label="Изменить плейлист" title="Изменить"><Pencil size={18} /></button>
        <button type="button" onClick={() => setConfirmDelete(true)} className="grid h-11 w-11 place-items-center rounded-xl border border-red-300/20 text-red-200 transition hover:bg-red-300/10" aria-label="Удалить плейлист" title="Удалить"><Trash2 size={18} /></button>
      </div>}
      {confirmDelete && <div className="rounded-xl border border-red-300/25 bg-red-300/[0.08] p-4 sm:col-span-3">
        <div className="flex items-start gap-3"><span className="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-red-300/15 text-red-200"><Trash2 size={18} /></span><div className="min-w-0 flex-1"><p className="m-0 font-medium text-red-50">Удалить «{playlist.name}»?</p><p className="m-0 mt-1 text-sm text-red-100/75">Плейлист исчезнет из коллекций, треки останутся в медиатеке.</p></div></div>
        <div className="mt-4 grid grid-cols-2 gap-2"><button onClick={removePlaylist} disabled={deletePlaylist.isPending} className="rounded-xl bg-red-300 px-4 py-3 font-medium text-black disabled:opacity-50">Удалить</button><button onClick={() => setConfirmDelete(false)} className="rounded-xl border border-white/10 px-4 py-3 text-[#d7dbe4] hover:bg-white/8">Оставить</button></div>
      </div>}
    </div>
    {/* Same geometry as TrackRow / Flutter's TrackRow: 52px art, 12px gap, no
        rule between rows — the current track reads as a rounded accent pill. */}
    {tracks.length ? <div className="grid gap-0.5 px-3 py-2">{tracks.map((track, index) => <div key={track.id} aria-current={currentTrackID === track.id ? 'true' : undefined} className={`grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-control px-1 py-2 transition-colors ${currentTrackID === track.id ? 'bg-lime-300/[0.08]' : 'hover:bg-white/[0.03]'}`}><button type="button" onClick={() => onPlay(track, tracks, index)} className="flex min-w-0 items-center gap-3 text-left"><span className="grid h-13 w-13 shrink-0 place-items-center overflow-hidden rounded-control"><TrackArt track={track} className="h-full w-full" /></span><span className="min-w-0"><strong className="block truncate">{track.title}</strong><small className="block truncate text-[#8c919e]">{track.artist || 'Исполнитель не указан'} · {formatDuration(track.duration_seconds)}</small></span></button><div className="flex items-center gap-1"><button aria-label={favoriteIDs.has(track.id) ? 'Убрать лайк' : 'Поставить лайк'} onClick={() => onLike(track)} className={`grid h-10 w-10 place-items-center rounded-lg hover:bg-white/8 ${favoriteIDs.has(track.id) ? 'text-red-300' : ''}`}><Heart size={17} fill={favoriteIDs.has(track.id) ? 'currentColor' : 'none'} /></button><button type="button" onClick={() => removeTrack(track)} disabled={removePlaylistTrack.isPending} className={`grid h-10 w-10 place-items-center rounded-lg ${armedTrackID === track.id ? 'bg-red-300 text-black' : 'text-red-200 hover:bg-red-300/10'}`} aria-label={armedTrackID === track.id ? 'Подтвердить удаление трека из плейлиста' : 'Убрать трек из плейлиста'} title={armedTrackID === track.id ? 'Подтвердить' : 'Убрать'}><Trash2 size={17} /></button></div></div>)}</div> : <div className="p-5 text-sm text-[#9aa0ad]">Плейлист пустой. Найди трек кнопкой ниже или добавь его из меню любого трека.</div>}
    <div className="border-t border-white/8 px-4 py-3">
      <button type="button" onClick={() => setAddingTrack((value) => !value)} aria-expanded={addingTrack} className="inline-flex items-center gap-2 rounded-xl border border-white/10 bg-surface-2 px-4 py-2.5 text-sm text-[#d7dbe4] transition hover:bg-white/8"><Plus size={16} /> Добавить трек</button>
    </div>
    {addingTrack && <PlaylistTrackPicker playlistId={playlist.id} existingIDs={new Set(tracks.map((item) => item.id))} onClose={() => setAddingTrack(false)} />}
  </section>;
}

/* Adding to a playlist used to be possible only from a track's own menu, which
   meant opening a playlist to fill it was a dead end. This is the same search
   the main view uses, scoped to one playlist and debounced the same 350ms. */
function PlaylistTrackPicker({ playlistId, existingIDs, onClose }: { playlistId: string; existingIDs: Set<string>; onClose: () => void }) {
  const [draft, setDraft] = useState('');
  const [query, setQuery] = useState('');
  const [addedIDs, setAddedIDs] = useState<Set<string>>(new Set());
  const addPlaylistTrack = useAddPlaylistTrack();
  useEffect(() => {
    const next = draft.trim();
    if (next === query) return;
    const timer = window.setTimeout(() => setQuery(next), 350);
    return () => window.clearTimeout(timer);
  }, [draft, query]);
  const result = useSearch(query, DEFAULT_PROVIDERS, 10, 0);
  const hasFreshResult = Boolean(query) && result.data?.query === query && !result.isFetching;
  const found = hasFreshResult ? result.data?.items ?? [] : [];
  const add = async (track: Track) => {
    await addPlaylistTrack.mutateAsync({ playlistId, trackId: track.id });
    setAddedIDs((current) => new Set(current).add(track.id));
  };
  return <div className="border-t border-white/8 bg-surface-2/40 p-4">
    <div className="mb-3 flex items-center justify-between gap-3">
      <h4 className="m-0 text-sm font-semibold">Найти трек для плейлиста</h4>
      <button type="button" onClick={onClose} aria-label="Закрыть поиск треков" className="rounded-lg p-1.5 text-[#a6abb7] hover:bg-white/8"><X size={17} /></button>
    </div>
    <div className="flex h-12 items-center gap-3 rounded-xl border border-white/12 bg-surface px-4 transition focus-within:border-lime-300/60">
      <Search size={17} className="text-[#8c919e]" />
      <input autoFocus value={draft} onChange={(event) => setDraft(event.target.value)} placeholder="Название или исполнитель" aria-label="Поиск трека для плейлиста" className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-[#626875]" />
      {result.isFetching && <Loader2 className="animate-spin text-lime-300" size={16} />}
    </div>
    {!query && <p className="m-0 mt-3 text-xs text-[#8c919e]">Начни вводить — результаты подтянутся из подключённых источников.</p>}
    {Boolean(query) && result.isError && <p className="m-0 mt-3 text-xs text-red-200">Поиск не удался. Попробуй ещё раз.</p>}
    {Boolean(found.length) && <div className="mt-3 grid max-h-72 gap-2 overflow-y-auto overscroll-contain pr-1">{found.map((track) => {
      const already = existingIDs.has(track.id) || addedIDs.has(track.id);
      return <div key={track.id} className="grid grid-cols-[42px_minmax(0,1fr)_auto] items-center gap-3 rounded-xl border border-white/10 bg-surface-2 px-3 py-2">
        <span className="grid h-10 w-10 shrink-0 place-items-center overflow-hidden rounded-lg"><TrackArt track={track} className="h-full w-full" /></span>
        <span className="min-w-0"><strong className="block truncate text-sm">{track.title}</strong><small className="block truncate text-[#8c919e]">{track.artist || 'Исполнитель не указан'} · {formatDuration(track.duration_seconds)}</small></span>
        <button type="button" onClick={() => add(track)} disabled={already || addPlaylistTrack.isPending} aria-label={already ? `${track.title} уже в плейлисте` : `Добавить ${track.title}`} title={already ? 'Уже в плейлисте' : 'Добавить'} className={`grid h-10 w-10 place-items-center rounded-lg disabled:opacity-60 ${already ? 'text-lime-300' : 'text-[#a6abb7] hover:bg-white/8'}`}>{already ? <Check size={18} /> : <Plus size={18} />}</button>
      </div>;
    })}</div>}
    {Boolean(query) && hasFreshResult && !found.length && <p className="m-0 mt-3 text-xs text-[#8c919e]">Ничего не нашлось — попробуй другой запрос.</p>}
  </div>;
}

function SettingsView({ providers, riskyEnabled, selectedProviders, toggleProvider, onLogout, localMode = false, onLeaveLocalMode }: { providers: Provider[]; riskyEnabled: boolean; selectedProviders: ProviderId[]; toggleProvider: (id: ProviderId) => void; onLogout?: () => void; localMode?: boolean; onLeaveLocalMode?: () => void }) {
  const defaultBackend = getDefaultBackendBaseURL() || 'текущий origin';
  const session = useSession();
  const users = useUsers();
  const updateAccount = useUpdateAccount();
  const createUser = useCreateUser();
  const updateUser = useUpdateUser();
  const deleteUser = useDeleteUser();
  const [accountName, setAccountName] = useState(session.data?.username ?? '');
  const [accountPassword, setAccountPassword] = useState('');
  const [accountStatus, setAccountStatus] = useState<string | null>(null);
  const [newUserName, setNewUserName] = useState('');
  const [newUserPassword, setNewUserPassword] = useState('');
  const [backendDraft, setBackendDraft] = useState(getBackendBaseURL());
  const [backendStatus, setBackendStatus] = useState<string | null>(null);
  const [accent, setAccent] = useState(getStoredAccent);
  useEffect(() => { setAccountName(session.data?.username ?? ''); }, [session.data?.username]);
  const chooseAccent = (hex: string) => { setAccent(hex); applyAccent(hex); };
  const saveAccount = async (event: FormEvent) => { event.preventDefault(); if (!accountName.trim()) return; await updateAccount.mutateAsync({ username: accountName.trim(), password: accountPassword || undefined }); setAccountPassword(''); setAccountStatus('Аккаунт обновлён.'); };
  const addUser = async (event: FormEvent) => { event.preventDefault(); if (!newUserName.trim() || newUserPassword.length < 10) return; await createUser.mutateAsync({ username: newUserName.trim(), password: newUserPassword }); setNewUserName(''); setNewUserPassword(''); };
  const saveBackend = () => { setBackendBaseURL(backendDraft); setBackendStatus('Сохранено. Перезагрузи экран, чтобы все запросы пошли на новый backend.'); };
  const resetBackend = () => { setBackendDraft(''); setBackendBaseURL(''); setBackendStatus('Сброшено на backend по умолчанию.'); };
  const testBackend = async () => {
    const base = backendDraft.trim().replace(/\/+$/, '');
    setBackendStatus('Проверяю /health…');
    try {
      const response = await fetch(`${base}/health`, { headers: { Accept: 'application/json' } });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      setBackendStatus('Backend отвечает на /health.');
    } catch (error) {
      setBackendStatus(error instanceof Error ? `Backend не ответил: ${error.message}` : 'Backend не ответил.');
    }
  };
  return <>
    <SectionHeader eyebrow="Профиль и backend" title="Настройки" subtitle="Аккаунт, источники и подключение к серверу." />
    {/* Flutter separates every settings block with Divider(height: 40) — 20px of
        air on each side of the rule. `settings-section` is that, minus the rule
        on the first block. */}
    <section className="settings-section settings-section--first">
      <p className="t-eyebrow m-0 text-lime-300">Внешний вид</p>
      <p className="t-body-sm m-0 mt-2 mb-4">Акцентный цвет применяется сразу везде — к кнопкам, плееру и активным состояниям.</p>
      <div className="flex flex-wrap items-center gap-[14px]">
        {ACCENT_PRESETS.map((preset) => <button key={preset.hex} type="button" onClick={() => chooseAccent(preset.hex)} aria-label={preset.name} aria-pressed={accent === preset.hex} title={preset.name} className={`h-11 w-11 shrink-0 rounded-full border-2 transition ${accent === preset.hex ? 'border-white scale-105' : 'border-white/25 hover:scale-105'}`} style={{ background: preset.hex }} />)}
        <label className="grid h-11 w-11 shrink-0 cursor-pointer place-items-center rounded-full border-2 border-dashed border-white/25 text-muted hover:border-white/50" title="Свой цвет">
          <input type="color" value={accent} onChange={(e) => chooseAccent(e.target.value)} className="sr-only" aria-label="Свой акцентный цвет" />
          <Pencil size={15} />
        </label>
      </div>
    </section>
    {(onLogout || localMode) && <section className="settings-section">
      <p className="t-eyebrow m-0 mb-2 text-lime-300">Аккаунт</p>
      <div className="flex flex-wrap items-center justify-between gap-3"><p className="m-0 text-sm text-[#a6abb7]">{localMode ? 'Локальный режим: без аккаунта, данные живут на этом устройстве.' : `${session.data?.username ?? 'Аккаунт'} · ${session.data?.role === 'admin' ? 'администратор' : 'пользователь'}`}</p>{localMode ? <button type="button" onClick={onLeaveLocalMode} className="inline-flex items-center gap-2 rounded-xl border border-white/10 px-4 py-3 text-sm text-[#c6cad3] transition hover:bg-white/8 active:scale-[0.98]"><LogOut size={16} /> Вернуться к экрану входа</button> : <button type="button" onClick={onLogout} className="inline-flex items-center gap-2 rounded-xl border border-red-300/30 px-4 py-3 text-sm text-red-100 transition hover:bg-red-300/10 active:scale-[0.98]"><LogOut size={16} /> Выйти</button>}</div>
      {!localMode && session.data?.role === 'admin' && <div className="mt-5 grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <form onSubmit={saveAccount} className="rounded-2xl border border-white/10 bg-surface p-4"><h3 className="m-0 text-lg">Данные владельца</h3><p className="m-0 mt-1 text-sm text-[#8c919e]">Роль владельца: администратор.</p><div className="mt-4 grid gap-3"><input value={accountName} onChange={(e) => setAccountName(e.target.value)} className="rounded-xl border border-white/10 bg-surface-2 px-4 py-3 outline-none" placeholder="Логин" /><input type="password" value={accountPassword} onChange={(e) => setAccountPassword(e.target.value)} className="rounded-xl border border-white/10 bg-surface-2 px-4 py-3 outline-none" placeholder="Новый пароль — минимум 10 символов" /><button type="submit" disabled={!accountName.trim() || updateAccount.isPending} className="rounded-xl bg-lime-300 px-4 py-3 font-medium text-black disabled:opacity-50">Сохранить аккаунт</button>{accountStatus && <p className="m-0 text-sm text-lime-200">{accountStatus}</p>}</div></form>
        <section className="rounded-2xl border border-white/10 bg-surface p-4"><h3 className="m-0 text-lg">Пользователи</h3><p className="m-0 mt-1 text-sm text-[#8c919e]">Новые пользователи получают роль «пользователь» и не видят админ-раздел.</p><form onSubmit={addUser} className="mt-4 grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]"><input value={newUserName} onChange={(e) => setNewUserName(e.target.value)} className="rounded-xl border border-white/10 bg-surface-2 px-4 py-3 outline-none" placeholder="Логин" /><input type="password" value={newUserPassword} onChange={(e) => setNewUserPassword(e.target.value)} className="rounded-xl border border-white/10 bg-surface-2 px-4 py-3 outline-none" placeholder="Пароль" /><button disabled={!newUserName.trim() || newUserPassword.length < 10 || createUser.isPending} className="rounded-xl bg-lime-300 px-4 py-3 font-medium text-black disabled:opacity-50">Добавить</button></form><div className="mt-4 grid gap-2">{users.data?.map((user) => <UserRow key={user.id} user={user} onSave={(patch) => updateUser.mutate({ userId: user.id, patch })} onDelete={() => deleteUser.mutate(user.id)} />)}{!users.data?.length && <p className="m-0 rounded-xl border border-white/10 bg-surface-2 p-3 text-sm text-[#8c919e]">Пользователей пока нет.</p>}</div></section>
      </div>}
    </section>}
    <section className="settings-section">
      <p className="t-eyebrow m-0 text-lime-300">Источники поиска</p>
      <p className="t-body-sm m-0 mt-2">Где искать музыку — включи нужные источники.</p>
      <div className="mt-4 grid gap-3 md:grid-cols-2">{providers.filter((p) => p.capabilities.search || p.id === 'local').map((p) => {
        const selected = selectedProviders.includes(p.id);
        const disabled = !p.enabled || !p.capabilities.search;
        return <button key={p.id} type="button" disabled={disabled} onClick={() => toggleProvider(p.id)} aria-pressed={selected} className={`source-toggle ${selected && !disabled ? 'source-toggle--active' : ''}`}>
          <span className="flex min-w-0 items-center gap-3"><SourceIcon id={p.id} className="h-5 w-5 shrink-0" /><span className="min-w-0"><strong className="block truncate">{providerName(p)}</strong><small className="block text-[#8c919e]">{disabled ? 'Недоступен на сервере' : selected ? 'Включён' : 'Выключен'}</small></span></span>
          <span className={`source-switch ${selected && !disabled ? 'source-switch--on' : ''}`}><span /></span>
        </button>;
      })}</div>
    </section>
    {session.data?.role === 'admin' && <ServerSettingsSection />}
    <details className="settings-section text-sm text-[#a6abb7]">
      <summary className="t-eyebrow cursor-pointer select-none">Подключение к серверу</summary>
      <label className="mt-4 block text-sm text-[#a6abb7]" htmlFor="backend-url">Адрес API</label>
      <div className="mt-2 flex flex-col gap-2 md:flex-row">
        <input id="backend-url" value={backendDraft} onChange={(event) => setBackendDraft(event.target.value)} placeholder="Пусто = текущий домен" className="min-w-0 flex-1 rounded-xl border border-white/10 bg-surface-2 px-4 py-3 outline-none" />
        <button type="button" onClick={testBackend} className="rounded-xl border border-white/10 px-4 py-3 text-sm hover:bg-white/8">Проверить</button>
        <button type="button" onClick={saveBackend} className="rounded-xl bg-lime-300 px-4 py-3 text-sm font-medium text-black">Сохранить</button>
        <button type="button" onClick={resetBackend} className="rounded-xl border border-white/10 px-4 py-3 text-sm text-[#a6abb7] hover:bg-white/8">Сброс</button>
      </div>
      <p className="m-0 mt-2 text-xs text-[#8c919e]">По умолчанию: {defaultBackend}. Настройка хранится в этом браузере.</p>
      {backendStatus && <p className="m-0 mt-3 rounded-xl border border-white/10 bg-surface-2 px-3 py-2 text-sm text-[#d7dbe4]">{backendStatus}</p>}
    </details>
    <div className="mt-8 flex items-center justify-center gap-2 text-xs text-[#626875]">
      <span className="inline-block h-4 w-4 rounded bg-lime-300/20 text-center text-[10px] font-bold leading-4 text-lime-300">O</span>
      <span>Music Orchestrator v{__APP_VERSION__}</span>
    </div>
  </>;
}

function UserRow({ user, onSave, onDelete }: { user: User; onSave: (patch: { username?: string; password?: string }) => void; onDelete: () => void }) {
  const [editing, setEditing] = useState(false);
  const [username, setUsername] = useState(user.username);
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState(false);
  useEffect(() => setUsername(user.username), [user.username]);
  return <div className="rounded-xl border border-white/10 bg-surface-2 p-3">{editing ? <form onSubmit={(event) => { event.preventDefault(); onSave({ username: username.trim(), password: password || undefined }); setPassword(''); setEditing(false); }} className="grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto_auto]"><input value={username} onChange={(e) => setUsername(e.target.value)} className="rounded-lg border border-white/10 bg-black/25 px-3 py-2 outline-none" /><input type="password" value={password} onChange={(e) => setPassword(e.target.value)} className="rounded-lg border border-white/10 bg-black/25 px-3 py-2 outline-none" placeholder="Новый пароль" /><button disabled={!username.trim()} className="rounded-lg bg-lime-300 px-3 py-2 text-black disabled:opacity-50">OK</button><button type="button" onClick={() => setEditing(false)} className="rounded-lg border border-white/10 px-3 py-2 text-[#a6abb7]">Отмена</button></form> : <div className="flex flex-wrap items-center justify-between gap-2"><span><strong>{user.username}</strong><small className="ml-2 text-[#8c919e]">роль: пользователь</small></span><span className="flex gap-2"><button type="button" onClick={() => setEditing(true)} className="rounded-lg border border-white/10 px-3 py-2 text-sm text-[#a6abb7] hover:bg-white/8">Изменить</button><button type="button" onClick={() => confirm ? onDelete() : setConfirm(true)} className={`rounded-lg px-3 py-2 text-sm ${confirm ? 'bg-red-300 text-black' : 'text-red-200 hover:bg-red-300/10'}`}>{confirm ? 'Удалить?' : 'Удалить'}</button></span></div>}</div>;
}


function MiniPlayerTitle({ text }: { text: string }) {
  const outerRef = useRef<HTMLDivElement | null>(null);
  const innerRef = useRef<HTMLSpanElement | null>(null);
  const [overflowing, setOverflowing] = useState(false);
  useEffect(() => {
    const measure = () => setOverflowing(Boolean(outerRef.current && innerRef.current && innerRef.current.scrollWidth > outerRef.current.clientWidth + 8));
    measure();
    const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(measure) : null;
    if (outerRef.current) ro?.observe(outerRef.current);
    window.addEventListener('resize', measure);
    return () => { ro?.disconnect(); window.removeEventListener('resize', measure); };
  }, [text]);
  return <div ref={outerRef} className={`player-marquee text-[13px] font-semibold md:text-sm ${overflowing ? 'player-marquee--animate' : ''}`}><span ref={innerRef}>{text}</span></div>;
}

function PlayerBar({ favoriteIDs, downloadedByTrack, onLike }: { favoriteIDs: Set<string>; downloadedByTrack: Map<string | undefined, Job>; onLike: (track: Track) => void }) {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const touchStartX = useRef<number | null>(null);
  const touchStartY = useRef<number | null>(null);
  const sheetTouchStartX = useRef<number | null>(null);
  const sheetTouchStartY = useRef<number | null>(null);
  const sheetTouchTarget = useRef<HTMLElement | null>(null);
  const sheetDragActive = useRef(false);
  const closeTimer = useRef<number | null>(null);
  const seekingRef = useRef(false);
  const seekDraftRef = useRef(0);
  const didSwipe = useRef(false);
  const autoAdvancedRef = useRef(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [bufferedTime, setBufferedTime] = useState(0);
  const [expanded, setExpanded] = useState(false);
  const [sheetClosing, setSheetClosing] = useState(false);
  const [sheetOffset, setSheetOffset] = useState(0);
  const [miniEntered, setMiniEntered] = useState(false);
  const [isSeeking, setIsSeeking] = useState(false);
  const [seekDraft, setSeekDraft] = useState(0);
  const [miniMenuOpen, setMiniMenuOpen] = useState(false);
  const currentTrack = usePlayerStore((s) => s.currentTrack);
  const playback = usePlayerStore((s) => s.playback);
  const setPlayback = usePlayerStore((s) => s.setPlayback);
  const error = usePlayerStore((s) => s.error);
  const setError = usePlayerStore((s) => s.setError);
  const status = usePlayerStore((s) => s.status);
  const setStatus = usePlayerStore((s) => s.setStatus);
  const volume = usePlayerStore((s) => s.volume);
  const setVolume = usePlayerStore((s) => s.setVolume);
  const shuffle = usePlayerStore((s) => s.shuffle);
  const repeat = usePlayerStore((s) => s.repeat);
  const toggleShuffle = usePlayerStore((s) => s.toggleShuffle);
  const cycleRepeat = usePlayerStore((s) => s.cycleRepeat);
  const toggleTrack = usePlayerStore((s) => s.toggleTrack);
  const next = usePlayerStore((s) => s.next);
  const previous = usePlayerStore((s) => s.previous);
  const queue = usePlayerStore((s) => s.queue);
  const queueIndex = usePlayerStore((s) => s.queueIndex);
  const playQueue = usePlayerStore((s) => s.playQueue);
  const updateDuration = usePlayerStore((s) => s.updateDuration);
  const createDownload = useCreateDownload();
  const query = usePlayback(currentTrack?.id ?? null);

  const runPlayerMorph = (callback: () => void) => {
    const startViewTransition = (document as Document & { startViewTransition?: (cb: () => void) => { finished?: Promise<unknown> } }).startViewTransition;
    if (typeof startViewTransition === 'function') { startViewTransition.call(document, callback); return true; }
    callback();
    return false;
  };
  const closePlayer = () => {
    if (!expanded || sheetClosing) return;
    const morphed = runPlayerMorph(() => { setExpanded(false); setSheetClosing(false); });
    if (morphed) return;
    setSheetClosing(true);
    if (closeTimer.current) window.clearTimeout(closeTimer.current);
    closeTimer.current = window.setTimeout(() => { setExpanded(false); setSheetClosing(false); closeTimer.current = null; }, 260);
  };

  useEffect(() => { if (query.data) setPlayback(query.data); }, [query.data, setPlayback]);
  useEffect(() => { if (query.error) setError(query.error instanceof Error ? query.error.message : 'Playback failed'); }, [query.error, setError]);
  useEffect(() => { const audio = audioRef.current; if (audio) audio.volume = volume; }, [volume]);
  useEffect(() => { const audio = audioRef.current; if (!audio) return; if (status === 'paused') { audio.pause(); return; } if (playback?.stream_url && status === 'playing') audio.play().catch((error) => setError(error instanceof Error ? error.message : 'Autoplay blocked')); }, [playback?.stream_url, status, setError]);
  useEffect(() => { seekingRef.current = false; seekDraftRef.current = 0; autoAdvancedRef.current = false; setCurrentTime(0); setDuration(0); setBufferedTime(0); setIsSeeking(false); setSeekDraft(0); }, [currentTrack?.id]);
  useEffect(() => () => { if (closeTimer.current) window.clearTimeout(closeTimer.current); }, []);
  useEffect(() => { const frame = requestAnimationFrame(() => setMiniEntered(true)); return () => cancelAnimationFrame(frame); }, []);
  useEffect(() => {
    if (!currentTrack || !('mediaSession' in navigator)) return;
    // iOS refuses to fetch the raw YouTube/SoundCloud artwork host (CORS), and it
    // drops artwork entries that declare no sizes/type — so the lock screen ended
    // up blank. The backend proxy plus explicit descriptors fixes both.
    const artwork = artworkURL(currentTrack.artwork_url);
    navigator.mediaSession.metadata = new MediaMetadata({ title: currentTrack.title, artist: currentTrack.artist ?? 'Исполнитель не указан', artwork: artwork ? [{ src: artwork, sizes: '512x512', type: 'image/jpeg' }] : [] });
    navigator.mediaSession.setActionHandler('play', () => toggleTrack(currentTrack));
    navigator.mediaSession.setActionHandler('pause', () => toggleTrack(currentTrack));
    navigator.mediaSession.setActionHandler('previoustrack', previous);
    navigator.mediaSession.setActionHandler('nexttrack', next);
  }, [currentTrack, next, previous, toggleTrack]);
  useEffect(() => {
    if (!expanded) return;
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') closePlayer(); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [expanded]);
  useEffect(() => {
    if (!expanded) return;
    const previousOverflow = document.body.style.overflow;
    const previousOverscroll = document.body.style.overscrollBehavior;
    document.body.style.overflow = 'hidden';
    document.body.style.overscrollBehavior = 'none';
    return () => { document.body.style.overflow = previousOverflow; document.body.style.overscrollBehavior = previousOverscroll; };
  }, [expanded]);

  if (!currentTrack) return null;
  const activeRepeat = repeat !== 'off';
  const canSeek = duration > 0 && Number.isFinite(duration);
  const seekValue = canSeek ? (isSeeking ? seekDraft : currentTime) : 0;
  const progress = canSeek ? Math.min(100, (seekValue / duration) * 100) : 0;
  const bufferProgress = canSeek ? Math.min(100, Math.max(progress, (bufferedTime / duration) * 100)) : 0;
  const updateBuffered = (audio: HTMLAudioElement) => {
    const mediaDuration = Number.isFinite(audio.duration) ? audio.duration : 0;
    // YouTube's CDN sends a Content-Length for the whole video+audio container,
    // so the audio element reports roughly double the real track and then plays
    // silence to the end. When the element claims far more than the provider
    // metadata, the provider number is the one that matches the audio.
    const providerDuration = currentTrack.duration_seconds ?? 0;
    const overreported = providerDuration > 0 && mediaDuration > providerDuration * OVERREPORTED_DURATION_RATIO;
    const effectiveDuration = overreported ? providerDuration : mediaDuration;
    if (effectiveDuration > 0) setDuration(effectiveDuration);
    let bufferedEnd = 0;
    for (let index = 0; index < audio.buffered.length; index += 1) {
      const start = audio.buffered.start(index);
      const end = audio.buffered.end(index);
      if (audio.currentTime >= start && audio.currentTime <= end) { bufferedEnd = end; break; }
      if (end > bufferedEnd) bufferedEnd = end;
    }
    setBufferedTime(Math.max(0, Math.min(bufferedEnd, effectiveDuration || bufferedEnd)));
  };
  /* When the element overreports duration it keeps "playing" silence past the real
     end, so its own `ended` event arrives minutes late. Advance at the corrected
     end instead — and rewind by hand for repeat-one, since the loop attribute
     would also wait for the bogus end. */
  const advanceIfTrackEnded = (audio: HTMLAudioElement) => {
    const providerDuration = currentTrack.duration_seconds ?? 0;
    const mediaDuration = Number.isFinite(audio.duration) ? audio.duration : 0;
    if (providerDuration <= 0 || mediaDuration <= providerDuration * OVERREPORTED_DURATION_RATIO) return;
    if (audio.currentTime < providerDuration || autoAdvancedRef.current) return;
    autoAdvancedRef.current = true;
    if (repeat === 'one') { autoAdvancedRef.current = false; audio.currentTime = 0; return; }
    next();
  };
  const liked = favoriteIDs.has(currentTrack.id);
  const cached = downloadedByTrack.get(currentTrack.id);
  const downloaded = currentTrack.downloaded || Boolean(currentTrack.download_media_url || getMediaURL(cached));
  const beginSeek = () => { if (!canSeek) return; seekingRef.current = true; seekDraftRef.current = currentTime; setIsSeeking(true); setSeekDraft(currentTime); };
  const updateSeekDraft = (value: number) => { if (!canSeek) return; const nextValue = Math.max(0, Math.min(value, duration)); seekDraftRef.current = nextValue; setSeekDraft(nextValue); };
  const commitSeek = () => {
    if (!canSeek || !seekingRef.current) return;
    const audio = audioRef.current;
    const nextTime = Math.max(0, Math.min(seekDraftRef.current, duration));
    seekingRef.current = false;
    setIsSeeking(false);
    setCurrentTime(nextTime);
    if (!audio) return;
    try { audio.currentTime = nextTime; }
    catch (error) { setError(error instanceof Error ? `Перемотка недоступна: ${error.message}` : 'Перемотка недоступна для этого потока'); }
  };
  const download = async () => { if (!currentTrack.policy.download_allowed || downloaded) return; await createDownload.mutateAsync({ trackId: currentTrack.id, format: 'mp3' }); setMiniMenuOpen(false); analytics.track('download_requested', { provider_id: currentTrack.provider_id }); };
  const playPause = () => toggleTrack(currentTrack);
  const onTouchStart = (event: TouchEvent) => {
    const target = event.target as HTMLElement;
    if (target.closest('[data-player-control="true"]')) {
      touchStartX.current = null;
      touchStartY.current = null;
      return;
    }
    touchStartX.current = event.touches[0]?.clientX ?? null;
    touchStartY.current = event.touches[0]?.clientY ?? null;
  };
  const onTouchEnd = (event: TouchEvent) => { const startX = touchStartX.current; const startY = touchStartY.current; touchStartX.current = null; touchStartY.current = null; if (startX === null || startY === null) return; const deltaX = (event.changedTouches[0]?.clientX ?? startX) - startX; const deltaY = (event.changedTouches[0]?.clientY ?? startY) - startY; if (Math.abs(deltaX) < 48 || Math.abs(deltaX) < Math.abs(deltaY) * 1.5) return; didSwipe.current = true; setMiniMenuOpen(false); window.setTimeout(() => { didSwipe.current = false; }, 350); if (deltaX < 0) next(); else previous(); };
  const actionClass = 'grid h-8 w-8 place-items-center rounded-xl border border-white/10 text-[#a6abb7] disabled:opacity-40 sm:h-9 sm:w-9';
  const openPlayer = () => { if (closeTimer.current) window.clearTimeout(closeTimer.current); closeTimer.current = null; runPlayerMorph(() => { setSheetClosing(false); setExpanded(true); }); };
  const onSheetTouchStart = (event: TouchEvent) => {
    const target = event.target as HTMLElement;
    sheetTouchStartY.current = event.touches[0]?.clientY ?? null;
    sheetTouchStartX.current = event.touches[0]?.clientX ?? null;
    sheetTouchTarget.current = target;
    if (target.closest('[data-player-control="true"]')) { sheetDragActive.current = false; return; }
    if (target.closest('[data-sheet-handle]')) { sheetDragActive.current = true; return; }
    let scrolled: HTMLElement | null = target;
    sheetDragActive.current = true;
    while (scrolled && scrolled !== event.currentTarget) { if (scrolled.scrollTop > 0) { sheetDragActive.current = false; break; } scrolled = scrolled.parentElement; }
  };
  const onSheetTouchMove = (event: TouchEvent) => {
    if (!sheetDragActive.current || sheetTouchStartY.current === null) return;
    const deltaY = (event.touches[0]?.clientY ?? sheetTouchStartY.current) - sheetTouchStartY.current;
    setSheetOffset(Math.max(0, deltaY));
  };
  const onSheetTouchEnd = (event: TouchEvent) => { const startY = sheetTouchStartY.current; const startX = sheetTouchStartX.current; const target = sheetTouchTarget.current; const canClose = sheetDragActive.current; sheetTouchStartY.current = null; sheetTouchStartX.current = null; sheetTouchTarget.current = null; sheetDragActive.current = false; setSheetOffset(0); if (startY === null || startX === null || !canClose) return; if (target?.closest('[data-player-control="true"]')) return; const deltaY = (event.changedTouches[0]?.clientY ?? startY) - startY; const deltaX = (event.changedTouches[0]?.clientX ?? startX) - startX; if (deltaY > 90 && deltaY > Math.abs(deltaX) * 1.2) closePlayer(); };
  const openFromShell = (event: MouseEvent<HTMLElement>) => {
    const target = event.target as HTMLElement;
    if (didSwipe.current || target.closest('[data-player-control="true"]')) return;
    openPlayer();
  };
  const range = <div className="seek-control" aria-label={`Загружено ${formatDuration(bufferedTime)}`} data-buffer-percent={bufferProgress.toFixed(0)}>
    <div className="seek-control__rail" aria-hidden="true"><span className="seek-control__buffer" style={{ width: `${bufferProgress}%` }} /><span className="seek-control__progress" style={{ width: `${progress}%` }} /></div>
    <input data-player-control="true" className="range seek-control__input h-3 touch-pan-x disabled:opacity-50" type="range" min={0} max={duration || 0} step="1" value={seekValue} disabled={!canSeek} onPointerDown={(event) => { event.stopPropagation(); beginSeek(); }} onPointerUp={(event) => { event.stopPropagation(); commitSeek(); }} onPointerCancel={(event) => { event.stopPropagation(); seekingRef.current = false; setIsSeeking(false); }} onTouchStart={(event) => event.stopPropagation()} onTouchEnd={(event) => { event.stopPropagation(); commitSeek(); }} onMouseUp={(event) => { event.stopPropagation(); commitSeek(); }} onKeyUp={(event) => { if (event.key === 'ArrowLeft' || event.key === 'ArrowRight' || event.key === 'Home' || event.key === 'End') commitSeek(); }} onClick={(event) => event.stopPropagation()} onChange={(event) => updateSeekDraft(Number(event.currentTarget.value))} aria-label={canSeek ? 'Перемотка' : 'Перемотка недоступна, трек ещё грузится'} />
  </div>;

  /* The audio element lives outside the collapsed/expanded branch: mounting it
     inside the mini player meant expanding to the full player tore it down and
     `autoPlay` restarted the track from zero. */
  const audio = playback?.stream_url ? <audio ref={audioRef} className="hidden" src={playback.stream_url} autoPlay loop={repeat === 'one'} onTimeUpdate={(e) => { if (!isSeeking) setCurrentTime(e.currentTarget.currentTime); updateBuffered(e.currentTarget); advanceIfTrackEnded(e.currentTarget); }} onProgress={(e) => updateBuffered(e.currentTarget)} onLoadedMetadata={(e) => { e.currentTarget.volume = volume; updateBuffered(e.currentTarget); updateDuration(e.currentTarget.duration); }} onDurationChange={(e) => updateDuration(e.currentTarget.duration)} onWaiting={() => setStatus('buffering')} onCanPlay={(e) => { updateBuffered(e.currentTarget); if (status === 'buffering') setStatus('playing'); }} onPlay={() => setStatus('playing')} onPause={() => { if (status !== 'playing') setStatus('paused'); }} onEnded={next} /> : null;

  return <>
    {audio}
    {!expanded && (<footer onClick={openFromShell} onTouchStart={onTouchStart} onTouchEnd={onTouchEnd} className={`player-morph-card touch-pan-y fixed inset-x-3 bottom-[calc(84px+env(safe-area-inset-bottom,0px))] z-20 mx-auto max-w-5xl cursor-pointer rounded-card bg-surface-2 p-2 shadow-[0_8px_24px_rgba(0,0,0,.45)] md:bottom-2 md:grid md:grid-cols-[minmax(0,1fr)_minmax(300px,560px)] md:gap-3 md:p-3 lg:inset-x-0 lg:bottom-0 lg:z-40 lg:max-w-none lg:grid-cols-[minmax(220px,1fr)_minmax(360px,640px)] lg:items-center lg:justify-between lg:gap-6 lg:rounded-none lg:border-t lg:border-white/10 lg:px-6 lg:py-3 ${miniEntered ? '' : 'player-mini--enter'}`}>
      {error && <div role="alert" className="col-span-full mb-2 flex items-center justify-between gap-3 rounded-2xl border border-red-300/30 bg-red-300/10 px-3 py-2 text-sm text-red-100"><span className="min-w-0 truncate">{error}</span><button type="button" onClick={() => setError(null)} className="rounded-xl p-1 hover:bg-white/10" aria-label="Скрыть ошибку"><X size={16} /></button></div>}
      <div className="grid grid-cols-[44px_minmax(0,1fr)_auto] items-center gap-2.5 md:flex md:min-w-0 md:gap-3">
        <span className="relative grid h-11 w-11 shrink-0 place-items-center overflow-hidden rounded-control"><TrackArt track={currentTrack} className="h-full w-full" /></span>
        <button type="button" data-open-player="true" onClick={openPlayer} className="min-w-0 overflow-hidden text-left" aria-label="Открыть плеер"><MiniPlayerTitle text={`${currentTrack.title} — ${currentTrack.artist ?? 'Исполнитель не указан'}`} /><div className="mt-0.5 flex items-center gap-1 text-xs text-muted"><SourceIcon id={currentTrack.provider_id} className="h-[13px] w-[13px] shrink-0" /><span className="truncate">{currentTrack.artist ?? 'Выбери трек'}</span></div></button>
        <div data-player-control="true" className="relative flex items-center gap-1 justify-self-end md:hidden">
          <button aria-label="Меню плеера" aria-haspopup="menu" aria-expanded={miniMenuOpen} onClick={() => setMiniMenuOpen((value) => !value)} className={`${actionClass} ${shuffle || activeRepeat || liked || downloaded ? 'text-lime-200' : ''}`}><MoreHorizontal size={17} /></button>
          {miniMenuOpen && <><button type="button" aria-label="Закрыть меню плеера" tabIndex={-1} onClick={() => setMiniMenuOpen(false)} className="fixed inset-0 z-30 cursor-default" /><div role="menu" className="absolute right-0 bottom-11 z-40 w-64 overflow-hidden rounded-xl border border-white/10 bg-surface-3 p-1 shadow-2xl shadow-black/50">
            <button role="menuitem" onClick={() => { previous(); setMiniMenuOpen(false); }} className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8"><SkipBack size={16} /> Предыдущий трек</button>
            <button role="menuitem" onClick={() => { next(); setMiniMenuOpen(false); }} className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8"><SkipForward size={16} /> Следующий трек</button>
            <button role="menuitem" aria-pressed={liked} onClick={() => { onLike(currentTrack); setMiniMenuOpen(false); }} className={`flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8 ${liked ? 'text-red-300' : ''}`}><Heart size={16} fill={liked ? 'currentColor' : 'none'} /> {liked ? 'Убрать лайк' : 'Лайк'}</button>
            <button role="menuitem" disabled={!currentTrack.policy.download_allowed || downloaded || createDownload.isPending} onClick={download} className={`flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8 disabled:opacity-50 ${downloaded ? 'text-lime-300' : ''}`}>{createDownload.isPending ? <Loader2 className="animate-spin" size={16} /> : downloaded ? <Folder size={16} /> : <Download size={16} />} {downloaded ? 'Уже скачано' : 'Скачать mp3'}</button>
            <button role="menuitem" aria-pressed={shuffle} onClick={() => { toggleShuffle(); setMiniMenuOpen(false); }} className={`flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8 ${shuffle ? 'text-lime-300' : ''}`}><Shuffle size={16} /> {shuffle ? 'Выключить шафл' : 'Включить шафл'}</button>
            <button role="menuitem" aria-pressed={activeRepeat} onClick={() => { cycleRepeat(); setMiniMenuOpen(false); }} className={`flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8 ${activeRepeat ? 'text-lime-300' : ''}`}>{repeat === 'one' ? <Repeat1 size={16} /> : <Repeat size={16} />} Повтор: {repeat === 'off' ? 'выкл' : repeat === 'all' ? 'всё' : 'один'}</button>
            <button role="menuitem" onClick={() => { setMiniMenuOpen(false); openPlayer(); }} className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8"><Music2 size={16} /> Открыть плеер</button>
          </div></>}
          <button aria-label={status === 'playing' ? 'Pause' : 'Play'} onClick={playPause} className="grid h-9 w-9 place-items-center rounded-full text-lime-300">{query.isFetching ? <Loader2 className="animate-spin" size={26} /> : status === 'playing' ? <Pause size={26} fill="currentColor" /> : <Play size={26} fill="currentColor" />}</button>
        </div>
        <div data-player-control="true" className="hidden gap-2 md:flex"><button aria-label={liked ? 'Убрать лайк' : 'Поставить лайк'} onClick={() => onLike(currentTrack)} className={`grid h-11 w-11 place-items-center rounded-2xl border border-white/10 hover:bg-white/8 ${liked ? 'text-red-300' : 'text-[#a6abb7]'}`}><Heart size={18} fill={liked ? 'currentColor' : 'none'} /></button><button aria-label={downloaded ? 'Уже скачано' : 'Скачать'} disabled={!currentTrack.policy.download_allowed || downloaded || createDownload.isPending} onClick={download} className={`grid h-11 w-11 place-items-center rounded-2xl border border-white/10 hover:bg-white/8 disabled:opacity-50 ${downloaded ? 'text-lime-300' : 'text-[#a6abb7]'}`}>{createDownload.isPending ? <Loader2 className="animate-spin" size={18} /> : downloaded ? <Folder size={18} /> : <Download size={18} />}</button></div>
      </div>
      {/* Flutter's mini player carries a hairline progress bar and nothing else;
          the scrubber and transport are the full player's job. Desktop keeps
          them because there is room and no full-screen player to fall back to. */}
      <div aria-hidden="true" className="pointer-events-none absolute inset-x-2 bottom-1.5 h-[3px] overflow-hidden rounded-full bg-white/10 md:hidden"><span className="block h-full rounded-full bg-lime-300" style={{ width: `${progress}%` }} /></div>
      <div data-player-control="true" className="mt-2 hidden min-w-0 gap-2 md:mt-0 md:grid"><div className="hidden items-center justify-center gap-2 md:flex"><button aria-label="Shuffle" onClick={toggleShuffle} className={`rounded-xl p-2 ${shuffle ? 'bg-lime-300 text-black' : 'hover:bg-white/8 text-[#a6abb7]'}`}><Shuffle size={17} /></button><button aria-label="Previous" onClick={previous} className="rounded-xl p-2 text-[#a6abb7] hover:bg-white/8"><SkipBack size={18} /></button><button aria-label={status === 'playing' ? 'Pause' : 'Play'} onClick={playPause} className="grid h-10 w-10 place-items-center rounded-full bg-lime-300 text-black shadow-lg shadow-lime-300/20">{query.isFetching ? <Loader2 className="animate-spin" size={19} /> : status === 'playing' ? <Pause size={19} /> : <Play size={19} />}</button><button aria-label="Next" onClick={next} className="rounded-xl p-2 text-[#a6abb7] hover:bg-white/8"><SkipForward size={18} /></button><button aria-label="Repeat" onClick={cycleRepeat} className={`rounded-xl p-2 ${activeRepeat ? 'bg-lime-300 text-black' : 'hover:bg-white/8 text-[#a6abb7]'}`}>{repeat === 'one' ? <Repeat1 size={17} /> : <Repeat size={17} />}</button><input className="range max-w-24" type="range" min={0} max={1} step={0.01} value={volume} onChange={(event) => setVolume(Number(event.currentTarget.value))} aria-label="Громкость" /></div><div className="grid grid-cols-[42px_1fr_42px] items-center gap-2 font-mono text-[11px] text-muted"><span>{formatDuration(seekValue)}</span>{range}<span>{formatDuration(duration)}</span></div></div>
    </footer>)}
    {expanded && <div className={`player-sheet-backdrop fixed inset-0 z-50 grid place-items-end bg-black/70 p-0 backdrop-blur md:place-items-center md:p-3 lg:place-items-stretch lg:justify-items-end lg:bg-black/40 lg:p-0 ${sheetClosing ? 'player-sheet-backdrop--closing' : ''}`} role="dialog" aria-modal="true" aria-label="Плеер" onClick={closePlayer}>
      <section onClick={(event) => event.stopPropagation()} onTouchStart={onSheetTouchStart} onTouchMove={onSheetTouchMove} onTouchEnd={onSheetTouchEnd} style={sheetOffset > 0 ? { transform: `translateY(${sheetOffset}px)`, transition: 'none' } : { transition: 'transform 320ms cubic-bezier(.22,1,.36,1)' }} data-player-panel="true" className={`player-sheet player-morph-card max-h-[92dvh] w-full overflow-hidden rounded-t-sheet border border-white/10 bg-surface shadow-2xl md:max-w-md md:rounded-sheet lg:h-full lg:max-h-none lg:w-[420px] lg:max-w-none lg:rounded-none lg:rounded-l-sheet lg:border-y-0 lg:border-r-0 ${sheetClosing ? 'player-sheet--closing' : ''}`}>
        {/* Order and spacing follow NowPlayingSheet: handle → art → title →
            progress → times → actions → transport → queue. */}
        <div className="max-h-[92dvh] touch-pan-y overflow-y-auto overscroll-contain px-5 pt-3 pb-8">
          <div data-sheet-handle="true" className="relative flex h-6 cursor-grab items-center justify-center">
            <span className="h-[5px] w-11 rounded-full bg-white/25" aria-hidden="true" />
            <button onClick={closePlayer} aria-label="Закрыть плеер" className="absolute right-0 grid h-9 w-9 place-items-center rounded-full text-muted transition hover:bg-white/8"><X size={20} /></button>
          </div>
          <div className="mt-6 aspect-square w-full overflow-hidden rounded-card shadow-2xl shadow-black/35"><TrackArt track={currentTrack} className="h-full w-full" /></div>
          <h2 className="t-headline-md m-0 mt-6 truncate text-2xl">{currentTrack.title}</h2>
          <p className="t-body m-0 mt-1 flex items-center gap-2 text-muted"><SourceIcon id={currentTrack.provider_id} className="h-4 w-4 shrink-0" /><span className="truncate">{currentTrack.artist ?? 'Исполнитель не указан'}</span></p>
          <div className="mt-6">{range}</div>
          <div className="mt-2 flex items-center justify-between font-mono text-xs text-muted"><span>{formatDuration(seekValue)}</span><span>{formatDuration(duration)}</span></div>
          {status === 'buffering' && <p className="m-0 mt-2 text-center text-xs text-muted">Буферизую… загружено {formatDuration(bufferedTime)} · {bufferProgress.toFixed(0)}%</p>}
          <div className="mt-6 flex items-center justify-center gap-3">
            <button aria-label={liked ? 'Убрать лайк' : 'Поставить лайк'} aria-pressed={liked} onClick={() => onLike(currentTrack)} className={`grid h-12 w-12 place-items-center rounded-full border transition ${liked ? 'border-danger/45 bg-danger/15 text-danger' : 'border-white/10 text-muted hover:bg-white/8'}`}><Heart size={21} fill={liked ? 'currentColor' : 'none'} /></button>
            <button aria-label={downloaded ? 'Уже скачано' : 'Скачать'} disabled={!currentTrack.policy.download_allowed || downloaded || createDownload.isPending} onClick={download} className={`grid h-12 w-12 place-items-center rounded-full border transition disabled:opacity-50 ${downloaded ? 'border-lime-300/45 bg-lime-300/15 text-lime-300' : 'border-white/10 text-muted hover:bg-white/8'}`}>{createDownload.isPending ? <Loader2 className="animate-spin" size={21} /> : downloaded ? <Folder size={21} /> : <Download size={21} />}</button>
            <button aria-label="Shuffle" aria-pressed={shuffle} onClick={toggleShuffle} className={`grid h-12 w-12 place-items-center rounded-full border transition ${shuffle ? 'border-lime-300/45 bg-lime-300/15 text-lime-300' : 'border-white/10 text-muted hover:bg-white/8'}`}><Shuffle size={21} /></button>
            <button aria-label="Repeat" aria-pressed={activeRepeat} onClick={cycleRepeat} className={`grid h-12 w-12 place-items-center rounded-full border transition ${activeRepeat ? 'border-lime-300/45 bg-lime-300/15 text-lime-300' : 'border-white/10 text-muted hover:bg-white/8'}`}>{repeat === 'one' ? <Repeat1 size={21} /> : <Repeat size={21} />}</button>
          </div>
          <div className="mt-6 flex items-center justify-center gap-6">
            <button aria-label="Previous" onClick={previous} className="grid h-12 w-12 place-items-center rounded-full text-muted transition hover:bg-white/8"><SkipBack size={30} /></button>
            <button aria-label={status === 'playing' ? 'Pause' : 'Play'} onClick={playPause} className="grid h-16 w-16 place-items-center rounded-full bg-lime-300 text-black shadow-lg shadow-lime-300/20">{query.isFetching ? <Loader2 className="animate-spin" size={30} /> : status === 'playing' ? <Pause size={30} /> : <Play size={30} />}</button>
            <button aria-label="Next" onClick={next} className="grid h-12 w-12 place-items-center rounded-full text-muted transition hover:bg-white/8"><SkipForward size={30} /></button>
          </div>
          {queue.length > 1 && <section className="mt-8">
            <div className="mb-2 flex items-center justify-between px-1"><h3 className="t-eyebrow m-0">Дальше в очереди</h3><span className="font-mono text-xs text-muted">{queueIndex + 1}/{queue.length}</span></div>
            <div className="grid max-h-52 gap-1 overflow-y-auto overscroll-contain pr-1">
              {queue.map((track, index) => <button key={`${track.id}-${index}`} type="button" onClick={() => playQueue(queue, index)} className={`grid grid-cols-[44px_minmax(0,1fr)_auto] items-center gap-3 rounded-control px-1 py-2 text-left ${index === queueIndex ? 'bg-lime-300/[0.08]' : 'hover:bg-white/[0.03]'}`}>
                <span className="grid h-11 w-11 shrink-0 place-items-center overflow-hidden rounded-control"><TrackArt track={track} className="h-full w-full" /></span>
                <span className="min-w-0"><strong className="block truncate text-[13px] font-semibold">{track.title}</strong><span className="t-body-sm block truncate">{track.artist ?? 'Исполнитель не указан'}</span></span>
                {index === queueIndex && <span className="eq text-lime-300" aria-label="Сейчас играет"><span /><span /><span /></span>}
              </button>)}
            </div>
          </section>}
        </div>
      </section>
    </div>}
  </>;
}

/** Everything the server exposes as changeable at runtime, so a self-hosted
 *  admin does not have to SSH in and edit .env for day-to-day settings.
 *  Values fixed at boot, and the yt-dlp path (which the server executes), are
 *  shown read-only — see settings.go for why. */
function ServerSettingsSection() {
  const settings = useSettings();
  const update = useUpdateSettings();
  const [status, setStatus] = useState<string | null>(null);
  const data = settings.data;
  const [draft, setDraft] = useState<ServerSettingsPatch>({});
  const value = <K extends keyof ServerSettingsPatch>(key: K, fallback: ServerSettingsPatch[K]) => (draft[key] !== undefined ? draft[key] : fallback);
  const set = <K extends keyof ServerSettingsPatch>(key: K, next: ServerSettingsPatch[K]) => setDraft((current) => ({ ...current, [key]: next }));

  const save = async () => {
    setStatus(null);
    try {
      await update.mutateAsync(draft);
      setDraft({});
      setStatus('Настройки применены — перезапуск не нужен.');
    } catch (error) {
      setStatus(error instanceof Error ? error.message : 'Не удалось сохранить');
    }
  };

  if (settings.isLoading) return <section className="settings-section"><SpinnerLine label="Загружаю настройки сервера…" /></section>;
  if (!data) return null;
  const dirty = Object.keys(draft).length > 0;

  return <section className="settings-section">
    <p className="t-eyebrow m-0 text-lime-300">Сервер</p>
    <p className="t-body-sm m-0 mt-2">Меняются на лету и переживают перезапуск — правку <code className="font-mono text-xs">.env</code> они не требуют.</p>

    <div className="mt-5 grid gap-3">
      <label className="flex items-center justify-between gap-4 rounded-xl border border-white/10 bg-surface p-4">
        <span className="min-w-0">
          <strong className="block">Источники YouTube и SoundCloud</strong>
          <small className="mt-1 block text-[#8c919e]">Нужны установленные <code className="font-mono">yt-dlp</code> и <code className="font-mono">ffmpeg</code>. Только для личного self-hosted использования.</small>
        </span>
        <button type="button" role="switch" aria-checked={Boolean(value('enable_risky_extractors', data.enable_risky_extractors))} onClick={() => set('enable_risky_extractors', !value('enable_risky_extractors', data.enable_risky_extractors))} className={`source-switch shrink-0 ${value('enable_risky_extractors', data.enable_risky_extractors) ? 'source-switch--on' : ''}`}><span /></button>
      </label>

      <div className="grid gap-3 md:grid-cols-2">
        <NumberField label="Таймаут поиска, сек" hint="5–600" value={Number(value('extractor_timeout_seconds', data.extractor_timeout_seconds))} onChange={(n) => set('extractor_timeout_seconds', n)} />
        <NumberField label="Таймаут загрузки, сек" hint="30–7200" value={Number(value('download_timeout_seconds', data.download_timeout_seconds))} onChange={(n) => set('download_timeout_seconds', n)} />
        <NumberField label="Время жизни сессии, часов" hint="1–8760" value={Number(value('session_ttl_hours', data.session_ttl_hours))} onChange={(n) => set('session_ttl_hours', n)} />
        <TextField label="Публичный адрес медиа" placeholder="Пусто = отдавать с этого сервера" value={String(value('public_media_base_url', data.public_media_base_url) ?? '')} onChange={(v) => set('public_media_base_url', v)} />
      </div>

      <TextField label="Разрешённые Origin (CORS), через запятую" value={(value('cors_origins', data.cors_origins) as string[] ?? []).join(', ')} onChange={(v) => set('cors_origins', v.split(',').map((x) => x.trim()).filter(Boolean))} />

      <label className="flex items-center justify-between gap-4 rounded-xl border border-white/10 bg-surface p-4">
        <span className="min-w-0">
          <strong className="block">Secure-флаг у cookie</strong>
          <small className="mt-1 block text-[#8c919e]">Включай, когда сервер работает по HTTPS. По HTTP вход перестанет работать.</small>
        </span>
        <button type="button" role="switch" aria-checked={Boolean(value('secure_cookies', data.secure_cookies))} onClick={() => set('secure_cookies', !value('secure_cookies', data.secure_cookies))} className={`source-switch shrink-0 ${value('secure_cookies', data.secure_cookies) ? 'source-switch--on' : ''}`}><span /></button>
      </label>

      <div className="grid gap-3 md:grid-cols-2">
        <SecretField label="YouTube API key" isSet={data.youtube_api_key_set} onChange={(v) => set('youtube_api_key', v)} />
        <SecretField label="SoundCloud client ID" isSet={data.soundcloud_client_id_set} onChange={(v) => set('soundcloud_client_id', v)} />
        <TextField label="Navidrome — адрес" value={String(value('navidrome_base_url', data.navidrome_base_url) ?? '')} onChange={(v) => set('navidrome_base_url', v)} />
        <TextField label="Navidrome — пользователь" value={String(value('navidrome_username', data.navidrome_username) ?? '')} onChange={(v) => set('navidrome_username', v)} />
        <SecretField label="Navidrome — токен" isSet={data.navidrome_token_set} onChange={(v) => set('navidrome_token', v)} />
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <button type="button" onClick={save} disabled={!dirty || update.isPending} className="rounded-xl bg-lime-300 px-4 py-3 text-sm font-medium text-black disabled:opacity-50">{update.isPending ? 'Сохраняю…' : 'Применить'}</button>
        {dirty && <button type="button" onClick={() => setDraft({})} className="rounded-xl border border-white/10 px-4 py-3 text-sm text-[#a6abb7] hover:bg-white/8">Отменить</button>}
        {status && <span className="text-sm text-[#a6abb7]">{status}</span>}
      </div>

      <details className="rounded-xl border border-white/10 bg-surface p-4">
        <summary className="cursor-pointer select-none text-sm text-[#a6abb7]">Задано при запуске — только чтение</summary>
        <p className="m-0 mt-3 text-xs text-[#8c919e]">{data.read_only.reason}</p>
        <dl className="mt-3 grid gap-2 text-sm">
          {([['Адрес и порт', data.read_only.addr], ['Окружение', data.read_only.environment], ['Файл данных', data.read_only.store_path], ['Папка медиа', data.read_only.media_root], ['Папка веб-клиента', data.read_only.web_root], ['Путь к yt-dlp', data.read_only.yt_dlp_binary]] as const).map(([label, v]) => (
            <div key={label} className="flex flex-wrap items-baseline justify-between gap-2 border-b border-white/8 pb-2 last:border-b-0">
              <dt className="text-[#8c919e]">{label}</dt>
              <dd className="m-0 font-mono text-xs break-all">{v || '—'}</dd>
            </div>
          ))}
        </dl>
      </details>
    </div>
  </section>;
}

function TextField({ label, value, onChange, placeholder }: { label: string; value: string; onChange: (v: string) => void; placeholder?: string }) {
  return <label className="grid gap-2 text-sm text-[#a6abb7]">{label}
    <input value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} className="rounded-xl border border-white/10 bg-surface-2 px-4 py-3 text-white outline-none placeholder:text-[#626875]" />
  </label>;
}

function NumberField({ label, value, onChange, hint }: { label: string; value: number; onChange: (v: number) => void; hint?: string }) {
  return <label className="grid gap-2 text-sm text-[#a6abb7]">{label}{hint && <span className="text-xs text-[#626875]">{hint}</span>}
    <input type="number" value={value} onChange={(e) => onChange(Number(e.target.value))} className="rounded-xl border border-white/10 bg-surface-2 px-4 py-3 text-white tabular-nums outline-none" />
  </label>;
}

/** The server never sends a secret back, so the field starts empty and an
 *  empty value means "leave it alone" — saving other settings must not wipe a
 *  working key. */
function SecretField({ label, isSet, onChange }: { label: string; isSet: boolean; onChange: (v: string) => void }) {
  return <label className="grid gap-2 text-sm text-[#a6abb7]">{label}
    <input type="password" placeholder={isSet ? 'Задан — оставь пустым, чтобы не менять' : 'Не задан'} onChange={(e) => onChange(e.target.value)} className="rounded-xl border border-white/10 bg-surface-2 px-4 py-3 text-white outline-none placeholder:text-[#626875]" />
  </label>;
}

