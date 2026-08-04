import 'dart:async';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../state/library_controller.dart';
import '../state/player_controller.dart';
import '../theme/tokens.dart';
import '../widgets/skeleton.dart';
import '../widgets/source_icon.dart';
import '../widgets/track_row.dart';

class SearchScreen extends StatefulWidget {
  const SearchScreen({super.key});
  @override
  State<SearchScreen> createState() => _SearchScreenState();
}

class _SearchScreenState extends State<SearchScreen> {
  final _controller = TextEditingController();
  Timer? _debounce;

  void _onChanged(String value) {
    _debounce?.cancel();
    _debounce = Timer(
      const Duration(milliseconds: 350),
      () => context.read<LibraryController>().search(value),
    );
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final library = context.watch<LibraryController>();
    final player = context.watch<PlayerController>();
    final searchProviders = library.providers
        .where((provider) => provider.enabled && provider.canSearch)
        .toList();

    return CustomScrollView(
      slivers: [
        SliverPadding(
          padding: const EdgeInsets.fromLTRB(20, 20, 20, 8),
          sliver: SliverToBoxAdapter(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'ПОИСК',
                  style: Theme.of(context).textTheme.labelSmall?.copyWith(
                    color: Theme.of(context).colorScheme.primary,
                  ),
                ),
                const SizedBox(height: 6),
                Text(
                  'Что послушаем?',
                  style: Theme.of(
                    context,
                  ).textTheme.headlineLarge?.copyWith(fontSize: 40),
                ),
                const SizedBox(height: 16),
                TextField(
                  controller: _controller,
                  onChanged: _onChanged,
                  onSubmitted: (v) => library.search(v),
                  decoration: const InputDecoration(
                    hintText: 'Название, исполнитель или альбом',
                    prefixIcon: Icon(Icons.search),
                  ),
                ),
                const SizedBox(height: 12),
                Text(
                  'ИСТОЧНИКИ',
                  style: Theme.of(context).textTheme.labelSmall,
                ),
                const SizedBox(height: 8),
                Wrap(
                  spacing: 10,
                  runSpacing: 10,
                  children: [
                    for (final provider in searchProviders)
                      _SourceToggle(
                        providerId: provider.id,
                        selected: library.selectedProviderIds.contains(provider.id),
                        onTap: () => library.toggleProvider(provider.id),
                      ),
                  ],
                ),
              ],
            ),
          ),
        ),
        if (library.searchState == LoadState.loading)
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(20, 8, 20, 100),
            sliver: SliverList.builder(
              itemCount: 6,
              itemBuilder: (context, index) => const TrackRowSkeleton(),
            ),
          )
        else if (library.searchQuery.isEmpty)
          SliverFillRemaining(
            hasScrollBody: false,
            child: _hint(
              context,
              'Найди свой трек',
              'Введи название песни, исполнителя или альбома.',
            ),
          )
        else if (library.searchResults.isEmpty)
          SliverFillRemaining(
            hasScrollBody: false,
            child: _hint(
              context,
              'Ничего не нашлось',
              'Попробуй написать короче или другими словами.',
            ),
          )
        else
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(20, 8, 20, 100),
            sliver: SliverList.builder(
              itemCount: library.searchResults.length,
              itemBuilder: (context, index) {
                final track = library.searchResults[index];
                return TrackRow(
                  track: track,
                  liked: library.favoriteIds.contains(track.id),
                  isCurrent: player.currentTrack?.id == track.id,
                  downloaded:
                      track.downloaded ||
                      library.downloadedIds.contains(track.id),
                  downloading: library.downloadingIds.contains(track.id),
                  onPlay: () => player.playFrom(library.searchResults, index),
                  onLike: () => library.toggleFavorite(track),
                  onDownload: track.policy.downloadAllowed
                      ? () async {
                          final ok = await library.downloadTrack(track);
                          if (!ok && context.mounted) {
                            ScaffoldMessenger.of(context).showSnackBar(
                              SnackBar(
                                content: Text(
                                  library.actionError ??
                                      'Не удалось скачать трек',
                                ),
                              ),
                            );
                          }
                        }
                      : null,
                );
              },
            ),
          ),
      ],
    );
  }

  Widget _hint(BuildContext context, String title, String subtitle) => Center(
    child: Padding(
      padding: const EdgeInsets.all(32),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Icon(Icons.search, size: 40, color: AppColors.muted),
          const SizedBox(height: 16),
          Text(title, style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 6),
          Text(subtitle, textAlign: TextAlign.center),
        ],
      ),
    ),
  );
}

/// Icon-only source filter.
///
/// The brand mark identifies the source faster than its name, but an icon
/// alone cannot say "on" or "off" — so selection is carried by a filled,
/// brand-tinted surface plus a tooltip/Semantics label, not colour alone.
class _SourceToggle extends StatelessWidget {
  const _SourceToggle({required this.providerId, required this.selected, required this.onTap});

  final String providerId;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final color = sourceColor(providerId, context);
    final name = sourceName(providerId);
    return Tooltip(
      message: selected ? '$name — искать здесь' : '$name — не искать',
      child: Semantics(
        button: true,
        toggled: selected,
        label: name,
        child: InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(AppRadius.card),
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 180),
            curve: Curves.easeOut,
            width: 56,
            height: 44,
            decoration: BoxDecoration(
              color: selected ? color.withValues(alpha: 0.18) : AppColors.surface2,
              borderRadius: BorderRadius.circular(AppRadius.card),
              border: Border.all(
                color: selected ? color.withValues(alpha: 0.55) : AppColors.border,
                width: selected ? 1.5 : 1,
              ),
            ),
            child: Center(
              child: Opacity(
                opacity: selected ? 1 : 0.45,
                child: SourceIcon(providerId: providerId, size: 22),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
