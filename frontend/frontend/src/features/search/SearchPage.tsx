import { FormEvent, useState } from 'react';
import { Download, Heart, Play, Search } from 'lucide-react';
import { useSearch } from '@/api/queries';
import { usePlayerStore } from '@/store/player';
import { analytics } from '@/lib/analytics';

export function SearchPage(){
  const [draft,setDraft]=useState(''); const [query,setQuery]=useState(''); const result=useSearch(query,[]); const setTrack=usePlayerStore(s=>s.setTrack);
  const submit=(e:FormEvent)=>{e.preventDefault();const next=draft.trim();if(!next)return;setQuery(next);analytics.track('search_submitted');};
  return <main className="mx-auto min-h-screen max-w-6xl px-5 py-8 pb-32">
    <p className="mb-2 font-mono text-[11px] uppercase tracking-[.09em] text-[#b8f545]">Личный эфирный пульт</p>
    <h1 className="mb-6 text-4xl font-semibold tracking-[-.03em] md:text-6xl">Поиск без потери контроля.</h1>
    <form onSubmit={submit} className="mb-8 flex h-12 items-center gap-3 rounded-xl border border-white/15 bg-[#101217] px-4"><Search size={18} className="text-[#8c919e]"/><input value={draft} onChange={e=>setDraft(e.target.value)} className="min-w-0 flex-1 bg-transparent outline-none" placeholder="Искать в библиотеке, YouTube и SoundCloud"/><kbd className="text-xs text-[#5f6572]">Enter</kbd></form>
    {result.isLoading&&<div aria-busy="true" className="text-[#8c919e]">Источники отвечают…</div>}
    {result.isError&&<section role="alert" className="rounded-xl border border-[#eb777c]/30 bg-[#eb777c]/5 p-5"><h2>Поиск не выполнен</h2><p className="text-[#8c919e]">Проверьте backend и повторите запрос.</p></section>}
    <div className="divide-y divide-white/10 border-t border-white/10">{result.data?.items.map(track=><article key={track.id} className="grid min-h-16 grid-cols-[1fr_auto] items-center gap-4"><div><strong>{track.title}</strong><p className="m-0 text-sm text-[#8c919e]">{track.artist||'Исполнитель не указан'} · {track.provider_id}</p></div><div className="flex gap-2"><button aria-label="Воспроизвести" onClick={()=>setTrack(track)} className="rounded-lg p-2 hover:bg-white/5"><Play size={17}/></button><button aria-label="В избранное" className="rounded-lg p-2 hover:bg-white/5"><Heart size={17}/></button>{track.policy.download_allowed&&<button aria-label="Сохранить offline" className="rounded-lg p-2 hover:bg-white/5"><Download size={17}/></button>}</div></article>)}</div>
  </main>;
}
