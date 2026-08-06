import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../api/api_client.dart';
import '../state/accent_controller.dart';
import '../state/auth_controller.dart';
import '../state/library_controller.dart';
import '../theme/tokens.dart';
import '../widgets/source_icon.dart';

class SettingsScreen extends StatelessWidget {
  const SettingsScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final accentController = context.watch<AccentController>();
    final auth = context.watch<AuthController>();
    final library = context.watch<LibraryController>();

    return ListView(
      padding: const EdgeInsets.fromLTRB(20, 20, 20, 100),
      children: [
        Text(
          'ПРОФИЛЬ И BACKEND',
          style: Theme.of(context).textTheme.labelSmall?.copyWith(
            color: Theme.of(context).colorScheme.primary,
          ),
        ),
        const SizedBox(height: 6),
        Text(
          'Настройки',
          style: Theme.of(
            context,
          ).textTheme.headlineLarge?.copyWith(fontSize: 40),
        ),
        const SizedBox(height: 8),
        const Text('Аккаунт, источники и подключение к серверу.'),

        const Divider(height: 40),
        Text(
          'ВНЕШНИЙ ВИД',
          style: Theme.of(context).textTheme.labelSmall?.copyWith(
            color: Theme.of(context).colorScheme.primary,
          ),
        ),
        const SizedBox(height: 8),
        const Text(
          'Акцентный цвет применяется сразу везде — к кнопкам, плееру и активным состояниям.',
        ),
        const SizedBox(height: 16),
        Wrap(
          spacing: 14,
          runSpacing: 14,
          children: [
            for (final preset in accentPresets)
              _Swatch(
                color: preset,
                selected: accentController.raw.toARGB32() == preset.toARGB32(),
                onTap: () => accentController.choose(preset),
              ),
            GestureDetector(
              onTap: () => _pickCustomColor(context, accentController),
              child: Container(
                width: 44,
                height: 44,
                decoration: const BoxDecoration(
                  shape: BoxShape.circle,
                  border: Border.fromBorderSide(
                    BorderSide(color: Colors.white24, width: 2),
                  ),
                ),
                child: const Icon(Icons.edit, size: 16, color: AppColors.muted),
              ),
            ),
          ],
        ),

        const Divider(height: 40),
        Text(
          'ИСТОЧНИКИ ПОИСКА',
          style: Theme.of(context).textTheme.labelSmall?.copyWith(
            color: Theme.of(context).colorScheme.primary,
          ),
        ),
        const SizedBox(height: 8),
        const Text('Где искать музыку — включи нужные источники.'),
        const SizedBox(height: 12),
        if (library.providers.isEmpty)
          const Text(
            'Providers не загрузились — проверь подключение к серверу ниже.',
            style: TextStyle(color: AppColors.muted),
          )
        else
          ...library.providers.where((p) => p.canSearch).map((p) {
            final selected = library.selectedProviderIds.contains(p.id);
            return SwitchListTile(
              contentPadding: EdgeInsets.zero,
              value: selected && p.enabled,
              onChanged: p.enabled ? (_) => library.toggleProvider(p.id) : null,
              title: Row(
                children: [
                  SourceIcon(providerId: p.id, size: 18),
                  const SizedBox(width: 8),
                  Text(sourceName(p.id)),
                ],
              ),
              subtitle: Text(
                p.enabled
                    ? (selected ? 'Включён' : 'Выключен')
                    : 'Недоступен на сервере',
                style: Theme.of(context).textTheme.bodySmall,
              ),
            );
          }),

        const Divider(height: 40),
        Text(
          'АККАУНТ',
          style: Theme.of(context).textTheme.labelSmall?.copyWith(
            color: Theme.of(context).colorScheme.primary,
          ),
        ),
        const SizedBox(height: 12),
        _AccountSection(auth: auth),

        if (auth.status == AuthStatus.authenticated &&
            auth.session!.isAdmin) ...[
          const Divider(height: 40),
          Text(
            'ПОЛЬЗОВАТЕЛИ',
            style: Theme.of(context).textTheme.labelSmall?.copyWith(
              color: Theme.of(context).colorScheme.primary,
            ),
          ),
          const SizedBox(height: 12),
          _UsersSection(auth: auth),
        ],

        const Divider(height: 40),
        Text(
          'ПОДКЛЮЧЕНИЕ К СЕРВЕРУ',
          style: Theme.of(context).textTheme.labelSmall?.copyWith(
            color: Theme.of(context).colorScheme.primary,
          ),
        ),
        const SizedBox(height: 12),
        _BackendSection(auth: auth, library: library),

        const SizedBox(height: 32),
        Center(
          child: Text(
            'Music Orchestrator v0.3.2',
            style: TextStyle(color: AppColors.subtle, fontSize: 12),
          ),
        ),
      ],
    );
  }

  Future<void> _pickCustomColor(
    BuildContext context,
    AccentController controller,
  ) async {
    double hue = HSLColor.fromColor(controller.raw).hue;
    await showModalBottomSheet(
      context: context,
      backgroundColor: AppColors.surface2,
      builder: (context) => StatefulBuilder(
        builder: (context, setState) => Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                'Свой акцентный цвет',
                style: Theme.of(context).textTheme.titleLarge,
              ),
              const SizedBox(height: 16),
              Container(
                width: 60,
                height: 60,
                decoration: BoxDecoration(
                  color: HSLColor.fromAHSL(1, hue, 0.85, 0.55).toColor(),
                  shape: BoxShape.circle,
                ),
              ),
              Slider(
                value: hue,
                max: 360,
                onChanged: (v) {
                  setState(() => hue = v);
                  controller.choose(
                    HSLColor.fromAHSL(1, hue, 0.85, 0.55).toColor(),
                  );
                },
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _Swatch extends StatelessWidget {
  const _Swatch({
    required this.color,
    required this.selected,
    required this.onTap,
  });
  final Color color;
  final bool selected;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) => GestureDetector(
    onTap: onTap,
    child: Container(
      width: 44,
      height: 44,
      decoration: BoxDecoration(
        color: color,
        shape: BoxShape.circle,
        border: Border.all(
          color: selected ? Colors.white : Colors.white24,
          width: 2,
        ),
      ),
    ),
  );
}

class _AccountSection extends StatefulWidget {
  const _AccountSection({required this.auth});
  final AuthController auth;
  @override
  State<_AccountSection> createState() => _AccountSectionState();
}

class _AccountSectionState extends State<_AccountSection> {
  final _username = TextEditingController();
  final _password = TextEditingController();
  bool _saving = false;

  @override
  void didUpdateWidget(covariant _AccountSection oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.auth.session?.username != null && _username.text.isEmpty) {
      _username.text = widget.auth.session!.username!;
    }
  }

  @override
  Widget build(BuildContext context) {
    final auth = widget.auth;
    if (auth.status != AuthStatus.authenticated || auth.session == null) {
      return Text(
        'Не выполнен вход — см. «Подключение к серверу» ниже.',
        style: Theme.of(context).textTheme.bodySmall,
      );
    }
    final session = auth.session!;
    if (_username.text.isEmpty && session.username != null) {
      _username.text = session.username!;
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text(
              '${session.username ?? 'Аккаунт'} · ${session.isAdmin ? 'администратор' : 'пользователь'}',
            ),
            OutlinedButton.icon(
              onPressed: auth.logout,
              icon: const Icon(Icons.logout, size: 16),
              label: const Text('Выйти'),
            ),
          ],
        ),
        if (session.isAdmin) ...[
          const SizedBox(height: 16),
          TextField(
            controller: _username,
            decoration: const InputDecoration(labelText: 'Логин'),
          ),
          const SizedBox(height: 10),
          TextField(
            controller: _password,
            obscureText: true,
            decoration: const InputDecoration(
              labelText: 'Новый пароль — необязательно',
            ),
          ),
          const SizedBox(height: 12),
          ElevatedButton(
            onPressed: _saving
                ? null
                : () async {
                    setState(() => _saving = true);
                    final ok = await auth.updateAccount(
                      username: _username.text.trim(),
                      password: _password.text.isEmpty ? null : _password.text,
                    );
                    setState(() => _saving = false);
                    if (context.mounted) {
                      ScaffoldMessenger.of(context).showSnackBar(
                        SnackBar(
                          content: Text(
                            ok
                                ? 'Аккаунт обновлён'
                                : (auth.error ?? 'Не удалось сохранить'),
                          ),
                        ),
                      );
                    }
                  },
            child: _saving
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Text('Сохранить аккаунт'),
          ),
        ],
      ],
    );
  }
}

