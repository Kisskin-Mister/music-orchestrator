import { FormEvent, MouseEvent, ReactNode, TouchEvent, useEffect, useMemo, useRef, useState } from 'react';
import { Cloud, Download, Folder, Heart, Library, ListMusic, Loader2, LogOut, MoreHorizontal, Music2, Pause, Pencil, Play, Plus, Repeat, Repeat1, Search, SearchX, Settings, ShieldCheck, Shuffle, SkipBack, SkipForward, Trash2, X, Youtube } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { useAddFavorite, useAddPlaylistTrack, useCreateDownload, useCreatePlaylist, useDeleteDownload, useDeleteFavorite, useDeletePlaylist, useDownloads, useFavorites, usePlayback, usePlaylists, useProviders, useRemovePlaylistTrack, useSearch, useSession, useUpdateAccount, useCreateUser, useDeleteUser, useUpdatePlaylist, useUpdateUser, useUsers } from '@/api/queries';
import { usePlayerStore } from '@/store/player';
import { analytics } from '@/lib/analytics';
import { getBackendBaseURL, getDefaultBackendBaseURL, setBackendBaseURL } from '@/api/client';
import type { Job, Playlist, Provider, ProviderId, Track, User } from '@/api/types';

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

function SourceIcon({ id, className = '' }: { id: string; className?: string }) {
  if (id.includes('youtube')) return <Youtube className={className} aria-label="YouTube" />;
  if (id.includes('soundcloud')) return <Cloud className={className} aria-label="SoundCloud" />;
  if (id.includes('local')) return <Folder className={className} aria-label="Local" />;
  return <Music2 className={className} aria-label="Source" />;
}
function sourceName(id: string) { if (id.includes('youtube')) return 'YouTube'; if (id.includes('soundcloud')) return 'SoundCloud'; if (id === 'local') return 'Local'; return id; }
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

  useEffect(() => {
    if (activeView !== 'search') return;
    const next = draft.trim();
    if (next === query) return;
    const timer = window.setTimeout(() => { setSearchLimit(PAGE_SIZE); setQuery(next); }, 350);
    return () => window.clearTimeout(timer);
  }, [draft, query, activeView]);

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
  const toggleProvider = (id: ProviderId) => setSelectedProviders((current) => current.includes(id) ? current.filter((p) => p !== id) : [...current, id]);

  return <main className="mx-auto grid min-h-screen max-w-5xl gap-4 px-3 py-4 pb-[calc(10rem+env(safe-area-inset-bottom,0px))] lg:grid-cols-[76px_minmax(0,1fr)] lg:px-5">
    <aside className="fixed inset-x-3 bottom-[calc(0.5rem+env(safe-area-inset-bottom,0px))] z-30 rounded-3xl border border-white/10 bg-[#101217]/95 p-2 shadow-2xl shadow-black/40 backdrop-blur lg:static lg:inset-auto lg:block lg:h-fit lg:p-3">
      <div className="hidden place-items-center rounded-2xl bg-lime-300 py-3 text-black lg:grid">≋</div>
      <nav className="grid grid-cols-5 gap-1 lg:mt-4 lg:grid-cols-1">{NAV.map(({ id, label, icon: Icon }) => <button key={id} type="button" onClick={() => setActiveView(id)} className={`grid place-items-center gap-1 rounded-2xl px-2 py-2 transition active:scale-[.96] lg:gap-0 lg:py-3 ${activeView === id ? 'bg-lime-300 text-black' : 'text-[#9aa0ad] hover:bg-white/8 hover:text-white'}`} aria-label={label} aria-current={activeView === id ? 'page' : undefined} title={label}><Icon size={20} /><span className="text-[10px] leading-none lg:hidden">{label}</span></button>)}</nav>
    </aside>
    <section className="min-w-0"><div key={activeView} className="view-enter">
      {activeView === 'library' && <LibraryView tracks={filteredLibrary} filter={libraryFilter} setFilter={setLibraryFilter} {...shared} />}
      {activeView === 'search' && <SearchView draft={draft} setDraft={setDraft} submit={submit} clearSearch={clearSearch} query={query} resultQuery={result.data?.query ?? ''} providers={providers.data ?? []} selectedProviders={selectedProviders} toggleProvider={toggleProvider} isLoading={result.isLoading} isFetching={result.isFetching} isError={result.isError} total={result.data?.total ?? 0} limit={searchLimit} more={() => setSearchLimit((n) => n + PAGE_SIZE)} tracks={result.data?.items ?? []} {...shared} />}
      {activeView === 'playlists' && <PlaylistsView libraryTracks={libraryTracks} {...shared} />}
      {activeView === 'downloads' && <DownloadsView jobs={downloadedJobs.filter((j) => j.track_id)} filter={downloadsFilter} setFilter={setDownloadsFilter} {...shared} />}
      {activeView === 'settings' && <SettingsView providers={providers.data ?? []} riskyEnabled={riskyEnabled} selectedProviders={selectedProviders} toggleProvider={toggleProvider} onLogout={onLogout} localMode={localMode} onLeaveLocalMode={onLeaveLocalMode} />}
    </div></section>
    <PlayerBar favoriteIDs={favoriteIDs} downloadedByTrack={downloadedByTrack} onLike={shared.onLike} />
  </main>;
}

