import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../api/models.dart';
import '../services/folder_import.dart';
import '../state/library_controller.dart';
import '../theme/tokens.dart';
import '../util/plural.dart';

/// Импорт музыки в медиатеку сервера: файлы или папка целиком.
///
/// Лист живёт одним из трёх состояний — выбор, загрузка, итог, — и переход
/// между ними не закрывает лист. Загрузка тысячи файлов идёт минутами, и
/// экран, который в этот момент нечего показать, пугает сильнее ожидания.
Future<void> showImportSheet(BuildContext context) => showModalBottomSheet(
  context: context,
  isScrollControlled: true,
  backgroundColor: AppColors.surface,
  shape: const RoundedRectangleBorder(
    borderRadius: BorderRadius.vertical(top: Radius.circular(AppRadius.sheet)),
  ),
  builder: (_) => const _ImportSheet(),
);

class _ImportSheet extends StatefulWidget {
  const _ImportSheet();
  @override
  State<_ImportSheet> createState() => _ImportSheetState();
}

class _ImportSheetState extends State<_ImportSheet> {
  List<UploadFile> _picked = [];
  String? _source; // что именно выбрали — показываем пользователю
  bool _busy = false;
  int _done = 0;
  ImportResult? _result;
  String? _error;

  Future<void> _pickFiles() async {
    setState(() => _error = null);
    try {
      final picked = await FilePicker.platform.pickFiles(
        allowMultiple: true,
        type: FileType.audio,
        // Веб отдаёт байты, натив — путь; читать гигабайты в память нельзя,
        // поэтому на нативе просим именно путь.
        withData: false,
        withReadStream: false,
      );
      if (picked == null || !mounted) return;
      setState(() {
        _picked = picked.files
            .map(
              (file) => UploadFile(
                name: file.name,
                path: file.path,
                bytes: file.bytes,
              ),
            )
            .toList();
        _source = 'Выбрано файлов';
        _result = null;
      });
    } catch (e) {
      if (mounted) setState(() => _error = 'Не удалось открыть файлы: $e');
    }
  }

  Future<void> _pickFolder() async {
    setState(() => _error = null);
    try {
      final root = await FilePicker.platform.getDirectoryPath();
      if (root == null || !mounted) return;
      setState(() => _busy = true);
      final files = await collectFolder(root);
      if (!mounted) return;
      setState(() {
        _picked = files;
        _source = 'Папка целиком';
        _result = null;
        _busy = false;
        if (files.isEmpty) _error = 'В этой папке не нашлось аудиофайлов.';
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _busy = false;
        _error = 'Не удалось прочитать папку: $e';
      });
    }
  }

