# Contributing

## Development

1. Create a virtual environment.
2. Install dev dependencies: `pip install -e '.[dev]'`.
3. Copy `.env.example` to `.env` for local-only settings.
4. Run tests: `pytest -q`.
5. Run lint: `ruff check .`.

## Rules

- Keep provider adapters capability-driven.
- Do not add risky extraction/download/cache behavior unless it is explicitly gated, disabled by default, documented, and tested.
- Do not commit secrets, cookies, local domains, tokens, or real credentials.
- Add tests for auth, validation, policy, favorites/playlists, jobs, and search merge behavior when touching those areas.

## API compatibility

The frontend contract is the OpenAPI schema at `/openapi.json`. Schema-affecting changes must update `docs/API.en.md` and `docs/API.ru.md`.