function SectionHeader({ eyebrow, title, subtitle, children }: { eyebrow: string; title: string; subtitle: string; children?: ReactNode }) { return <header className="mb-5 rounded-3xl border border-white/10 bg-white/[0.035] p-5"><p className="mb-2 font-mono text-[11px] uppercase tracking-[.12em] text-lime-300">{eyebrow}</p><h1 className="m-0 text-3xl font-semibold tracking-[-.04em] md:text-5xl">{title}</h1><p className="mb-0 mt-2 max-w-2xl text-sm text-[#a6abb7] md:text-base">{subtitle}</p>{children && <div className="mt-4">{children}</div>}</header>; }
function InlineFilter({ value, onChange, placeholder }: { value: string; onChange: (value: string) => void; placeholder: string }) { return <div className="flex h-12 items-center gap-3 rounded-2xl border border-white/15 bg-[#101217] px-4 transition focus-within:border-lime-300/60"><Search size={18} className="text-[#8c919e]" /><input value={value} onChange={(e) => onChange(e.target.value)} className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-[#626875]" placeholder={placeholder} /></div>; }
function SpinnerLine({ label }: { label: string }) { return <div className="flex items-center gap-2 rounded-2xl border border-white/10 bg-white/[0.03] p-4 text-[#9aa0ad]"><Loader2 className="animate-spin text-lime-300" size={18} /> {label}</div>; }
function EmptyState({ icon: Icon, title, hint }: { icon: LucideIcon; title: string; hint: string }) { return <div className="grid place-items-center gap-3 rounded-3xl border border-dashed border-white/12 bg-white/[0.02] px-6 py-12 text-center"><span className="grid h-14 w-14 place-items-center rounded-2xl bg-white/[0.05] text-[#8c919e]"><Icon size={24} /></span><div><h2 className="m-0 text-lg font-semibold">{title}</h2><p className="m-0 mt-1 max-w-sm text-sm text-[#8c919e]">{hint}</p></div></div>; }

function SearchView({ draft, setDraft, submit, clearSearch, query, resultQuery, providers, selectedProviders, toggleProvider, isLoading, isFetching, isError, total, limit, more, tracks, playlists, favoriteIDs, downloadedByTrack, onLike, onPlay }: { draft:string; setDraft:(v:string)=>void; submit:(e:FormEvent)=>void; clearSearch:()=>void; query:string; resultQuery:string; providers:Provider[]; selectedProviders:ProviderId[]; toggleProvider:(id:ProviderId)=>void; isLoading:boolean; isFetching:boolean; isError:boolean; total:number; limit:number; more:()=>void; tracks: Track[] } & TrackSurfaceProps) {
  const trimmedDraft = draft.trim();
  const waitingForDebounce = Boolean(trimmedDraft) && trimmedDraft !== query;
  const hasFreshResult = Boolean(query) && resultQuery === query;
  const visibleTracks = hasFreshResult ? tracks : [];
  const visibleTotal = hasFreshResult ? total : 0;
  const waitingForFreshResult = Boolean(query) && !hasFreshResult && !isError;
  const showInitialSpinner = Boolean(query) && (isLoading || isFetching || waitingForFreshResult) && !visibleTracks.length;
  const canShowEmpty = Boolean(query) && hasFreshResult && !waitingForDebounce && !isFetching && !isLoading && !isError && !visibleTracks.length;
  return <>
    <SectionHeader eyebrow="Поиск" title="Что послушаем?" subtitle="Начни вводить название или исполнителя — результаты подтянутся сами, Enter запускает поиск сразу.">
      <form onSubmit={submit} className="mb-3 flex h-13 items-center gap-3 rounded-2xl border border-white/15 bg-[#101217] px-4 transition focus-within:border-lime-300/60">
        <Search size={19} className="text-[#8c919e]" />
        <input value={draft} onChange={(e) => setDraft(e.target.value)} className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-[#626875]" placeholder="Название, исполнитель или альбом" />
        {draft && <button type="button" onClick={clearSearch} aria-label="Очистить поиск" className="rounded-xl p-2 text-[#8c919e] hover:bg-white/8"><X size={17} /></button>}
        <button type="submit" aria-label="Найти" className="rounded-xl bg-lime-300 px-3 py-2 text-sm font-medium text-black disabled:opacity-50" disabled={!draft.trim()}>{isFetching ? <Loader2 className="animate-spin" size={16} /> : 'Найти'}</button>
      </form>
      <div className="flex flex-wrap gap-2">{providers.filter((p) => p.enabled).map((provider) => { const selected = selectedProviders.includes(provider.id); return <button key={provider.id} type="button" onClick={() => toggleProvider(provider.id)} aria-pressed={selected} className={`inline-flex min-h-11 items-center gap-2 rounded-2xl border px-3 py-2 transition ${selected ? 'border-lime-300/60 bg-lime-300/[0.12] text-lime-50 ring-1 ring-lime-300/50' : 'border-white/10 bg-white/[0.035] text-[#9aa0ad] opacity-70 hover:opacity-100'}`} title={providerName(provider)} aria-label={providerName(provider)}><SourceIcon id={provider.id} className="h-5 w-5" /><span className="text-sm">{providerName(provider)}</span></button>; })}</div>
    </SectionHeader>
    {!query && <EmptyState icon={Search} title="Найди свой трек" hint="Введи название песни, исполнителя или альбома — соберём результаты из подключённых источников." />}
    {waitingForDebounce && <SpinnerLine label="Готовлю новый поиск…" />}
    {showInitialSpinner && !waitingForDebounce && <SpinnerLine label="Ищу треки…" />}
    {query && isError && !waitingForDebounce && <section role="alert" className="rounded-2xl border border-red-300/30 bg-red-300/[0.06] p-5"><h2 className="m-0 text-lg">Поиск не удался</h2><p className="mb-0 text-[#b9bec9]">Источник не ответил вовремя. Попробуй ещё раз или сократи запрос.</p></section>}
    {canShowEmpty && <EmptyState icon={SearchX} title={`По запросу «${query}» ничего нет`} hint="Попробуй написать короче или другими словами — или выбери другой источник выше." />}
    {Boolean(visibleTracks.length) && <><TrackList tracks={visibleTracks} playlists={playlists} favoriteIDs={favoriteIDs} downloadedByTrack={downloadedByTrack} onLike={onLike} onPlay={onPlay} />{isFetching && <div className="mt-3"><SpinnerLine label="Подгружаю…" /></div>}{visibleTracks.length < visibleTotal && <button onClick={more} disabled={isFetching} className="mt-4 inline-flex items-center gap-2 rounded-2xl border border-white/10 bg-white/[0.04] px-4 py-3 text-sm hover:bg-white/8 disabled:opacity-50">{isFetching ? <Loader2 className="animate-spin" size={16} /> : <Plus size={16} />} Ещё 20</button>}<p className="mt-3 text-xs text-[#777]">Показано {Math.min(limit, visibleTracks.length)} из {visibleTotal}</p></>}
  </>;
}

function TrackRow({ track, playlists, liked, cachedJob, onLike, onPlay }: { track: Track; playlists: Playlist[]; liked: boolean; cachedJob?: Job; onLike: () => void; onPlay: () => void }) {
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
  return <article aria-current={isCurrent ? 'true' : undefined} className={`grid min-h-[76px] grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-b border-white/10 px-3 py-2.5 transition-colors last:border-b-0 hover:bg-white/[0.03] md:px-4 ${isCurrent ? 'bg-lime-300/[0.07] hover:bg-lime-300/[0.09]' : ''}`}>
    <button type="button" onClick={onPlay} className="flex min-w-0 items-center gap-3 text-left outline-none ring-lime-300/70 focus-visible:ring-2">
      <span className="group relative grid h-13 w-13 shrink-0 place-items-center overflow-hidden rounded-2xl bg-white/[0.06] text-[#6f7480] transition hover:ring-2">{track.artwork_url ? <img alt="" src={track.artwork_url} className="h-full w-full object-cover transition group-hover:scale-105" /> : <Music2 size={22} />}<span className={`absolute inset-0 grid place-items-center text-white transition ${isCurrent ? 'bg-black/45 opacity-100' : 'bg-black/0 opacity-0 group-hover:bg-black/35 group-hover:opacity-100'}`}>{isCurrent ? <span className={`eq ${playerStatus === 'playing' ? '' : 'eq--paused'}`} aria-label="Сейчас играет"><span /><span /><span /></span> : <Play size={18} />}</span></span>
      <span className="min-w-0"><span className="mb-1 flex min-w-0 items-center gap-2"><strong className="truncate">{track.title}</strong>{track.official && <ShieldCheck size={15} className="shrink-0 text-sky-300" aria-label="Official Topic" />}{downloaded && <Folder size={15} className="shrink-0 text-lime-300" aria-label="Локально" />}</span><span className="m-0 flex min-w-0 items-center gap-2 text-sm text-[#8c919e]"><SourceIcon id={track.provider_id} className="h-4 w-4 shrink-0" /><span className="truncate">{track.artist || 'Исполнитель не указан'}</span><span className="shrink-0">· {formatDuration(liveDuration ?? track.duration_seconds)}</span></span>{job?.error && <span className="m-0 mt-1 block text-xs text-red-200">{job.error}</span>}</span>
    </button>
    <div className="relative flex gap-1">
      <button aria-label={liked ? 'Убрать лайк' : 'Поставить лайк'} onClick={onLike} className={`grid h-10 w-10 place-items-center rounded-xl hover:bg-white/8 ${liked ? 'text-red-300' : ''}`}><Heart size={18} fill={liked ? 'currentColor' : 'none'} /></button>
      <button aria-label="Действия" onClick={() => setMenuOpen((v) => !v)} aria-haspopup="menu" aria-expanded={menuOpen} className="grid h-10 w-10 place-items-center rounded-xl hover:bg-white/8"><MoreHorizontal size={19} /></button>
      {menuOpen && <><button type="button" aria-label="Закрыть меню" tabIndex={-1} onClick={() => setMenuOpen(false)} className="fixed inset-0 z-30 cursor-default" /><div role="menu" onKeyDown={(event) => { if (event.key === 'Escape') setMenuOpen(false); }} className="absolute right-0 top-10 z-40 w-64 overflow-hidden rounded-2xl border border-white/10 bg-[#151821] p-1 shadow-2xl shadow-black/50">
        {track.policy.download_allowed && <button onClick={download} disabled={createDownload.isPending || downloaded} className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8 disabled:opacity-50">{createDownload.isPending ? <Loader2 className="animate-spin" size={16} /> : <Download size={16} />} {downloaded ? 'Уже скачано' : 'Скачать mp3'}</button>}
        {downloaded && <button onClick={remove} disabled={deleteDownload.isPending} className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm text-red-200 hover:bg-red-300/10"><Trash2 size={16} /> Удалить локально</button>}
        <button role="menuitem" onClick={() => { playNext(track); setMenuOpen(false); }} className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm text-[#a6abb7] hover:bg-white/8"><SkipForward size={16} /> Слушать следующим</button><button role="menuitem" onClick={() => { addToQueue(track); setMenuOpen(false); }} className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm text-[#a6abb7] hover:bg-white/8"><ListMusic size={16} /> В конец очереди</button><button role="menuitem" onClick={() => setPlaylistPicker((v) => !v)} className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm text-[#a6abb7] hover:bg-white/8"><Plus size={16} /> Добавить в плейлист</button>
        {playlistPicker && <div className="mt-1 border-t border-white/10 p-2">
          <div className="grid gap-1">{playlists.map((playlist) => <button key={playlist.id} onClick={() => addToPlaylist(playlist.id)} className="flex w-full items-center justify-between rounded-xl px-2 py-2 text-left text-sm hover:bg-white/8"><span className="truncate">{playlist.name}</span><small className="text-[#8c919e]">{playlist.track_count}</small></button>)}</div>
          <div className="mt-2 flex gap-2"><input value={newPlaylistName} onChange={(e) => setNewPlaylistName(e.target.value)} placeholder="Новый" className="min-w-0 flex-1 rounded-xl border border-white/10 bg-black/25 px-3 py-2 text-sm outline-none" /><button onClick={createAndAdd} disabled={!newPlaylistName.trim() || createPlaylist.isPending || addPlaylistTrack.isPending} className="rounded-xl bg-lime-300 px-3 py-2 text-sm text-black disabled:opacity-50">+</button></div>
        </div>}
      </div></>}
    </div>
  </article>;
}

function TrackList({ tracks, playlists, favoriteIDs, downloadedByTrack, onLike, onPlay }: { tracks: Track[] } & TrackSurfaceProps) { return <div className="overflow-hidden rounded-3xl border border-white/10 bg-white/[0.025]">{tracks.map((track, index) => <TrackRow key={track.id} track={track} playlists={playlists} liked={favoriteIDs.has(track.id)} cachedJob={downloadedByTrack.get(track.id)} onLike={() => onLike(track)} onPlay={() => onPlay(track, tracks, index)} />)}</div>; }
function LibraryView({ tracks, filter, setFilter, playlists, favoriteIDs, downloadedByTrack, onLike, onPlay }: { tracks: Track[]; filter:string; setFilter:(value:string)=>void } & TrackSurfaceProps) { return <><SectionHeader eyebrow="Коллекция" title="Медиатека" subtitle="Всё, что ты лайкнул или скачал, — в одном месте."><InlineFilter value={filter} onChange={setFilter} placeholder="Искать в медиатеке" /></SectionHeader>{tracks.length ? <TrackList tracks={tracks} playlists={playlists} favoriteIDs={favoriteIDs} downloadedByTrack={downloadedByTrack} onLike={onLike} onPlay={onPlay} /> : <EmptyState icon={Heart} title="Пока пусто" hint="Лайкни трек или скачай его — и он появится здесь." />}</>; }
function DownloadsView({ jobs, filter, setFilter, playlists, favoriteIDs, downloadedByTrack, onLike, onPlay }: { jobs: Job[]; filter:string; setFilter:(value:string)=>void } & TrackSurfaceProps) { const tracks = filterTracks(jobs.map((job) => { const track = trackFromJob(job); return track ? { ...track, downloaded: true, download_media_url: getMediaURL(job) ?? track.download_media_url } : null; }).filter(Boolean) as Track[], filter); return <><SectionHeader eyebrow="Offline" title="Загрузки" subtitle="Треки, сохранённые на устройство, — слушай без сети."><InlineFilter value={filter} onChange={setFilter} placeholder="Искать в загрузках" /></SectionHeader>{tracks.length ? <TrackList tracks={tracks} playlists={playlists} favoriteIDs={favoriteIDs} downloadedByTrack={downloadedByTrack} onLike={onLike} onPlay={onPlay} /> : <EmptyState icon={Download} title="Пока ничего не скачано" hint="Скачанные треки появятся здесь — открой меню трека и выбери «Скачать mp3»." />}</>; }

function PlaylistsView({ playlists, libraryTracks, favoriteIDs, downloadedByTrack, onLike, onPlay }: TrackSurfaceProps & { libraryTracks: Track[] }) {
  const createPlaylist = useCreatePlaylist();
  const addPlaylistTrack = useAddPlaylistTrack();
  const [creating, setCreating] = useState(false);
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
    <SectionHeader eyebrow="Коллекции" title="Плейлисты" subtitle="Собирай подборки под настроение: название, описание и треки выбираются сразу при создании.">
      <button type="button" onClick={() => setCreating(true)} className="inline-flex items-center gap-2 rounded-2xl bg-lime-300 px-4 py-3 font-medium text-black shadow-lg shadow-lime-300/10"><Plus size={17} /> Создать плейлист</button>
    </SectionHeader>
    {creating && <form onSubmit={create} className="mb-5 overflow-hidden rounded-[2rem] border border-lime-300/20 bg-gradient-to-br from-lime-300/[0.12] via-white/[0.04] to-black/20 shadow-2xl shadow-black/20">
      <div className="border-b border-white/10 p-5">
        <div className="flex items-start justify-between gap-3"><div><p className="m-0 font-mono text-[11px] uppercase tracking-[.12em] text-lime-300">Новый плейлист</p><h2 className="m-0 mt-1 text-2xl font-semibold">Создать подборку</h2><p className="m-0 mt-1 text-sm text-[#a6abb7]">Сначала опиши вайб, потом выбери треки из медиатеки.</p></div><button type="button" onClick={resetCreateForm} aria-label="Закрыть создание плейлиста" className="rounded-2xl p-2 text-[#a6abb7] hover:bg-white/8"><X size={19} /></button></div>
        <div className="mt-5 grid gap-3 md:grid-cols-2">
          <label className="grid gap-2 text-sm text-lime-100">Название плейлиста<input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Например: Ночной вайб" className="rounded-2xl border border-white/10 bg-black/35 px-4 py-3 text-white outline-none placeholder:text-[#626875]" /></label>
          <label className="grid gap-2 text-sm text-lime-100">Описание плейлиста<input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Коротко: настроение, жанр, для чего" className="rounded-2xl border border-white/10 bg-black/35 px-4 py-3 text-white outline-none placeholder:text-[#626875]" /></label>
        </div>
      </div>
      <div className="grid gap-4 p-5 lg:grid-cols-[minmax(0,1fr)_260px]">
        <section>
          <div className="mb-3 flex items-center justify-between gap-3"><div><h3 className="m-0 text-base font-semibold">Добавить треки из медиатеки</h3><p className="m-0 mt-1 text-xs text-[#8c919e]">Доступны лайкнутые и скачанные треки.</p></div><span className="rounded-full border border-white/10 px-3 py-1 text-xs text-[#a6abb7]">{selectedTrackIDs.size} выбрано</span></div>
          {libraryTracks.length ? <div className="grid max-h-80 gap-2 overflow-y-auto overscroll-contain pr-1">{libraryTracks.map((track) => {
            const checked = selectedTrackIDs.has(track.id);
            return <label key={track.id} className={`grid cursor-pointer grid-cols-[auto_42px_minmax(0,1fr)] items-center gap-3 rounded-2xl border px-3 py-2 transition ${checked ? 'border-lime-300/40 bg-lime-300/[0.10]' : 'border-white/10 bg-black/20 hover:bg-white/8'}`}>
              <input type="checkbox" checked={checked} onChange={() => toggleSelectedTrack(track.id)} aria-label={`Добавить ${track.title}`} className="h-5 w-5 accent-lime-300" />
              <span className="grid h-10 w-10 place-items-center overflow-hidden rounded-xl bg-white/8">{track.artwork_url ? <img alt="" src={track.artwork_url} className="h-full w-full object-cover" /> : <SourceIcon id={track.provider_id} className="h-4 w-4" />}</span>
              <span className="min-w-0"><strong className="block truncate text-sm">{track.title}</strong><small className="block truncate text-[#8c919e]">{track.artist ?? 'Исполнитель не указан'}</small></span>
            </label>;
          })}</div> : <div className="rounded-3xl border border-white/10 bg-black/20 p-5 text-sm text-[#a6abb7]">Медиатека пустая. Лайкни или скачай треки — потом их можно будет добавить сюда при создании.</div>}
        </section>
        <aside className="rounded-3xl border border-white/10 bg-black/25 p-4">
          <h3 className="m-0 text-base font-semibold">Превью</h3>
          <p className="m-0 mt-1 text-sm text-[#8c919e]">{name.trim() || 'Без названия'}</p>
          <p className="m-0 mt-3 text-xs text-[#8c919e]">{description.trim() || 'Описание можно оставить пустым.'}</p>
          <div className="mt-4 grid gap-2">{selectedTracks.slice(0, 4).map((track) => <div key={track.id} className="truncate rounded-xl bg-white/[0.06] px-3 py-2 text-sm">{track.title}</div>)}{selectedTracks.length > 4 && <div className="text-xs text-[#8c919e]">+ ещё {selectedTracks.length - 4}</div>}</div>
          <button type="submit" disabled={!name.trim() || createPlaylist.isPending || addPlaylistTrack.isPending} className="mt-5 w-full rounded-2xl bg-lime-300 px-4 py-3 font-medium text-black disabled:opacity-50">{createPlaylist.isPending || addPlaylistTrack.isPending ? 'Создаю…' : 'Создать'}</button>
        </aside>
      </div>
    </form>}
    {playlists.length ? <div className="grid gap-4">{playlists.map((playlist) => <PlaylistCard key={playlist.id} playlist={playlist} playlists={playlists} favoriteIDs={favoriteIDs} downloadedByTrack={downloadedByTrack} onLike={onLike} onPlay={onPlay} />)}</div> : !creating && <EmptyState icon={ListMusic} title="Плейлистов пока нет" hint="Нажми «Создать плейлист», задай название и выбери треки из медиатеки." />}
  </>;
}

function PlaylistCard({ playlist, playlists, favoriteIDs, downloadedByTrack, onLike, onPlay }: { playlist: Playlist } & TrackSurfaceProps) {
  void playlists; void downloadedByTrack;
  const updatePlaylist = useUpdatePlaylist();
  const deletePlaylist = useDeletePlaylist();
  const removePlaylistTrack = useRemovePlaylistTrack();
  const [editing, setEditing] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [armedTrackID, setArmedTrackID] = useState<string | null>(null);
  const [name, setName] = useState(playlist.name);
  const [description, setDescription] = useState(playlist.description ?? '');
  const tracks = playlist.tracks.map((item) => item.track).filter(Boolean) as Track[];
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
  return <section className="overflow-hidden rounded-[1.75rem] border border-white/10 bg-white/[0.035] shadow-[0_18px_70px_rgba(0,0,0,.28)]">
    <div className="grid gap-4 border-b border-white/10 p-4 sm:grid-cols-[auto_minmax(0,1fr)_auto] sm:items-center">
      <button type="button" onClick={() => tracks.length && onPlay(tracks[0], tracks, 0)} disabled={!tracks.length} aria-label="Воспроизвести плейлист" className="group grid grid-cols-[64px_minmax(0,1fr)] items-center gap-3 text-left sm:contents disabled:cursor-default">
        <span className="relative grid h-16 w-16 shrink-0 place-items-center overflow-hidden rounded-2xl bg-lime-300/15 text-lime-100 sm:row-span-2">{playlist.cover_url ? <img alt="" src={playlist.cover_url} className="h-full w-full object-cover" /> : <ListMusic size={24} />}<span className="absolute inset-0 grid place-items-center bg-black/35 opacity-0 transition group-enabled:group-hover:opacity-100"><Play size={20} /></span></span>
        {!editing && <span className="min-w-0 sm:hidden"><strong className="block truncate text-lg">{playlist.name}</strong><small className="mt-1 block truncate text-[#8c919e]">{playlist.track_count} · {formatDuration(playlist.duration_seconds)}</small></span>}
      </button>
      {editing ? <form onSubmit={save} className="min-w-0 sm:col-span-2">
        <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]"><input value={name} onChange={(e) => setName(e.target.value)} className="rounded-2xl border border-white/10 bg-black/30 px-4 py-3 outline-none" placeholder="Название плейлиста" /><input value={description} onChange={(e) => setDescription(e.target.value)} className="rounded-2xl border border-white/10 bg-black/30 px-4 py-3 outline-none" placeholder="Описание" /></div>
        <div className="mt-3 flex flex-wrap gap-2"><button type="submit" disabled={!name.trim() || updatePlaylist.isPending} className="rounded-2xl bg-lime-300 px-4 py-2 text-sm font-medium text-black disabled:opacity-50">Сохранить</button><button type="button" onClick={() => { setEditing(false); setName(playlist.name); setDescription(playlist.description ?? ''); }} className="rounded-2xl border border-white/10 px-4 py-2 text-sm text-[#a6abb7] hover:bg-white/8">Отмена</button></div>
      </form> : <div className="hidden min-w-0 sm:block"><h3 className="m-0 truncate text-xl font-semibold">{playlist.name}</h3>{playlist.description && <p className="m-0 mt-1 line-clamp-2 text-sm text-[#a6abb7]">{playlist.description}</p>}<p className="m-0 mt-1 text-sm text-[#8c919e]">{playlist.track_count} треков · {formatDuration(playlist.duration_seconds)}</p></div>}
      {!editing && <div className="flex items-center justify-end gap-2">
        {tracks.length > 0 && <button type="button" onClick={() => onPlay(tracks[0], tracks, 0)} className="grid h-12 w-12 place-items-center rounded-full bg-lime-300 text-black shadow-lg shadow-lime-300/15 transition hover:scale-[1.03] active:scale-[.97]" aria-label="Воспроизвести плейлист"><Play size={20} /></button>}
        <button type="button" onClick={() => setEditing(true)} className="grid h-11 w-11 place-items-center rounded-2xl border border-white/10 text-[#a6abb7] transition hover:bg-white/8" aria-label="Изменить плейлист" title="Изменить"><Pencil size={18} /></button>
        <button type="button" onClick={() => setConfirmDelete(true)} className="grid h-11 w-11 place-items-center rounded-2xl border border-red-300/20 text-red-200 transition hover:bg-red-300/10" aria-label="Удалить плейлист" title="Удалить"><Trash2 size={18} /></button>
      </div>}
      {confirmDelete && <div className="rounded-3xl border border-red-300/25 bg-red-300/[0.08] p-4 sm:col-span-3">
        <div className="flex items-start gap-3"><span className="grid h-10 w-10 shrink-0 place-items-center rounded-2xl bg-red-300/15 text-red-200"><Trash2 size={18} /></span><div className="min-w-0 flex-1"><p className="m-0 font-medium text-red-50">Удалить «{playlist.name}»?</p><p className="m-0 mt-1 text-sm text-red-100/75">Плейлист исчезнет из коллекций, треки останутся в медиатеке.</p></div></div>
        <div className="mt-4 grid grid-cols-2 gap-2"><button onClick={removePlaylist} disabled={deletePlaylist.isPending} className="rounded-2xl bg-red-300 px-4 py-3 font-medium text-black disabled:opacity-50">Удалить</button><button onClick={() => setConfirmDelete(false)} className="rounded-2xl border border-white/10 px-4 py-3 text-[#d7dbe4] hover:bg-white/8">Оставить</button></div>
      </div>}
    </div>
    {tracks.length ? <div className="overflow-hidden">{tracks.map((track, index) => <div key={track.id} className="grid min-h-[72px] grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-b border-white/10 px-4 py-2 last:border-b-0"><button type="button" onClick={() => onPlay(track, tracks, index)} className="flex min-w-0 items-center gap-3 text-left"><span className="grid h-11 w-11 shrink-0 place-items-center overflow-hidden rounded-2xl bg-white/[0.06]">{track.artwork_url ? <img alt="" src={track.artwork_url} className="h-full w-full object-cover" /> : <SourceIcon id={track.provider_id} className="h-4 w-4" />}</span><span className="min-w-0"><strong className="block truncate">{track.title}</strong><small className="block truncate text-[#8c919e]">{track.artist || 'Исполнитель не указан'} · {formatDuration(track.duration_seconds)}</small></span></button><div className="flex items-center gap-1"><button aria-label={favoriteIDs.has(track.id) ? 'Убрать лайк' : 'Поставить лайк'} onClick={() => onLike(track)} className={`grid h-10 w-10 place-items-center rounded-xl hover:bg-white/8 ${favoriteIDs.has(track.id) ? 'text-red-300' : ''}`}><Heart size={17} fill={favoriteIDs.has(track.id) ? 'currentColor' : 'none'} /></button><button type="button" onClick={() => removeTrack(track)} disabled={removePlaylistTrack.isPending} className={`grid h-10 w-10 place-items-center rounded-xl ${armedTrackID === track.id ? 'bg-red-300 text-black' : 'text-red-200 hover:bg-red-300/10'}`} aria-label={armedTrackID === track.id ? 'Подтвердить удаление трека из плейлиста' : 'Убрать трек из плейлиста'} title={armedTrackID === track.id ? 'Подтвердить' : 'Убрать'}><Trash2 size={17} /></button></div></div>)}</div> : <div className="p-5 text-sm text-[#9aa0ad]">Плейлист пустой. Открой меню любого трека и добавь его сюда.</div>}
  </section>;
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
  useEffect(() => { setAccountName(session.data?.username ?? ''); }, [session.data?.username]);
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
    {(onLogout || localMode) && <section className="mb-5 rounded-3xl border border-white/10 bg-white/[0.035] p-5">
      <p className="mb-2 font-mono text-[11px] uppercase tracking-[.12em] text-lime-300">Аккаунт</p>
      <div className="flex flex-wrap items-center justify-between gap-3"><p className="m-0 text-sm text-[#a6abb7]">{localMode ? 'Локальный режим: без аккаунта, данные живут на этом устройстве.' : `${session.data?.username ?? 'Аккаунт'} · ${session.data?.role === 'admin' ? 'администратор' : 'пользователь'}`}</p>{localMode ? <button type="button" onClick={onLeaveLocalMode} className="inline-flex items-center gap-2 rounded-2xl border border-white/10 px-4 py-3 text-sm text-[#c6cad3] transition hover:bg-white/8 active:scale-[0.99]"><LogOut size={16} /> Вернуться к экрану входа</button> : <button type="button" onClick={onLogout} className="inline-flex items-center gap-2 rounded-2xl border border-red-300/30 px-4 py-3 text-sm text-red-100 transition hover:bg-red-300/10 active:scale-[0.99]"><LogOut size={16} /> Выйти</button>}</div>
      {!localMode && session.data?.role === 'admin' && <div className="mt-5 grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <form onSubmit={saveAccount} className="rounded-3xl border border-white/10 bg-black/20 p-4"><h3 className="m-0 text-lg">Данные владельца</h3><p className="m-0 mt-1 text-sm text-[#8c919e]">Роль владельца: администратор.</p><div className="mt-4 grid gap-3"><input value={accountName} onChange={(e) => setAccountName(e.target.value)} className="rounded-2xl border border-white/10 bg-black/25 px-4 py-3 outline-none" placeholder="Логин" /><input type="password" value={accountPassword} onChange={(e) => setAccountPassword(e.target.value)} className="rounded-2xl border border-white/10 bg-black/25 px-4 py-3 outline-none" placeholder="Новый пароль — минимум 10 символов" /><button type="submit" disabled={!accountName.trim() || updateAccount.isPending} className="rounded-2xl bg-lime-300 px-4 py-3 font-medium text-black disabled:opacity-50">Сохранить аккаунт</button>{accountStatus && <p className="m-0 text-sm text-lime-200">{accountStatus}</p>}</div></form>
        <section className="rounded-3xl border border-white/10 bg-black/20 p-4"><h3 className="m-0 text-lg">Пользователи</h3><p className="m-0 mt-1 text-sm text-[#8c919e]">Новые пользователи получают роль «пользователь» и не видят админ-раздел.</p><form onSubmit={addUser} className="mt-4 grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]"><input value={newUserName} onChange={(e) => setNewUserName(e.target.value)} className="rounded-2xl border border-white/10 bg-black/25 px-4 py-3 outline-none" placeholder="Логин" /><input type="password" value={newUserPassword} onChange={(e) => setNewUserPassword(e.target.value)} className="rounded-2xl border border-white/10 bg-black/25 px-4 py-3 outline-none" placeholder="Пароль" /><button disabled={!newUserName.trim() || newUserPassword.length < 10 || createUser.isPending} className="rounded-2xl bg-lime-300 px-4 py-3 font-medium text-black disabled:opacity-50">Добавить</button></form><div className="mt-4 grid gap-2">{users.data?.map((user) => <UserRow key={user.id} user={user} onSave={(patch) => updateUser.mutate({ userId: user.id, patch })} onDelete={() => deleteUser.mutate(user.id)} />)}{!users.data?.length && <p className="m-0 rounded-2xl border border-white/10 bg-white/[0.03] p-3 text-sm text-[#8c919e]">Пользователей пока нет.</p>}</div></section>
      </div>}
    </section>}
    <section className="mb-5 rounded-3xl border border-white/10 bg-white/[0.035] p-5">
      <p className="mb-2 font-mono text-[11px] uppercase tracking-[.12em] text-lime-300">Источники поиска</p>
      <h3 className="m-0 text-xl font-semibold">Где искать музыку</h3>
      <p className="m-0 mt-1 text-sm text-[#8c919e]">Включи нужные источники — поиск сразу использует этот выбор.</p>
      <div className="mt-4 grid gap-3 md:grid-cols-2">{providers.filter((p) => p.capabilities.search || p.id === 'local').map((p) => {
        const selected = selectedProviders.includes(p.id);
        const disabled = !p.enabled || !p.capabilities.search;
        return <button key={p.id} type="button" disabled={disabled} onClick={() => toggleProvider(p.id)} aria-pressed={selected} className={`source-toggle ${selected && !disabled ? 'source-toggle--active' : ''}`}>
          <span className="flex min-w-0 items-center gap-3"><SourceIcon id={p.id} className="h-5 w-5 shrink-0" /><span className="min-w-0"><strong className="block truncate">{providerName(p)}</strong><small className="block text-[#8c919e]">{disabled ? 'Недоступен на сервере' : selected ? 'Включён' : 'Выключен'}</small></span></span>
          <span className={`source-switch ${selected && !disabled ? 'source-switch--on' : ''}`}><span /></span>
        </button>;
      })}</div>
    </section>
    <details className="mb-5 rounded-3xl border border-white/10 bg-white/[0.025] p-5 text-sm text-[#a6abb7]">
      <summary className="cursor-pointer select-none text-base font-medium text-[#d7dbe4]">Подключение к серверу</summary>
      <label className="mt-4 block text-sm text-[#a6abb7]" htmlFor="backend-url">Адрес API</label>
      <div className="mt-2 flex flex-col gap-2 md:flex-row">
        <input id="backend-url" value={backendDraft} onChange={(event) => setBackendDraft(event.target.value)} placeholder="Пусто = текущий домен" className="min-w-0 flex-1 rounded-2xl border border-white/10 bg-black/25 px-4 py-3 outline-none" />
        <button type="button" onClick={testBackend} className="rounded-2xl border border-white/10 px-4 py-3 text-sm hover:bg-white/8">Проверить</button>
        <button type="button" onClick={saveBackend} className="rounded-2xl bg-lime-300 px-4 py-3 text-sm font-medium text-black">Сохранить</button>
        <button type="button" onClick={resetBackend} className="rounded-2xl border border-white/10 px-4 py-3 text-sm text-[#a6abb7] hover:bg-white/8">Сброс</button>
      </div>
      <p className="m-0 mt-2 text-xs text-[#8c919e]">По умолчанию: {defaultBackend}. Настройка хранится в этом браузере.</p>
      {backendStatus && <p className="m-0 mt-3 rounded-2xl border border-white/10 bg-black/25 px-3 py-2 text-sm text-[#d7dbe4]">{backendStatus}</p>}
    </details>
  </>;
}

function UserRow({ user, onSave, onDelete }: { user: User; onSave: (patch: { username?: string; password?: string }) => void; onDelete: () => void }) {
  const [editing, setEditing] = useState(false);
  const [username, setUsername] = useState(user.username);
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState(false);
  useEffect(() => setUsername(user.username), [user.username]);
  return <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-3">{editing ? <form onSubmit={(event) => { event.preventDefault(); onSave({ username: username.trim(), password: password || undefined }); setPassword(''); setEditing(false); }} className="grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto_auto]"><input value={username} onChange={(e) => setUsername(e.target.value)} className="rounded-xl border border-white/10 bg-black/25 px-3 py-2 outline-none" /><input type="password" value={password} onChange={(e) => setPassword(e.target.value)} className="rounded-xl border border-white/10 bg-black/25 px-3 py-2 outline-none" placeholder="Новый пароль" /><button disabled={!username.trim()} className="rounded-xl bg-lime-300 px-3 py-2 text-black disabled:opacity-50">OK</button><button type="button" onClick={() => setEditing(false)} className="rounded-xl border border-white/10 px-3 py-2 text-[#a6abb7]">Отмена</button></form> : <div className="flex flex-wrap items-center justify-between gap-2"><span><strong>{user.username}</strong><small className="ml-2 text-[#8c919e]">роль: пользователь</small></span><span className="flex gap-2"><button type="button" onClick={() => setEditing(true)} className="rounded-xl border border-white/10 px-3 py-2 text-sm text-[#a6abb7] hover:bg-white/8">Изменить</button><button type="button" onClick={() => confirm ? onDelete() : setConfirm(true)} className={`rounded-xl px-3 py-2 text-sm ${confirm ? 'bg-red-300 text-black' : 'text-red-200 hover:bg-red-300/10'}`}>{confirm ? 'Удалить?' : 'Удалить'}</button></span></div>}</div>;
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
  return <div ref={outerRef} className={`player-marquee text-sm font-semibold md:text-base ${overflowing ? 'player-marquee--animate' : ''}`}><span ref={innerRef}>{text}</span></div>;
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
  useEffect(() => { seekingRef.current = false; seekDraftRef.current = 0; setCurrentTime(0); setDuration(0); setBufferedTime(0); setIsSeeking(false); setSeekDraft(0); }, [currentTrack?.id]);
  useEffect(() => () => { if (closeTimer.current) window.clearTimeout(closeTimer.current); }, []);
  useEffect(() => { const frame = requestAnimationFrame(() => setMiniEntered(true)); return () => cancelAnimationFrame(frame); }, []);
  useEffect(() => {
    if (!currentTrack || !('mediaSession' in navigator)) return;
    navigator.mediaSession.metadata = new MediaMetadata({ title: currentTrack.title, artist: currentTrack.artist ?? 'Исполнитель не указан', artwork: currentTrack.artwork_url ? [{ src: currentTrack.artwork_url }] : [] });
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
    if (mediaDuration > 0) setDuration(mediaDuration);
    let bufferedEnd = 0;
    for (let index = 0; index < audio.buffered.length; index += 1) {
      const start = audio.buffered.start(index);
      const end = audio.buffered.end(index);
      if (audio.currentTime >= start && audio.currentTime <= end) { bufferedEnd = end; break; }
      if (end > bufferedEnd) bufferedEnd = end;
    }
    setBufferedTime(Math.max(0, Math.min(bufferedEnd, mediaDuration || bufferedEnd)));
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

  return <>
    {!expanded && (<footer onClick={openFromShell} onTouchStart={onTouchStart} onTouchEnd={onTouchEnd} className={`player-morph-card touch-pan-y fixed inset-x-2 bottom-[calc(84px+env(safe-area-inset-bottom,0px))] z-20 mx-auto max-w-5xl cursor-pointer rounded-3xl border border-white/12 bg-[#101217]/95 p-2 shadow-2xl shadow-black/40 backdrop-blur md:bottom-2 md:grid md:grid-cols-[minmax(0,1fr)_minmax(300px,560px)] md:gap-3 md:p-3 ${miniEntered ? '' : 'player-mini--enter'}`}>
      {error && <div role="alert" className="col-span-full mb-2 flex items-center justify-between gap-3 rounded-2xl border border-red-300/30 bg-red-300/10 px-3 py-2 text-sm text-red-100"><span className="min-w-0 truncate">{error}</span><button type="button" onClick={() => setError(null)} className="rounded-xl p-1 hover:bg-white/10" aria-label="Скрыть ошибку"><X size={16} /></button></div>}
      <div className="grid grid-cols-[52px_minmax(0,1fr)_auto] items-center gap-2 md:flex md:min-w-0 md:gap-3">
        <button data-player-control="true" aria-label={status === 'playing' ? 'Pause' : 'Play'} onClick={playPause} className="relative grid h-13 w-13 shrink-0 place-items-center overflow-hidden rounded-2xl bg-lime-300/15 text-lime-100">{currentTrack.artwork_url ? <img alt="" src={currentTrack.artwork_url} className="h-full w-full object-cover" /> : <SourceIcon id={currentTrack.provider_id} className="h-5 w-5" />}<span className="absolute inset-0 grid place-items-center bg-black/30 text-white">{query.isFetching ? <Loader2 className="animate-spin" size={17} /> : status === 'playing' ? <Pause size={17} /> : <Play size={17} />}</span></button>
        <button type="button" data-open-player="true" onClick={openPlayer} className="min-w-0 overflow-hidden text-left" aria-label="Открыть плеер"><MiniPlayerTitle text={`${currentTrack.title} — ${currentTrack.artist ?? 'Исполнитель не указан'}`} /><div className="mt-1 flex items-center gap-2 text-xs text-[#8c919e]"><SourceIcon id={currentTrack.provider_id} className="h-4 w-4" /><span className="truncate">{currentTrack.artist ?? 'Выбери трек'}</span></div></button>
        <div data-player-control="true" className="relative flex items-center gap-1 justify-self-end md:hidden">
          <button aria-label="Меню плеера" aria-haspopup="menu" aria-expanded={miniMenuOpen} onClick={() => setMiniMenuOpen((value) => !value)} className={`${actionClass} ${shuffle || activeRepeat || liked || downloaded ? 'text-lime-200' : ''}`}><MoreHorizontal size={17} /></button>
          {miniMenuOpen && <><button type="button" aria-label="Закрыть меню плеера" tabIndex={-1} onClick={() => setMiniMenuOpen(false)} className="fixed inset-0 z-30 cursor-default" /><div role="menu" className="absolute right-0 bottom-11 z-40 w-64 overflow-hidden rounded-2xl border border-white/10 bg-[#151821] p-1 shadow-2xl shadow-black/50">
            <button role="menuitem" onClick={() => { previous(); setMiniMenuOpen(false); }} className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8"><SkipBack size={16} /> Предыдущий трек</button>
            <button role="menuitem" onClick={() => { next(); setMiniMenuOpen(false); }} className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8"><SkipForward size={16} /> Следующий трек</button>
            <button role="menuitem" aria-pressed={liked} onClick={() => { onLike(currentTrack); setMiniMenuOpen(false); }} className={`flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8 ${liked ? 'text-red-300' : ''}`}><Heart size={16} fill={liked ? 'currentColor' : 'none'} /> {liked ? 'Убрать лайк' : 'Лайк'}</button>
            <button role="menuitem" disabled={!currentTrack.policy.download_allowed || downloaded || createDownload.isPending} onClick={download} className={`flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8 disabled:opacity-50 ${downloaded ? 'text-lime-300' : ''}`}>{createDownload.isPending ? <Loader2 className="animate-spin" size={16} /> : downloaded ? <Folder size={16} /> : <Download size={16} />} {downloaded ? 'Уже скачано' : 'Скачать mp3'}</button>
            <button role="menuitem" aria-pressed={shuffle} onClick={() => { toggleShuffle(); setMiniMenuOpen(false); }} className={`flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8 ${shuffle ? 'text-lime-300' : ''}`}><Shuffle size={16} /> {shuffle ? 'Выключить шафл' : 'Включить шафл'}</button>
            <button role="menuitem" aria-pressed={activeRepeat} onClick={() => { cycleRepeat(); setMiniMenuOpen(false); }} className={`flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8 ${activeRepeat ? 'text-lime-300' : ''}`}>{repeat === 'one' ? <Repeat1 size={16} /> : <Repeat size={16} />} Повтор: {repeat === 'off' ? 'выкл' : repeat === 'all' ? 'всё' : 'один'}</button>
            <button role="menuitem" onClick={() => { setMiniMenuOpen(false); openPlayer(); }} className="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm hover:bg-white/8"><Music2 size={16} /> Открыть плеер</button>
          </div></>}
        </div>
        <div data-player-control="true" className="hidden gap-2 md:flex"><button aria-label={liked ? 'Убрать лайк' : 'Поставить лайк'} onClick={() => onLike(currentTrack)} className={`grid h-11 w-11 place-items-center rounded-2xl border border-white/10 hover:bg-white/8 ${liked ? 'text-red-300' : 'text-[#a6abb7]'}`}><Heart size={18} fill={liked ? 'currentColor' : 'none'} /></button><button aria-label={downloaded ? 'Уже скачано' : 'Скачать'} disabled={!currentTrack.policy.download_allowed || downloaded || createDownload.isPending} onClick={download} className={`grid h-11 w-11 place-items-center rounded-2xl border border-white/10 hover:bg-white/8 disabled:opacity-50 ${downloaded ? 'text-lime-300' : 'text-[#a6abb7]'}`}>{createDownload.isPending ? <Loader2 className="animate-spin" size={18} /> : downloaded ? <Folder size={18} /> : <Download size={18} />}</button></div>
      </div>
      <div data-player-control="true" className="mt-2 grid min-w-0 gap-2 md:mt-0"><div className="hidden items-center justify-center gap-2 md:flex"><button aria-label="Shuffle" onClick={toggleShuffle} className={`rounded-xl p-2 ${shuffle ? 'bg-lime-300 text-black' : 'hover:bg-white/8 text-[#a6abb7]'}`}><Shuffle size={17} /></button><button aria-label="Previous" onClick={previous} className="rounded-xl p-2 text-[#a6abb7] hover:bg-white/8"><SkipBack size={18} /></button><button aria-label={status === 'playing' ? 'Pause' : 'Play'} onClick={playPause} className="grid h-10 w-10 place-items-center rounded-full bg-lime-300 text-black shadow-lg shadow-lime-300/20">{query.isFetching ? <Loader2 className="animate-spin" size={19} /> : status === 'playing' ? <Pause size={19} /> : <Play size={19} />}</button><button aria-label="Next" onClick={next} className="rounded-xl p-2 text-[#a6abb7] hover:bg-white/8"><SkipForward size={18} /></button><button aria-label="Repeat" onClick={cycleRepeat} className={`rounded-xl p-2 ${activeRepeat ? 'bg-lime-300 text-black' : 'hover:bg-white/8 text-[#a6abb7]'}`}>{repeat === 'one' ? <Repeat1 size={17} /> : <Repeat size={17} />}</button><input className="range max-w-24" type="range" min={0} max={1} step={0.01} value={volume} onChange={(event) => setVolume(Number(event.currentTarget.value))} aria-label="Громкость" /></div><div className="grid grid-cols-[42px_1fr_42px] items-center gap-2 text-[11px] text-[#8c919e]"><span>{formatDuration(seekValue)}</span>{range}<span>{formatDuration(duration)}</span></div>{playback?.stream_url && <audio ref={audioRef} className="hidden" src={playback.stream_url} autoPlay loop={repeat === 'one'} onTimeUpdate={(e) => { if (!isSeeking) setCurrentTime(e.currentTarget.currentTime); updateBuffered(e.currentTarget); }} onProgress={(e) => updateBuffered(e.currentTarget)} onLoadedMetadata={(e) => { e.currentTarget.volume = volume; updateBuffered(e.currentTarget); updateDuration(e.currentTarget.duration); }} onDurationChange={(e) => updateDuration(e.currentTarget.duration)} onWaiting={() => setStatus('buffering')} onCanPlay={(e) => { updateBuffered(e.currentTarget); if (status === 'buffering') setStatus('playing'); }} onPlay={() => setStatus('playing')} onPause={() => { if (status !== 'playing') setStatus('paused'); }} onEnded={next} />}</div>
    </footer>)}
    {expanded && <div className={`player-sheet-backdrop fixed inset-0 z-50 grid place-items-end bg-black/70 p-0 backdrop-blur md:place-items-center md:p-3 ${sheetClosing ? 'player-sheet-backdrop--closing' : ''}`} role="dialog" aria-modal="true" aria-label="Плеер" onClick={closePlayer}>
      <section onClick={(event) => event.stopPropagation()} onTouchStart={onSheetTouchStart} onTouchMove={onSheetTouchMove} onTouchEnd={onSheetTouchEnd} style={sheetOffset > 0 ? { transform: `translateY(${sheetOffset}px)`, transition: 'none' } : { transition: 'transform 320ms cubic-bezier(.22,1,.36,1)' }} className={`player-sheet player-morph-card max-h-[92dvh] w-full overflow-hidden rounded-t-[2rem] border border-white/10 bg-[#101217] shadow-2xl md:max-w-md md:rounded-[2rem] ${sheetClosing ? 'player-sheet--closing' : ''}`}>
        <div data-sheet-handle="true" className="mx-auto flex h-7 w-28 cursor-grab items-start justify-center pt-2 md:hidden" aria-hidden="true"><div className="h-1.5 w-12 rounded-full bg-white/25" /></div>
        <div className="max-h-[92dvh] touch-pan-y overflow-y-auto overscroll-contain p-5 pt-3">
          <div className="mb-4 flex items-center justify-between">
            <button onClick={closePlayer} aria-label="Закрыть плеер" className="rounded-2xl p-2 text-[#a6abb7] hover:bg-white/8"><X size={20} /></button>
            <div className="flex gap-2">
              <button aria-label="Shuffle" aria-pressed={shuffle} onClick={toggleShuffle} className={`rounded-2xl p-3 ${shuffle ? 'bg-lime-300 text-black' : 'bg-white/6 text-[#a6abb7]'}`}><Shuffle size={18} /></button>
              <button aria-label="Repeat" aria-pressed={activeRepeat} onClick={cycleRepeat} className={`rounded-2xl p-3 ${activeRepeat ? 'bg-lime-300 text-black' : 'bg-white/6 text-[#a6abb7]'}`}>{repeat === 'one' ? <Repeat1 size={18} /> : <Repeat size={18} />}</button>
            </div>
          </div>
          <div className="mx-auto mb-5 grid aspect-square max-w-[70vw] place-items-center overflow-hidden rounded-[2rem] bg-lime-300/15 shadow-2xl shadow-black/35 md:max-w-72">{currentTrack.artwork_url ? <img alt="" src={currentTrack.artwork_url} className="h-full w-full object-cover" /> : <SourceIcon id={currentTrack.provider_id} className="h-14 w-14" />}</div>
          <h2 className="m-0 truncate text-2xl font-semibold">{currentTrack.title}</h2>
          <p className="m-0 mt-1 flex items-center gap-2 text-[#9aa0ad]"><SourceIcon id={currentTrack.provider_id} className="h-4 w-4" />{currentTrack.artist ?? 'Исполнитель не указан'}</p>
          <div className="mt-5 grid grid-cols-[42px_1fr_42px] items-center gap-2 text-[11px] text-[#8c919e]"><span>{formatDuration(seekValue)}</span>{range}<span>{formatDuration(duration)}</span></div>
          <div className="mt-2 flex items-center justify-between text-[11px] text-[#8c919e]"><span>{status === 'buffering' ? 'Буферизую…' : 'Поток'}</span><span aria-label={`Загружено ${formatDuration(bufferedTime)}`}>Загружено {formatDuration(bufferedTime)} · {bufferProgress.toFixed(0)}%</span></div>
          <div className="mt-6 flex items-center justify-center gap-4">
            <button aria-label="Previous" onClick={previous} className="rounded-2xl bg-white/6 p-4 text-[#a6abb7]"><SkipBack size={24} /></button>
            <button aria-label={status === 'playing' ? 'Pause' : 'Play'} onClick={playPause} className="grid h-16 w-16 place-items-center rounded-full bg-lime-300 text-black shadow-lg shadow-lime-300/20">{query.isFetching ? <Loader2 className="animate-spin" size={24} /> : status === 'playing' ? <Pause size={24} /> : <Play size={24} />}</button>
            <button aria-label="Next" onClick={next} className="rounded-2xl bg-white/6 p-4 text-[#a6abb7]"><SkipForward size={24} /></button>
          </div>
          <div className="mt-5 flex items-center justify-center gap-2">
            <button aria-label={liked ? 'Убрать лайк' : 'Поставить лайк'} onClick={() => onLike(currentTrack)} className={`rounded-2xl border border-white/10 p-3 ${liked ? 'text-red-300' : 'text-[#a6abb7]'}`}><Heart size={18} fill={liked ? 'currentColor' : 'none'} /></button>
            <button aria-label={downloaded ? 'Уже скачано' : 'Скачать'} disabled={!currentTrack.policy.download_allowed || downloaded || createDownload.isPending} onClick={download} className={`rounded-2xl border border-white/10 p-3 disabled:opacity-50 ${downloaded ? 'text-lime-300' : 'text-[#a6abb7]'}`}>{createDownload.isPending ? <Loader2 className="animate-spin" size={18} /> : downloaded ? <Folder size={18} /> : <Download size={18} />}</button>
          </div>
          <section className="mt-6 rounded-3xl border border-white/10 bg-black/20 p-3">
            <div className="mb-2 flex items-center justify-between px-1"><h3 className="m-0 text-sm font-semibold">Дальше в очереди</h3><span className="text-xs text-[#8c919e]">{queueIndex + 1}/{queue.length || 1}</span></div>
            <div className="grid max-h-52 gap-1 overflow-y-auto overscroll-contain pr-1">
              {queue.map((track, index) => <button key={`${track.id}-${index}`} type="button" onClick={() => playQueue(queue, index)} className={`grid grid-cols-[38px_minmax(0,1fr)_auto] items-center gap-2 rounded-2xl px-2 py-2 text-left ${index === queueIndex ? 'bg-lime-300/15 text-lime-100' : 'hover:bg-white/8'}`}>
                <span className="grid h-9 w-9 place-items-center overflow-hidden rounded-xl bg-white/8">{track.artwork_url ? <img alt="" src={track.artwork_url} className="h-full w-full object-cover" /> : <SourceIcon id={track.provider_id} className="h-4 w-4" />}</span>
                <span className="min-w-0"><strong className="block truncate text-sm">{track.title}</strong><small className="block truncate text-[#8c919e]">{track.artist ?? 'Исполнитель не указан'}</small></span>
                {index === queueIndex && <span className="eq" aria-label="Сейчас играет"><span /><span /><span /></span>}
              </button>)}
            </div>
          </section>
        </div>
      </section>
    </div>}
  </>;
}
