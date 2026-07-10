import pytest
from fastapi.testclient import TestClient

from music_orchestrator.main import create_app


@pytest.fixture()
def client(tmp_path, monkeypatch):
    monkeypatch.setenv("APP_DATABASE_URL", f"sqlite:///{tmp_path / 'test.db'}")
    monkeypatch.setenv("APP_API_KEYS", "test-key")
    monkeypatch.setenv("APP_ENABLE_RISKY_EXTRACTORS", "false")
    monkeypatch.setenv("APP_YOUTUBE_API_KEY", "")
    monkeypatch.setenv("APP_SOUNDCLOUD_CLIENT_ID", "")
    app = create_app()
    with TestClient(app) as c:
        yield c


@pytest.fixture()
def auth_headers():
    return {"X-API-Key": "test-key"}