class _UsersSection extends StatefulWidget {
  const _UsersSection({required this.auth});
  final AuthController auth;
  @override
  State<_UsersSection> createState() => _UsersSectionState();
}

class _UsersSectionState extends State<_UsersSection> {
  final _username = TextEditingController();
  final _password = TextEditingController();

  @override
  Widget build(BuildContext context) {
    final auth = widget.auth;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text(
          'Новые пользователи получают роль «пользователь» и не видят этот раздел.',
          style: TextStyle(color: AppColors.muted, fontSize: 13),
        ),
        const SizedBox(height: 12),
        Row(
          children: [
            Expanded(
              child: TextField(
                controller: _username,
                decoration: const InputDecoration(labelText: 'Логин'),
              ),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: TextField(
                controller: _password,
                obscureText: true,
                decoration: const InputDecoration(labelText: 'Пароль'),
              ),
            ),
          ],
        ),
        const SizedBox(height: 10),
        ElevatedButton(
          onPressed: () async {
            if (_username.text.trim().isEmpty || _password.text.length < 10) {
              return;
            }
            final ok = await auth.createUser(
              _username.text.trim(),
              _password.text,
            );
            if (ok) {
              _username.clear();
              _password.clear();
            }
          },
          child: const Text('Добавить'),
        ),
        const SizedBox(height: 16),
        for (final user in auth.users)
          ListTile(
            contentPadding: EdgeInsets.zero,
            title: Text(user.username),
            trailing: IconButton(
              icon: const Icon(Icons.delete_outline, color: AppColors.danger),
              onPressed: () => auth.deleteUser(user.id),
            ),
          ),
        if (auth.users.isEmpty)
          const Text(
            'Пользователей пока нет.',
            style: TextStyle(color: AppColors.muted),
          ),
      ],
    );
  }
}

