import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../api/models.dart';
import '../state/library_controller.dart';
import '../state/offline_controller.dart';
import '../state/player_controller.dart';
import '../theme/tokens.dart';
import 'track_art.dart';

/// Everything you can do with a track, in one place.
///
/// A bottom sheet rather than a dropdown: on a phone this lands under the
/// thumb, while an anchored menu next to a list row would open near the top of
/// the screen where it is hardest to reach.
///
/// Destructive items sit in their own group at the bottom and are coloured, so
/// "remove from device" is never a neighbour of "add to playlist" in muscle
/// memory. Removals that lose data ask for confirmation; ones that are trivially
/// undoable (unlike, remove from queue) do not.
Future<void> showTrackActions(
  BuildContext context, {
  required Track track,

  /// Set when opened from inside a playlist — enables "remove from this playlist".
  String? playlistId,
  VoidCallback? onRemovedFromPlaylist,
}) {
  return showModalBottomSheet(
    context: context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    builder: (_) => _TrackActionsSheet(
      track: track,
      playlistId: playlistId,
      onRemovedFromPlaylist: onRemovedFromPlaylist,
    ),
  );
}

class _TrackActionsSheet extends StatelessWidget {
  const _TrackActionsSheet({
    required this.track,
    this.playlistId,
    this.onRemovedFromPlaylist,
  });
  final Track track;
  final String? playlistId;
  final VoidCallback? onRemovedFromPlaylist;