  Future<void> _upload() async {
    final library = context.read<LibraryController>();
    setState(() {
      _busy = true;
      _done = 0;
      _error = null;
    });
    final result = await library.importFiles(
      _picked,
      onProgress: (done, total) {
        if (mounted) setState(() => _done = done);
      },
    );
    if (!mounted) return;
    setState(() {
      _busy = false;
      _result = result;
      _error = result == null ? library.actionError : null;
      if (result != null) _picked = [];
    });
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Padding(
      padding: EdgeInsets.only(
        left: 20,
        right: 20,
        top: 14,
        bottom: MediaQuery.of(context).viewInsets.bottom + 24,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Center(
            child: Container(
              width: 38,
              height: 4,
              decoration: BoxDecoration(
                color: AppColors.borderStrong,
                borderRadius: BorderRadius.circular(999),
              ),
            ),
          ),
          const SizedBox(height: 18),
          Text('Импорт музыки', style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 6),
          Text(
            'Файлы уедут на сервер и появятся в медиатеке на всех устройствах.',
            style: Theme.of(context).textTheme.bodySmall,
          ),
          const SizedBox(height: 18),
          if (_result != null)
            _ResultCard(result: _result!)
          else if (_busy && _picked.isNotEmpty)
            _ProgressCard(done: _done, total: _picked.length)
          else ...[
            _PickTile(
              icon: Icons.audio_file_rounded,
              title: 'Выбрать файлы',
              subtitle: 'Один трек или сразу несколько',
              onTap: _busy ? null : _pickFiles,
            ),
            if (folderImportSupported) ...[
              const SizedBox(height: 10),
              _PickTile(
                icon: Icons.folder_open_rounded,
                title: 'Выбрать папку',
                subtitle: 'Со всеми вложенными папками',
                onTap: _busy ? null : _pickFolder,
                busy: _busy,
              ),
            ],
          ],
          if (_error != null) ...[
            const SizedBox(height: 14),
            Text(
              _error!,
              style: Theme.of(
                context,
              ).textTheme.bodySmall?.copyWith(color: AppColors.danger),
            ),
          ],
          if (_picked.isNotEmpty && !_busy) ...[
            const SizedBox(height: 16),
            Text(
              '$_source: ${_picked.length} ${plural(_picked.length, 'файл', 'файла', 'файлов')}',
              style: Theme.of(context).textTheme.bodyMedium,
            ),
            const SizedBox(height: 12),
            SizedBox(
              width: double.infinity,
              child: FilledButton(
                onPressed: _upload,
                style: FilledButton.styleFrom(
                  backgroundColor: scheme.primary,
                  foregroundColor: scheme.onPrimary,
                  padding: const EdgeInsets.symmetric(vertical: 15),
                ),
                child: const Text('Загрузить в медиатеку'),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _PickTile extends StatelessWidget {
  const _PickTile({
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.onTap,
    this.busy = false,
  });
  final IconData icon;
  final String title;
  final String subtitle;
  final VoidCallback? onTap;
  final bool busy;

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
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
          child: Row(
            children: [
              Container(
                width: 42,
                height: 42,
                decoration: BoxDecoration(
                  color: accent.withValues(alpha: 0.14),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: busy
                    ? Padding(
                        padding: const EdgeInsets.all(12),
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: accent,
                        ),
                      )
                    : Icon(icon, color: accent, size: 21),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      style: const TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      subtitle,
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ],
                ),
              ),
              const Icon(Icons.chevron_right_rounded, color: AppColors.subtle),
            ],
          ),
        ),
      ),
    );
  }
}

class _ProgressCard extends StatelessWidget {
  const _ProgressCard({required this.done, required this.total});
  final int done;
  final int total;

  @override
  Widget build(BuildContext context) {
    final accent = Theme.of(context).colorScheme.primary;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: AppColors.surface2,
        borderRadius: BorderRadius.circular(AppRadius.card),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Загружено $done из $total',
            style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 12),
          ClipRRect(
            borderRadius: BorderRadius.circular(999),
            child: LinearProgressIndicator(
              value: total == 0 ? null : done / total,
              minHeight: 6,
              backgroundColor: AppColors.surface3,
              color: accent,
            ),
          ),
          const SizedBox(height: 12),
          Text(
            'Не закрывай приложение, пока идёт загрузка.',
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ],
      ),
    );
  }
}

class _ResultCard extends StatelessWidget {
  const _ResultCard({required this.result});
  final ImportResult result;

  @override
  Widget build(BuildContext context) {
    final accent = Theme.of(context).colorScheme.primary;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: AppColors.surface2,
        borderRadius: BorderRadius.circular(AppRadius.card),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(Icons.check_circle_rounded, color: accent, size: 20),
              const SizedBox(width: 8),
              Text(
                'Добавлено ${result.imported} ${plural(result.imported, 'трек', 'трека', 'треков')}',
                style: const TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
          ),
          if (result.duplicate > 0) ...[
            const SizedBox(height: 8),
            Text(
              'Уже были в медиатеке: ${result.duplicate}',
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ],
          // Пропущенное показываем поимённо: молча потерянный файл — это
          // ровно тот случай, когда пользователь думает, что импорт сломан.
          if (result.skipped.isNotEmpty) ...[
            const SizedBox(height: 12),
            Text(
              'Не взяли ${result.skipped.length}:',
              style: Theme.of(
                context,
              ).textTheme.bodySmall?.copyWith(color: AppColors.danger),
            ),
            const SizedBox(height: 6),
            ConstrainedBox(
              constraints: const BoxConstraints(maxHeight: 160),
              child: ListView(
                shrinkWrap: true,
                children: [
                  for (final skip in result.skipped)
                    Padding(
                      padding: const EdgeInsets.only(bottom: 6),
                      child: Text(
                        '${skip.path} — ${skip.reason}',
                        style: Theme.of(context).textTheme.bodySmall,
                      ),
                    ),
                ],
              ),
            ),
          ],
        ],
      ),
    );
  }
}
