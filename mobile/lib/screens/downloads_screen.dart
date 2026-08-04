import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../state/library_controller.dart';
import '../state/offline_controller.dart';
import '../state/player_controller.dart';
import '../theme/tokens.dart';
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
                _Segments(
                  showDevice: _showDevice,
                  deviceCount: deviceTracks.length,
                  serverCount: serverTracks.length,
                  onChanged: (value) => setState(() => _showDevice = value),
                ),
                const SizedBox(height: 10),
                Text(
                  _showDevice
                      ? 'Лежат в памяти телефона — играют без интернета.'
                      : 'Лежат на сервере — нужен доступ к нему, чтобы слушать.',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
                if (_showDevice) _UsageLine(offline: offline),
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

/// A pill switch with a sliding indicator instead of Material's default
/// SegmentedButton: its selected state uses `secondaryContainer`, which on this
/// dark theme lands as dark-grey-on-grey and reads as disabled rather than
/// chosen. Here the active half carries the accent, so which tab is live is
/// obvious at a glance.
class _Segments extends StatelessWidget {
  const _Segments({
    required this.showDevice,
    required this.deviceCount,
    required this.serverCount,
    required this.onChanged,
  });
  final bool showDevice;
  final int deviceCount;
  final int serverCount;
  final ValueChanged<bool> onChanged;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Container(
      height: 48,
      padding: const EdgeInsets.all(4),
      decoration: BoxDecoration(
        color: AppColors.surface2,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Stack(
        children: [
          AnimatedAlign(
            duration: const Duration(milliseconds: 240),
            curve: Curves.easeOutCubic,
            alignment: showDevice
                ? Alignment.centerLeft
                : Alignment.centerRight,
            child: FractionallySizedBox(
              widthFactor: 0.5,
              heightFactor: 1,
              child: Container(
                decoration: BoxDecoration(
                  color: scheme.primary,
                  borderRadius: BorderRadius.circular(999),
                ),
              ),
            ),
          ),
          Row(
            children: [
              Expanded(
                child: _Segment(
                  icon: Icons.phone_iphone_rounded,
                  label: 'На устройстве',
                  count: deviceCount,
                  selected: showDevice,
                  onTap: () => onChanged(true),
                ),
              ),
              Expanded(
                child: _Segment(
                  icon: Icons.cloud_done_rounded,
                  label: 'На сервере',
                  count: serverCount,
                  selected: !showDevice,
                  onTap: () => onChanged(false),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class _Segment extends StatelessWidget {
  const _Segment({
    required this.icon,
    required this.label,
    required this.count,
    required this.selected,
    required this.onTap,
  });
  final IconData icon;
  final String label;
  final int count;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    // Colour alone should not carry the selected state, so the active half also
    // shifts to a heavier weight.
    final color = selected ? scheme.onPrimary : AppColors.muted;
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: onTap,
      child: Center(
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 17, color: color),
            const SizedBox(width: 7),
            Flexible(
              child: Text(
                label,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  color: color,
                  fontSize: 13.5,
                  fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
                ),
              ),
            ),
            const SizedBox(width: 6),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
              decoration: BoxDecoration(
                color: selected
                    ? scheme.onPrimary.withValues(alpha: 0.16)
                    : Colors.white.withValues(alpha: 0.07),
                borderRadius: BorderRadius.circular(999),
              ),
              child: Text(
                '$count',
                style: TextStyle(
                  color: color,
                  fontSize: 11.5,
                  fontWeight: FontWeight.w700,
                  fontFeatures: const [FontFeature.tabularFigures()],
                ),
              ),
            ),
          ],
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