class _BackendSection extends StatefulWidget {
  const _BackendSection({required this.auth, required this.library});
  final AuthController auth;
  final LibraryController library;
  @override
  State<_BackendSection> createState() => _BackendSectionState();
}

class _BackendSectionState extends State<_BackendSection> {
  late final _url = TextEditingController(
    text: context.read<ApiClient>().baseUrl,
  );
  late final _key = TextEditingController(
    text: context.read<ApiClient>().apiKey,
  );
  String? _status;
  bool _busy = false;

  Future<void> _test() async {
    setState(() {
      _busy = true;
      _status = 'Проверяю /health…';
    });
    try {
      final api = context.read<ApiClient>();
      final res = await api.session();
      _status =
          'Backend отвечает. Сессия: ${res.authenticated ? 'вошли как ${res.username}' : 'не авторизован'}.';
    } catch (e) {
      _status = 'Backend не ответил: $e';
    }
    setState(() => _busy = false);
  }

  Future<void> _save() async {
    setState(() => _busy = true);
    final api = context.read<ApiClient>();
    await api.updateConnection(baseUrl: _url.text, apiKey: _key.text);
    await widget.auth.refresh();
    await widget.library.refreshAll();
    setState(() {
      _busy = false;
      _status = 'Сохранено и переподключено.';
    });
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text(
          'На физическом телефоне 127.0.0.1 указывает на сам телефон. Подключи телефон и Mac к одной Wi‑Fi сети и укажи здесь LAN-адрес Mac, например http://192.168.1.20:18080.',
          style: TextStyle(color: AppColors.muted, fontSize: 13),
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _url,
          decoration: const InputDecoration(
            labelText: 'Адрес API',
            hintText: 'http://127.0.0.1:18080',
          ),
        ),
        const SizedBox(height: 10),
        TextField(
          controller: _key,
          decoration: const InputDecoration(
            labelText:
                'X-API-Key (необязательно — для входа используй логин выше)',
          ),
        ),
        const SizedBox(height: 12),
        Row(
          children: [
            Expanded(
              child: OutlinedButton(
                onPressed: _busy ? null : _test,
                child: const Text('Проверить'),
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: ElevatedButton(
                onPressed: _busy ? null : _save,
                child: const Text('Сохранить'),
              ),
            ),
          ],
        ),
        if (_status != null)
          Padding(
            padding: const EdgeInsets.only(top: 10),
            child: Text(_status!, style: Theme.of(context).textTheme.bodySmall),
          ),
      ],
    );
  }
}
