# Security Policy

## Supported versions

The `main` branch is supported.

## Secrets

Do not commit `.env`, API keys, cookies, media tokens, or private URLs. Configure everything through `.env.example` variables.

## Extractor mode warning

`APP_ENABLE_RISKY_EXTRACTORS=true` enables personal self-hosted YouTube/SoundCloud extraction through `yt-dlp`. This is not intended for public SaaS. Users are responsible for source terms and local law.

## Reporting issues

Open a private security advisory or contact the maintainer before publishing exploit details.
