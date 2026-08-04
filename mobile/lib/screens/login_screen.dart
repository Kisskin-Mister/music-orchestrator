import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../state/auth_controller.dart';
import '../state/library_controller.dart';
import '../theme/tokens.dart';

/// Mirrors LoginScreen.tsx's setup/login forms. TOTP is intentionally not
/// implemented here yet (documented in mobile/README.md) — accounts created
/// through this screen have no second factor, so login stays single-step.
class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key});
  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _username = TextEditingController();
  final _password = TextEditingController();
  bool _submitting = false;

  Future<void> _submit(AuthController auth, bool isSetup) async {
    if (_username.text.trim().isEmpty ||
        _password.text.length < (isSetup ? 10 : 1)) {
      return;
    }
    setState(() => _submitting = true);
    final ok = isSetup
        ? await auth.register(_username.text.trim(), _password.text)
        : await auth.login(_username.text.trim(), _password.text);
    if (ok && mounted) await context.read<LibraryController>().refreshAll();
    if (!mounted) return;
    setState(() => _submitting = false);
    if (!ok) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(auth.error ?? 'Не удалось войти')));
    }
  }

  @override
  Widget build(BuildContext context) {
    final auth = context.watch<AuthController>();
    final isSetup = auth.status == AuthStatus.needsSetup;
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 380),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Container(
                    width: 64,
                    height: 64,
                    alignment: Alignment.center,
                    decoration: BoxDecoration(
                      color: Theme.of(context).colorScheme.primary,
                      borderRadius: BorderRadius.circular(20),
                    ),
                    child: Icon(
                      Icons.music_note_rounded,
                      color: Theme.of(context).colorScheme.onPrimary,
                      size: 30,
                    ),
                  ),
                  const SizedBox(height: 20),
                  Text(
                    isSetup ? 'Создать владельца' : 'Войти',
                    style: Theme.of(
                      context,
                    ).textTheme.headlineLarge?.copyWith(fontSize: 32),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    isSetup
                        ? 'Первый запуск: этот аккаунт получит роль администратора.'
                        : 'Аккаунт хранится на этом сервере.',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                  const SizedBox(height: 24),
                  TextField(
                    controller: _username,
                    decoration: const InputDecoration(
                      labelText: 'Логин',
                      prefixIcon: Icon(Icons.person_outline),
                    ),
                  ),
                  const SizedBox(height: 12),
                  TextField(
                    controller: _password,
                    obscureText: true,
                    decoration: InputDecoration(
                      labelText: isSetup
                          ? 'Пароль — минимум 10 символов'
                          : 'Пароль',
                      prefixIcon: const Icon(Icons.lock_outline),
                    ),
                  ),
                  const SizedBox(height: 20),
                  ElevatedButton(
                    onPressed: _submitting
                        ? null
                        : () => _submit(auth, isSetup),
                    child: _submitting
                        ? const SizedBox(
                            width: 18,
                            height: 18,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : Text(isSetup ? 'Создать аккаунт' : 'Войти'),
                  ),
                  if (auth.status == AuthStatus.backendUnreachable) ...[
                    const SizedBox(height: 16),
                    Text(
                      'Backend недоступен: ${auth.error ?? ''}',
                      style: const TextStyle(
                        color: AppColors.danger,
                        fontSize: 13,
                      ),
                      textAlign: TextAlign.center,
                    ),
                    TextButton(
                      onPressed: auth.refresh,
                      child: const Text('Повторить'),
                    ),
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
