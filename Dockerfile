FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/music-orchestrator .

FROM debian:trixie-slim AS runtime
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl ffmpeg \
    && curl -fsSL https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp \
    && chmod +x /usr/local/bin/yt-dlp \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/music-orchestrator /usr/local/bin/music-orchestrator
COPY .env.example ./
ENV APP_ADDR=:8080
EXPOSE 8080
CMD ["music-orchestrator"]
