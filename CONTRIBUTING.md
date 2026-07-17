# Contributing

## Local setup

```bash
cp .env.example .env
go test ./...
go run .
```

## Quality gate

Before opening a PR:

```bash
gofmt -w .
go test ./...
```

## Rules

- Do not commit secrets or `.env`.
- Keep extractor mode opt-in and disabled by default.
- Update `docs/FRONTEND-CONTRACT.md` when changing frontend-facing responses.
- Prefer small, tested changes.
