from __future__ import annotations

from functools import lru_cache
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="APP_", env_file=".env", extra="ignore")

    app_name: str = "Music Orchestrator"
    environment: str = "local"
    database_url: str = "sqlite:///./data/music-orchestrator.db"
    api_keys: str = "dev-local-key-change-me"
    enable_risky_extractors: bool = False
    youtube_api_key: str = ""
    soundcloud_client_id: str = ""
    navidrome_base_url: str = ""
    navidrome_username: str = ""
    navidrome_token: str = ""
    cors_origins: str = "http://localhost:5173,http://localhost:3000"

    @property
    def api_key_set(self) -> set[str]:
        return {item.strip() for item in self.api_keys.split(",") if item.strip()}

    @property
    def cors_origin_list(self) -> list[str]:
        return [item.strip() for item in self.cors_origins.split(",") if item.strip()]


@lru_cache
def get_settings() -> Settings:
    return Settings()
