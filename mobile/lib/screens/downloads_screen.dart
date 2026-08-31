import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../state/library_controller.dart';
import '../state/offline_controller.dart';
import '../services/folder_import.dart';
import '../state/player_controller.dart';
import '../theme/tokens.dart';
import '../widgets/import_sheet.dart';
import '../widgets/pill_tabs.dart';
import '../widgets/track_row.dart';

/// Downloads live in two different places and the difference is not cosmetic:
/// a device copy plays with no network at all, a server copy still needs the
/// server to be reachable. Splitting them into explicit tabs is what makes
/// "will this play on the train?" answerable at a glance.
class DownloadsScreen extends StatefulWidget {
  const DownloadsScreen({super.key});

  @override
  State<DownloadsScreen> createState() => _DownloadsScreenState();
}

class _DownloadsScreenState extends State<DownloadsScreen> {
  bool _showDevice = true;

  @override
  Widget build(BuildContext context) {
    final library = context.watch<LibraryController>();
    final offline = context.watch<OfflineController>();
    final player = context.watch<PlayerController>();

    final deviceTracks = offline.deviceTracks;
    // A track saved on the device is also on the server, so the server tab
    // would otherwise repeat it. Show only what is *not* yet on the device.
    final serverTracks = library.downloads
        .where((track) => !offline.isOnDevice(track.id))
        .toList();
    final tracks = _showDevice ? deviceTracks : serverTracks;

    return CustomScrollView(
      slivers: [
        SliverPadding(
          padding: const EdgeInsets.fromLTRB(20, 20, 20, 8),
          sliver: SliverToBoxAdapter(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'OFFLINE',
                  style: Theme.of(context).textTheme.labelSmall?.copyWith(
                    color: Theme.of(context).colorScheme.primary,
                  ),
                ),
                const SizedBox(height: 6),
                Text(
                  'Загрузки',
                  style: Theme.of(
                    context,
                  ).textTheme.headlineLarge?.copyWith(fontSize: 40),
                ),
                const SizedBox(height: 16),
                PillTabs(
                  index: _showDevice ? 0 : 1,
                  onChanged: (i) => setState(() => _showDevice = i == 0),
                  tabs: [
                    PillTab(
                      icon: Icons.phone_iphone_rounded,
                      label: 'На устройстве',
                      count: deviceTracks.length,
                    ),
                    PillTab(
                      icon: Icons.cloud_done_rounded,
                      label: 'На сервере',
                      count: serverTracks.length,
                    ),
                  ],
                ),
                const SizedBox(height: 10),
                Text(
                  _showDevice
                      ? 'Лежат в памяти телефона — играют без интернета.'
                      : 'Лежат на сервере — нужен доступ к нему, чтобы слушать.',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
                if (_showDevice) _UsageLine(offline: offline),
                const SizedBox(height: 16),
                const _ImportButton(),
              ],
            ),
          ),
        ),
        if (tracks.isEmpty)
          SliverFillRemaining(
            hasScrollBody: false,
            child: Center(
              child: Padding(
                padding: const EdgeInsets.all(32),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(
                      _showDevice
                          ? Icons.phone_iphone_rounded
                          : Icons.cloud_done_rounded,
                      size: 40,
                      color: AppColors.muted,
                    ),
                    const SizedBox(height: 16),
                    Text(
                      _showDevice
                          ? 'На устройстве пусто'
                          : 'На сервере ничего нет',
                      style: Theme.of(context).textTheme.titleLarge,
                    ),
                    const SizedBox(height: 6),
                    Text(
                      _showDevice
                          ? 'Открой меню трека и выбери «Сохранить на устройство» — потом он будет играть без сети.'
                          : 'Скачай трек на сервер, чтобы не зависеть от YouTube и SoundCloud.',
                      textAlign: TextAlign.center,
                    ),
                  ],
                ),
              ),
            ),
          )
        else
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(20, 8, 20, 100),
            sliver: SliverList.builder(
              itemCount: tracks.length,
              itemBuilder: (context, index) {
                final track = tracks[index];
                return TrackRow(
                  track: track,
                  liked: library.favoriteIds.contains(track.id),
                  isCurrent: player.currentTrack?.id == track.id,
                  downloaded: true,
                  onPlay: () => player.playFrom(tracks, index),
                  onLike: () => library.toggleFavorite(track),
                );
              },
            ),
          ),
      ],
    );
  }
}

/// Своя музыка попадает в медиатеку отсюда же, где лежит скачанная: для
/// пользователя это один вопрос — «что у меня есть офлайн», — и разводить его
/// по разным экранам не за что.
class _ImportButton extends StatelessWidget {
  const _ImportButton();

  @override
  Widget build(BuildContext context) {
    final accent = Theme.of(context).colorScheme.primary;
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: () => showImportSheet(context),
        borderRadius: BorderRadius.circular(AppRadius.card),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
          decoration: BoxDecoration(
            color: accent.withValues(alpha: 0.10),
            border: Border.all(color: accent.withValues(alpha: 0.35)),
            borderRadius: BorderRadius.circular(AppRadius.card),
          ),
          child: Row(
            children: [
              Icon(Icons.upload_rounded, color: accent, size: 21),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text(
                      'Импортировать музыку',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      folderImportSupported
                          ? 'Файлы или папку целиком — со вложенными'
                          : 'Выбери файлы со своего устройства',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ],
                ),
              ),
              Icon(Icons.chevron_right_rounded, color: accent),
            ],
          ),
        ),
      ),
    );
  }
}

/// Storage taken by device downloads. Without it, "save to device" is an
/// action with an invisible cost the user only discovers when the phone
/// runs out of space.
class _UsageLine extends StatelessWidget {
  const _UsageLine({required this.offline});
  final OfflineController offline;

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<int>(
      future: offline.usedBytes(),
      builder: (context, snapshot) {
        final bytes = snapshot.data ?? 0;
        if (bytes <= 0) return const SizedBox.shrink();
        final mb = bytes / (1024 * 1024);
        return Padding(
          padding: const EdgeInsets.only(top: 6),
          child: Text(
            'Занято на устройстве: ${mb.toStringAsFixed(1)} МБ',
            style: Theme.of(context).textTheme.bodySmall,
          ),
        );
      },
    );
  }
}
