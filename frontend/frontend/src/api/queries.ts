import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from './client';

export const useProviders=()=>useQuery({queryKey:['providers'],queryFn:api.providers});
export const useSearch=(query:string,providers:string[])=>useQuery({queryKey:['search',query,providers],queryFn:()=>api.search(query,providers),enabled:query.trim().length>0});
export const usePlayback=(trackId:string|null)=>useQuery({queryKey:['playback',trackId],queryFn:()=>api.playback(trackId!),enabled:Boolean(trackId),staleTime:0});
export const useCreateDownload=()=>{const qc=useQueryClient();return useMutation({mutationFn:({trackId,format}:{trackId:string;format?:string})=>api.createDownload(trackId,format),onSuccess:()=>qc.invalidateQueries({queryKey:['jobs']})})};
