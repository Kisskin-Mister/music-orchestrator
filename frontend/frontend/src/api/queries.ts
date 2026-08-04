import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from './client';
import type { Favorite, Track } from './types';

export const useSettings=()=>useQuery({queryKey:['settings'],queryFn:api.settings,retry:false});
export const useUpdateSettings=()=>{const qc=useQueryClient();return useMutation({mutationFn:api.updateSettings,onSuccess:(next)=>{qc.setQueryData(['settings'],next);qc.invalidateQueries({queryKey:['providers']});}})};
export const useSession=()=>useQuery({queryKey:['session'],queryFn:api.session,staleTime:60_000,retry:false});
export const useUpdateAccount=()=>{const qc=useQueryClient();return useMutation({mutationFn:api.updateAccount,onSuccess:(session)=>{qc.setQueryData(['session'],session);qc.invalidateQueries({queryKey:['session']});}})};
export const useCreateUser=()=>{const qc=useQueryClient();return useMutation({mutationFn:({username,password}:{username:string;password:string})=>api.createUser(username,password),onSuccess:()=>qc.invalidateQueries({queryKey:['users']})})};
export const useUpdateUser=()=>{const qc=useQueryClient();return useMutation({mutationFn:({userId,patch}:{userId:string;patch:{username?:string;password?:string}})=>api.updateUser(userId,patch),onSuccess:()=>qc.invalidateQueries({queryKey:['users']})})};
export const useDeleteUser=()=>{const qc=useQueryClient();return useMutation({mutationFn:api.deleteUser,onSuccess:()=>qc.invalidateQueries({queryKey:['users']})})};
export const useLogout=()=>{
  const qc=useQueryClient();
  return useMutation({
    mutationFn:api.logout,
    onSettled:()=>{
      qc.clear();
      qc.setQueryData(['session'],{authenticated:false,setup_required:false,totp_required:false,totp_enabled:false,login_enabled:true});
      qc.invalidateQueries({queryKey:['session']});
    },
  });
};
export const useProviders=()=>useQuery({queryKey:['providers'],queryFn:api.providers});
export const useUsers=()=>useQuery({queryKey:['users'],queryFn:api.users});
export const useSearch=(query:string,providers:string[],limit=20,offset=0)=>useQuery({queryKey:['search',query,providers,limit,offset],queryFn:({signal})=>api.search(query,providers,limit,offset,signal),enabled:query.trim().length>0 && providers.length>0, staleTime:0, retry:false});
export const usePlayback=(trackId:string|null)=>useQuery({queryKey:['playback',trackId],queryFn:()=>api.playback(trackId!),enabled:Boolean(trackId),staleTime:0});
export const useDownloads=()=>useQuery({queryKey:['downloads'],queryFn:api.downloads});
export const useFavorites=()=>useQuery({queryKey:['favorites'],queryFn:api.favorites});
export const usePlaylists=()=>useQuery({queryKey:['playlists'],queryFn:api.playlists});
export const usePlaylist=(playlistId:string|null)=>useQuery({queryKey:['playlist',playlistId],queryFn:()=>api.playlist(playlistId!),enabled:Boolean(playlistId)});

export const useAddFavorite=()=>{
  const qc=useQueryClient();
  return useMutation({
    mutationFn:(track:Track)=>api.addFavorite(track.id,track),
    onMutate:async(track)=>{
      await qc.cancelQueries({queryKey:['favorites']});
      const previous=qc.getQueryData<Favorite[]>(['favorites']);
      const optimistic:Favorite={track_id:track.id,track,created_at:new Date().toISOString()};
      qc.setQueryData<Favorite[]>(['favorites'],(current=[])=>current.some((item)=>item.track_id===track.id)?current:[optimistic,...current]);
      return {previous};
    },
    onError:(_error,_track,context)=>{ if(context?.previous) qc.setQueryData(['favorites'],context.previous); },
    onSettled:()=>qc.invalidateQueries({queryKey:['favorites']}),
  });
};
export const useDeleteFavorite=()=>{
  const qc=useQueryClient();
  return useMutation({
    mutationFn:api.deleteFavorite,
    onMutate:async(trackId:string)=>{
      await qc.cancelQueries({queryKey:['favorites']});
      const previous=qc.getQueryData<Favorite[]>(['favorites']);
      qc.setQueryData<Favorite[]>(['favorites'],(current=[])=>current.filter((item)=>item.track_id!==trackId));
      return {previous};
    },
    onError:(_error,_trackId,context)=>{ if(context?.previous) qc.setQueryData(['favorites'],context.previous); },
    onSettled:()=>qc.invalidateQueries({queryKey:['favorites']}),
  });
};
export const useCreateDownload=()=>{const qc=useQueryClient();return useMutation({mutationFn:({trackId,format}:{trackId:string;format?:string})=>api.createDownload(trackId,format),onSuccess:()=>{qc.invalidateQueries({queryKey:['downloads']});qc.invalidateQueries({queryKey:['search']});qc.invalidateQueries({queryKey:['playback']});}})};
export const useDeleteDownload=()=>{const qc=useQueryClient();return useMutation({mutationFn:api.deleteDownload,onSuccess:()=>{qc.invalidateQueries({queryKey:['downloads']});qc.invalidateQueries({queryKey:['search']});qc.invalidateQueries({queryKey:['playback']});}})};
export const useCreatePlaylist=()=>{const qc=useQueryClient();return useMutation({mutationFn:({name,description}:{name:string;description?:string})=>api.createPlaylist(name,description),onSuccess:()=>qc.invalidateQueries({queryKey:['playlists']})})};
export const useUpdatePlaylist=()=>{const qc=useQueryClient();return useMutation({mutationFn:({playlistId,patch}:{playlistId:string;patch:{name?:string;description?:string;cover_url?:string}})=>api.updatePlaylist(playlistId,patch),onSuccess:(playlist)=>{qc.invalidateQueries({queryKey:['playlists']});qc.invalidateQueries({queryKey:['playlist',playlist.id]});}})};
export const useDeletePlaylist=()=>{const qc=useQueryClient();return useMutation({mutationFn:api.deletePlaylist,onSuccess:()=>qc.invalidateQueries({queryKey:['playlists']})})};
export const useAddPlaylistTrack=()=>{const qc=useQueryClient();return useMutation({mutationFn:({playlistId,trackId}:{playlistId:string;trackId:string})=>api.addPlaylistTrack(playlistId,trackId),onSuccess:(playlist)=>{qc.invalidateQueries({queryKey:['playlists']});qc.invalidateQueries({queryKey:['playlist',playlist.id]});}})};
export const useRemovePlaylistTrack=()=>{const qc=useQueryClient();return useMutation({mutationFn:({playlistId,trackId}:{playlistId:string;trackId:string})=>api.removePlaylistTrack(playlistId,trackId),onSuccess:(playlist)=>{qc.invalidateQueries({queryKey:['playlists']});qc.invalidateQueries({queryKey:['playlist',playlist.id]});}})};
