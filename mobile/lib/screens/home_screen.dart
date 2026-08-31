import 'dart:async';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../state/library_controller.dart';
import '../state/player_controller.dart';
import '../util/plural.dart';
import '../theme/tokens.dart';
import '../widgets/cover_strip.dart';
import '../widgets/pill_tabs.dart';
import '../widgets/source_icon.dart';
import '../widgets/track_row.dart';

/// Медиатека с тремя осями — «Треки», «Исполнители», «Альбомы».
///
/// Плоский список перестаёт быть медиатекой примерно на третьей сотне треков:
/// найти в нём что-то можно только пролистав. Оси и фасеты по источникам — это
/// то, чем много лет живут Navidrome, Plex и Apple Music, и придумывать здесь
/// что-то своё незачем.
///
/// Считает всё сервер: фильтрация, группировка и счётчики — запросы к SQLite,
/// а сюда приходит страница на 60 треков. Тянуть коллекцию целиком, чтобы
/// сгруппировать её в памяти телефона, — ровно то, что ломается на тысячах.
class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  final _searchController = TextEditingController();
  final _scrollController = ScrollController();
  Timer? _debounce;

  @override
  void initState() {
    super.initState();
    _scrollController.addListener(_onScroll);
  }

  @override
  void dispose() {
    _debounce?.cancel();
    _searchController.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  /// Запрос уходит только когда пользователь перестал печатать: иначе каждая
  /// буква — это запрос к серверу и мигающий список.
  void _onQueryChanged(String value) {
    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 300), () {
      context.read<LibraryController>().setLibraryQuery(value.trim());
    });
  }

  void _onScroll() {
    if (!_scrollController.hasClients) return;
    final position = _scrollController.position;
    if (position.pixels > position.maxScrollExtent - 600) {
      context.read<LibraryController>().loadMoreLibrary();
    }
  }

  /// Тап по исполнителю или альбому — это не отдельный экран, а тот же список
  /// треков, суженный до выбранного. Так с любой оси возвращаешься в одно и то
  /// же место, а не в очередной вложенный экран.
  void _openInTracks(String query) {
    _searchController.text = query;
    final library = context.read<LibraryController>();
    library.setLibraryQuery(query);
    library.setLibraryAxis(LibraryAxis.tracks);
  }

  @override
  Widget build(BuildContext context) {
    final library = context.watch<LibraryController>();
    final player = context.watch<PlayerController>();
    final recentTracks = player.recentTracks;
    final showRecent =
        recentTracks.isNotEmpty &&
        library.libraryQuery.isEmpty &&
        library.libraryAxis == LibraryAxis.tracks;

    return CustomScrollView(
      controller: _scrollController,
      slivers: [
        SliverPadding(
          padding: const EdgeInsets.fromLTRB(20, 20, 20, 8),
          sliver: SliverToBoxAdapter(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'КОЛЛЕКЦИЯ',
                  style: Theme.of(context).textTheme.labelSmall?.copyWith(
                    color: Theme.of(context).colorScheme.primary,
                  ),
                ),
                const SizedBox(height: 6),
                Text(
                  'Медиатека',
                  style: Theme.of(
                    context,
                  ).textTheme.headlineLarge?.copyWith(fontSize: 40),
                ),
                const SizedBox(height: 16),
                _SearchField(
                  controller: _searchController,
                  onChanged: _onQueryChanged,
                  onClear: () {
                    _searchController.clear();
                    _onQueryChanged('');
                  },
                ),
                const SizedBox(height: 12),
                PillTabs(
                  index: library.libraryAxis.index,
                  onChanged: (i) =>
                      library.setLibraryAxis(LibraryAxis.values[i]),
                  tabs: const [
                    PillTab(label: 'Треки'),
                    PillTab(label: 'Исполнители'),
                    PillTab(label: 'Альбомы'),
                  ],
                ),
                if (library.libraryAxis == LibraryAxis.tracks &&
                    library.librarySources.isNotEmpty) ...[
                  const SizedBox(height: 12),
                  _SourceFacets(
                    sources: library.librarySources,
                    selected: library.librarySource,
                    onSelect: library.setLibrarySource,
                  ),
                ],
                if (library.libraryFromCache) ...[
                  const SizedBox(height: 10),
                  Row(
                    children: [
                      const Icon(
                        Icons.cloud_off_rounded,
                        size: 15,
                        color: AppColors.muted,
                      ),
                      const SizedBox(width: 7),
                      Expanded(
                        child: Text(
                          'Сервер недоступен — показано то, что сохранено на устройстве.',
                          style: Theme.of(context).textTheme.bodySmall,
                        ),
                      ),
                    ],
                  ),
                ],
              ],
            ),
          ),
        ),
        if (showRecent)
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(20, 8, 20, 0),
            sliver: SliverToBoxAdapter(
              child: CoverStrip(
                eyebrow: 'Слушай снова',
                tracks: recentTracks,
                onPlay: (track, index) => player.playFrom(recentTracks, index),
              ),
            ),
          ),
        ...switch (library.libraryAxis) {
          LibraryAxis.tracks => _trackSlivers(library, player),
          LibraryAxis.artists => _artistSlivers(library),
          LibraryAxis.albums => _albumSlivers(library),
        },
      ],
    );
  }

  List<Widget> _trackSlivers(
    LibraryController library,
    PlayerController player,
  ) {
    final tracks = library.libraryPage;
    if (tracks.isEmpty) return [_empty(library)];
    return [
      SliverPadding(
        padding: const EdgeInsets.fromLTRB(20, 16, 20, 0),
        sliver: SliverList.builder(
          itemCount: tracks.length,
          itemBuilder: (context, index) {
            final track = tracks[index];
            return TrackRow(
              track: track,
              liked: library.favoriteIds.contains(track.id),
              isCurrent: player.currentTrack?.id == track.id,
              downloaded:
                  track.downloaded || library.downloadedIds.contains(track.id),
              downloading: library.downloadingIds.contains(track.id),
              onPlay: () => player.playFrom(tracks, index),
              onLike: () => library.toggleFavorite(track),
              onDownload: track.policy.downloadAllowed
                  ? () async {
                      final ok = await library.downloadTrack(track);
                      if (!ok && context.mounted) {
                        ScaffoldMessenger.of(context).showSnackBar(
                          SnackBar(
                            content: Text(
                              library.actionError ?? 'Не удалось скачать трек',
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
      SliverToBoxAdapter(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 16, 20, 100),
          child: Center(
            child: library.libraryLoadingMore
                ? const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Text(
                    'Показано ${tracks.length} из ${library.libraryTotal}',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
          ),
        ),
      ),
    ];
  }

  List<Widget> _artistSlivers(LibraryController library) {
    if (library.artists.isEmpty) return [_empty(library)];
    return [
      SliverPadding(
        padding: const EdgeInsets.fromLTRB(20, 16, 20, 100),
        sliver: SliverGrid.builder(
          gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(
            maxCrossAxisExtent: 200,
            mainAxisSpacing: 12,
            crossAxisSpacing: 12,
            childAspectRatio: 1.45,
          ),
          itemCount: library.artists.length,
          itemBuilder: (context, index) {
            final artist = library.artists[index];
            return _GroupCard(
              icon: Icons.person_rounded,
              title: artist.name,
              subtitle:
                  '${artist.tracks} ${plural(artist.tracks, 'трек', 'трека', 'треков')}'
                  ' · ${artist.albums} ${plural(artist.albums, 'альбом', 'альбома', 'альбомов')}',
              onTap: () => _openInTracks(artist.name),
            );
          },
        ),
      ),
    ];
  }

  List<Widget> _albumSlivers(LibraryController library) {
    if (library.albums.isEmpty) return [_empty(library)];
    return [
      SliverPadding(
        padding: const EdgeInsets.fromLTRB(20, 16, 20, 100),
        sliver: SliverGrid.builder(
          gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(
            maxCrossAxisExtent: 200,
            mainAxisSpacing: 12,
            crossAxisSpacing: 12,
            childAspectRatio: 1.45,
          ),
          itemCount: library.albums.length,
          itemBuilder: (context, index) {
            final album = library.albums[index];
            return _GroupCard(
              icon: Icons.album_rounded,
              title: album.name,
              subtitle:
                  '${album.artist} · ${album.tracks} ${plural(album.tracks, 'трек', 'трека', 'треков')}',
              onTap: () => _openInTracks(album.name),
            );
          },
        ),
      ),
    ];
  }

  Widget _empty(LibraryController library) => SliverFillRemaining(
    hasScrollBody: false,
    child: Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              library.libraryQuery.isEmpty
                  ? Icons.library_music_rounded
                  : Icons.search_off_rounded,
              size: 40,
              color: AppColors.muted,
            ),
            const SizedBox(height: 16),
            Text(
              library.libraryQuery.isEmpty ? 'Пока пусто' : 'Ничего не нашлось',
              style: Theme.of(context).textTheme.titleLarge,
            ),
            const SizedBox(height: 6),
            Text(
              library.libraryQuery.isEmpty
                  ? 'Лайкни трек, скачай его или импортируй свои файлы на вкладке «Загрузки».'
                  : 'Попробуй другое слово — ищем по названию, исполнителю и альбому.',
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    ),
  );
}

class _SearchField extends StatelessWidget {
  const _SearchField({
    required this.controller,
    required this.onChanged,
    required this.onClear,
  });
  final TextEditingController controller;
  final ValueChanged<String> onChanged;
  final VoidCallback onClear;

  @override
  Widget build(BuildContext context) {
    return TextField(
      controller: controller,
      onChanged: onChanged,
      textInputAction: TextInputAction.search,
      decoration: InputDecoration(
        hintText: 'Поиск по медиатеке',
        prefixIcon: const Icon(Icons.search_rounded, size: 20),
        suffixIcon: controller.text.isEmpty
            ? null
            : IconButton(
                icon: const Icon(Icons.close_rounded, size: 18),
                onPressed: onClear,
              ),
        filled: true,
        fillColor: AppColors.surface2,
        contentPadding: const EdgeInsets.symmetric(vertical: 14),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(999),
          borderSide: BorderSide.none,
        ),
      ),
    );
  }
}

/// Фасеты по источникам. Счётчики приходят с сервера и не зависят от того,
/// какой источник выбран сейчас: иначе выбор одного обнулял бы остальные
/// чипы, и вернуться было бы некуда.
class _SourceFacets extends StatelessWidget {
  const _SourceFacets({
    required this.sources,
    required this.selected,
    required this.onSelect,
  });
  final Map<String, int> sources;
  final String selected;
  final ValueChanged<String> onSelect;

  @override
  Widget build(BuildContext context) {
    final entries = sources.entries.toList()
      ..sort((a, b) => b.value.compareTo(a.value));
    return SizedBox(
      height: 34,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        itemCount: entries.length,
        separatorBuilder: (_, _) => const SizedBox(width: 8),
        itemBuilder: (context, index) {
          final entry = entries[index];
          final active = selected == entry.key;
          final color = sourceColor(entry.key, context);
          return GestureDetector(
            onTap: () => onSelect(entry.key),
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 12),
              decoration: BoxDecoration(
                color: active
                    ? color.withValues(alpha: 0.18)
                    : AppColors.surface2,
                border: Border.all(
                  color: active
                      ? color.withValues(alpha: 0.55)
                      : AppColors.border,
                ),
                borderRadius: BorderRadius.circular(999),
              ),
              child: Row(
                children: [
                  SourceIcon(providerId: entry.key, size: 15),
                  const SizedBox(width: 7),
                  Text(
                    '${sourceName(entry.key)} · ${entry.value}',
                    style: TextStyle(
                      fontSize: 12.5,
                      fontWeight: active ? FontWeight.w700 : FontWeight.w500,
                      color: active ? AppColors.fg : AppColors.muted,
                    ),
                  ),
                ],
              ),
            ),
          );
        },
      ),
    );
  }
}

class _GroupCard extends StatelessWidget {
  const _GroupCard({
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.onTap,
  });
  final IconData icon;
  final String title;
  final String subtitle;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final accent = Theme.of(context).colorScheme.primary;
    return Material(
      color: AppColors.surface2,
      borderRadius: BorderRadius.circular(AppRadius.card),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(AppRadius.card),
        child: Padding(
          padding: const EdgeInsets.all(14),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Container(
                width: 34,
                height: 34,
                decoration: BoxDecoration(
                  color: accent.withValues(alpha: 0.14),
                  borderRadius: BorderRadius.circular(10),
                ),
                child: Icon(icon, size: 18, color: accent),
              ),
              Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontSize: 14.5,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  const SizedBox(height: 3),
                  Text(
                    subtitle,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}
