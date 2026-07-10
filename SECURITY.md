# Security Policy

## Supported scope

This repository is a local/personal backend-first MVP. It is not a public SaaS product and should not be exposed to the internet without a separate hardening review.

## Secrets

Do not commit `.env`, API keys, SoundCloud credentials, YouTube API keys, Navidrome tokens, cookies, or extractor credentials. Use `.env.example` placeholders only.

## Provider compliance

Default mode is compliant-only:

- `APP_ENABLE_RISKY_EXTRACTORS=false`
- no yt-dlp download/cache implementation
- no cookie extraction
- no YouTube raw-audio proxying/streaming
- no SoundCloud persistent cache/offline storage
- external providers expose capability and policy flags to the frontend

## Reporting vulnerabilities

Open a private report or issue with:

- affected endpoint
- expected vs actual behavior
- reproduction steps
- whether provider credentials or user data could be exposed
