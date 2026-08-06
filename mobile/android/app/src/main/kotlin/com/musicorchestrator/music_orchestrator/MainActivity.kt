package com.musicorchestrator.music_orchestrator

import com.ryanheise.audioservice.AudioServiceActivity

// audio_service needs the single Flutter activity to be an AudioServiceActivity,
// otherwise the media session dies with the activity and the notification
// buttons stop reaching Dart.
class MainActivity : AudioServiceActivity()
