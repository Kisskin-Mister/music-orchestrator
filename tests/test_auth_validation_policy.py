def test_write_endpoints_require_api_key(client):
    response = client.post("/v1/favorites", json={"track_id": "local:1"})
    assert response.status_code == 401
    assert response.json()["error"]["code"] == "unauthorized"


def test_search_validates_query_length(client):
    response = client.get("/v1/search", params={"q": ""})
    assert response.status_code == 422


def test_risky_extractors_disabled_by_default(client):
    response = client.get("/v1/providers")
    assert response.status_code == 200
    providers = {item["id"]: item for item in response.json()["items"]}
    assert providers["youtube_official"]["risk_level"] == "compliant"
    assert providers["youtube_official"]["capabilities"]["raw_audio_stream"] is False
    assert providers["soundcloud_official"]["capabilities"]["persistent_cache"] is False
    assert all(not provider["risky_enabled"] for provider in providers.values())


def test_playback_policy_never_fakes_external_stream(client):
    response = client.get("/v1/playback/youtube_official:demo-video")
    assert response.status_code == 200
    body = response.json()
    assert body["playback_type"] == "embed"
    assert body["stream_url"] is None
    assert body["policy"]["cache_allowed"] is False
    assert "youtube.com/embed/demo-video" in body["embed_url"]