  @override
  Widget build(BuildContext context) {
    final library = context.watch<LibraryController>();
    final offline = context.watch<OfflineController>();
    final player = context.read<PlayerController>();
    final liked = library.favoriteIds.contains(track.id);
    final onServer = library.downloadedIds.contains(track.id);
    final onDevice = offline.isOnDevice(track.id);
    final busy = offline.isBusy(track.id) || library.downloadingIds.contains(track.id);

    return SafeArea(
      child: Container(
        margin: const EdgeInsets.all(8),
        decoration: BoxDecoration(
          color: AppColors.surface2,
          borderRadius: BorderRadius.circular(AppRadius.sheet),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 16, 16, 12),
              child: Row(
                children: [
                  SizedBox(
                    width: 48,
                    height: 48,
                    child: TrackArt(track: track, radius: AppRadius.control),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          track.title,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(fontWeight: FontWeight.w600),
                        ),
                        const SizedBox(height: 2),
                        Text(
                          track.artist ?? 'Исполнитель не указан',
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: Theme.of(context).textTheme.bodySmall,
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
            const Divider(height: 1),
            Flexible(
              child: ListView(
                shrinkWrap: true,
                padding: const EdgeInsets.symmetric(vertical: 4),
                children: [
                  _Action(
                    icon: Icons.playlist_play_rounded,
                    label: 'Слушать следующим',
                    onTap: () {
                      player.playNext(track);
                      Navigator.pop(context);
                      _toast(context, 'Пойдёт следующим');
                    },
                  ),
                  _Action(
                    icon: Icons.queue_music_rounded,
                    label: 'В конец очереди',
                    onTap: () {
                      player.addToQueue(track);
                      Navigator.pop(context);
                      _toast(context, 'Добавлено в очередь');
                    },
                  ),
                  _Action(
                    icon: liked ? Icons.favorite : Icons.favorite_border,
                    iconColor: liked ? AppColors.danger : null,
                    label: liked ? 'Убрать из медиатеки' : 'В медиатеку',
                    onTap: () {
                      library.toggleFavorite(track);
                      Navigator.pop(context);
                    },
                  ),
                  _Action(
                    icon: Icons.playlist_add_rounded,
                    label: 'Добавить в плейлист',
                    onTap: () async {
                      Navigator.pop(context);
                      await _pickPlaylist(context, track);
                    },
                  ),
                  const Divider(height: 9, indent: 16, endIndent: 16),

                  // Two destinations, deliberately worded by what they buy you
                  // rather than by where the bytes go.
                  _Action(
                    icon: onServer
                        ? Icons.cloud_done_rounded
                        : Icons.cloud_download_rounded,
                    iconColor: onServer ? Theme.of(context).colorScheme.primary : null,
                    label: onServer ? 'Уже на сервере' : 'Скачать на сервер',
                    hint: onServer
                        ? 'Не зависит от YouTube'
                        : 'Чтобы не зависеть от YouTube',
                    enabled: !onServer && !busy,
                    busy: busy && !onDevice,
                    onTap: () async {
                      Navigator.pop(context);
                      final ok = await library.downloadTrack(track);
                      if (context.mounted) {
                        _toast(context, ok ? 'Скачано на сервер' : (library.actionError ?? 'Не удалось скачать'));
                      }
                    },
                  ),
                  if (OfflineController.isSupported)
                    _Action(
                      icon: onDevice
                          ? Icons.phone_iphone_rounded
                          : Icons.download_for_offline_rounded,
                      iconColor: onDevice ? Theme.of(context).colorScheme.primary : null,
                      label: onDevice ? 'Уже на устройстве' : 'Сохранить на устройство',
                      hint: onDevice
                          ? 'Играет без интернета'
                          : 'Чтобы играло без интернета',
                      enabled: !onDevice && !busy,
                      busy: busy && onServer,
                      onTap: () async {
                        Navigator.pop(context);
                        final ok = await offline.saveToDevice(
                          track,
                          onServerCopyCreated: library.refreshDownloads,
                        );
                        if (context.mounted) {
                          _toast(context, ok ? 'Сохранено на устройство' : (offline.error ?? 'Не удалось сохранить'));
                        }
                      },
                    ),

                  if (onDevice || playlistId != null) ...[
                    const Divider(height: 9, indent: 16, endIndent: 16),
                    if (onDevice)
                      _Action(
                        icon: Icons.delete_outline_rounded,
                        label: 'Удалить с устройства',
                        hint: 'Освободит место, останется на сервере',
                        destructive: true,
                        onTap: () async {
                          Navigator.pop(context);
                          await offline.removeFromDevice(track.id);
                          if (context.mounted) _toast(context, 'Удалено с устройства');
                        },
                      ),
                    if (playlistId != null)
                      _Action(
                        icon: Icons.playlist_remove_rounded,
                        label: 'Убрать из плейлиста',
                        destructive: true,
                        onTap: () async {
                          final confirmed = await _confirm(
                            context,
                            title: 'Убрать из плейлиста?',
                            message: 'Трек останется в медиатеке и на сервере.',
                            action: 'Убрать',
                          );
                          if (!confirmed || !context.mounted) return;
                          Navigator.pop(context);
                          await library.removePlaylistTrack(playlistId!, track.id);
                          onRemovedFromPlaylist?.call();
                        },
                      ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Action extends StatelessWidget {
  const _Action({
    required this.icon,
    required this.label,
    required this.onTap,
    this.hint,
    this.iconColor,
    this.destructive = false,
    this.enabled = true,
    this.busy = false,
  });
  final IconData icon;
  final String label;
  final String? hint;
  final Color? iconColor;
  final bool destructive;
  final bool enabled;
  final bool busy;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final color = destructive
        ? AppColors.danger
        : (enabled ? null : AppColors.muted.withValues(alpha: 0.5));
    return ListTile(
      enabled: enabled && !busy,
      onTap: onTap,
      leading: busy
          ? const SizedBox(
              width: 22,
              height: 22,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          : Icon(icon, size: 22, color: iconColor ?? color ?? AppColors.muted),
      title: Text(label, style: TextStyle(color: color)),
      subtitle: hint == null
          ? null
          : Text(hint!, style: Theme.of(context).textTheme.bodySmall),
      dense: true,
    );
  }
}

void _toast(BuildContext context, String message) {
  ScaffoldMessenger.of(context).showSnackBar(
    SnackBar(content: Text(message), behavior: SnackBarBehavior.floating),
  );
}

Future<bool> _confirm(
  BuildContext context, {
  required String title,
  required String message,
  required String action,
}) async {
  final result = await showDialog<bool>(
    context: context,
    builder: (context) => AlertDialog(
      title: Text(title),
      content: Text(message),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context, false),
          child: const Text('Отмена'),
        ),
        TextButton(
          onPressed: () => Navigator.pop(context, true),
          style: TextButton.styleFrom(foregroundColor: AppColors.danger),
          child: Text(action),
        ),
      ],
    ),
  );
  return result ?? false;
}

Future<void> _pickPlaylist(BuildContext context, Track track) async {
  final library = context.read<LibraryController>();
  await showModalBottomSheet(
    context: context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    builder: (sheetContext) => SafeArea(
      child: Container(
        margin: const EdgeInsets.all(8),
        decoration: BoxDecoration(
          color: AppColors.surface2,
          borderRadius: BorderRadius.circular(AppRadius.sheet),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Padding(
              padding: const EdgeInsets.all(16),
              child: Text(
                'В какой плейлист?',
                style: Theme.of(sheetContext).textTheme.titleMedium,
              ),
            ),
            const Divider(height: 1),
            if (library.playlists.isEmpty)
              const Padding(
                padding: EdgeInsets.all(24),
                child: Text(
                  'Плейлистов пока нет. Создай его на вкладке «Плейлисты».',
                  textAlign: TextAlign.center,
                  style: TextStyle(color: AppColors.muted),
                ),
              )
            else
              Flexible(
                child: ListView.builder(
                  shrinkWrap: true,
                  itemCount: library.playlists.length,
                  itemBuilder: (context, index) {
                    final playlist = library.playlists[index];
                    return ListTile(
                      leading: const Icon(Icons.queue_music_rounded, color: AppColors.muted),
                      title: Text(playlist.name),
                      subtitle: Text('${playlist.trackCount} треков'),
                      onTap: () async {
                        Navigator.pop(sheetContext);
                        final ok = await library.addPlaylistTrack(playlist.id, track.id);
                        if (context.mounted) {
                          _toast(context, ok ? 'Добавлено в «${playlist.name}»' : (library.actionError ?? 'Не удалось добавить'));
                        }
                      },
                    );
                  },
                ),
              ),
          ],
        ),
      ),
    ),
  );
}
