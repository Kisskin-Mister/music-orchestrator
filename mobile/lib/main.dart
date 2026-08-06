import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:just_audio/just_audio.dart';
import 'package:provider/provider.dart';
import 'api/api_client.dart';
import 'services/music_audio_handler.dart';
import 'screens/login_screen.dart';
import 'screens/shell.dart';
import 'state/accent_controller.dart';
import 'state/auth_controller.dart';
import 'state/library_controller.dart';
import 'state/offline_controller.dart';
import 'state/player_controller.dart';
import 'theme/app_theme.dart';

// Same Go backend as the web app (main.go). Override at build time with
// --dart-define=API_BASE_URL=http://host:port for a device/simulator that
// isn't on localhost. API_KEY is a service-token shortcut for resource
// endpoints only — leave it empty (the default, same as the web app) and use
// the in-app login for account/user management, which needs a real session.
const _configuredApiBaseUrl = String.fromEnvironment('API_BASE_URL');
const _apiKey = String.fromEnvironment('API_KEY', defaultValue: '');

String get _apiBaseUrl {
  if (_configuredApiBaseUrl.isNotEmpty) return _configuredApiBaseUrl;
  if (kIsWeb) return Uri.base.origin;
  return 'http://127.0.0.1:18080';
}

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  // One AudioPlayer for the whole app: the media session wraps the same
  // instance the player controller drives, so lock-screen state and in-app
  // state can never drift apart.
  final audioPlayer = AudioPlayer();
  final audioHandler = await initMusicAudioHandler(audioPlayer);
  final api = ApiClient(baseUrl: _apiBaseUrl, apiKey: _apiKey);
  await api.loadPersistedSettings();
  final accent = AccentController()..load();
  final auth = AuthController(api)..refresh();
  final offline = OfflineController(api);
  final player = PlayerController(api, audio: audioPlayer, audioHandler: audioHandler)
    ..loadHistory();
  // The player prefers a device-local file when one exists; giving it the
  // offline index here keeps that lookup out of the widget tree.
  player.offline = offline;
  // Downloads saved before the offline index stored metadata (or restored on a
  // new device) get their titles back from whatever the library has loaded.
  final library = LibraryController(api);
  library.addListener(
    () => offline.adoptMetadata([...library.downloads, ...library.favorites]),
  );
  runApp(
    MultiProvider(
      providers: [
        Provider<ApiClient>.value(value: api),
        ChangeNotifierProvider.value(value: accent),
        ChangeNotifierProvider.value(value: auth),
        ChangeNotifierProvider.value(value: offline),
        ChangeNotifierProvider.value(value: library),
        ChangeNotifierProvider.value(value: player),
      ],
      child: const MusicOrchestratorApp(),
    ),
  );
}

class MusicOrchestratorApp extends StatelessWidget {
  const MusicOrchestratorApp({super.key});

  @override
  Widget build(BuildContext context) {
    final accent = context.watch<AccentController>();
    final auth = context.watch<AuthController>();
    return MaterialApp(
      title: 'Music Orchestrator',
      debugShowCheckedModeBanner: false,
      theme: buildAppTheme(accent.accent),
      home: switch (auth.status) {
        AuthStatus.loading => const Scaffold(
          body: Center(child: CircularProgressIndicator()),
        ),
        AuthStatus.authenticated || AuthStatus.offline => const AppShell(),
        AuthStatus.needsSetup ||
        AuthStatus.needsLogin ||
        AuthStatus.backendUnreachable => const LoginScreen(),
      },
    );
  }
}
