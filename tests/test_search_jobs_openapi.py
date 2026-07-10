def test_search_merges_and_dedupes_provider_results(client):
    response = client.get("/v1/search", params={"q": "demo song", "providers": "local,youtube_official,soundcloud_official"})
    assert response.status_code == 200
    body = response.json()
    assert body["query"] == "demo song"
    assert body["total"] >= 1
    first = body["items"][0]
    assert first["provider_results"]
    assert first["capabilities"]["server_favorites"] is True
    merged = [item for item in body["items"] if item["title"].lower() == "demo song"]
    assert len(merged) == 1
    assert len(merged[0]["provider_results"]) == 2
    assert {r["provider_id"] for r in merged[0]["provider_results"]} == {"local"}


def test_tracks_endpoint_returns_capability_and_policy(client):
    response = client.get("/v1/tracks/local:seed-1")
    assert response.status_code == 200
    body = response.json()
    assert body["id"] == "local:seed-1"
    assert body["capabilities"]["raw_audio_stream"] is True
    assert body["policy"]["cache_allowed"] is True


def test_jobs_abstraction(client, auth_headers):
    created = client.post("/v1/jobs", headers=auth_headers, json={"type": "resolve", "track_id": "youtube_official:demo-video"})
    assert created.status_code == 202
    job_id = created.json()["id"]
    assert created.json()["status"] == "queued"

    listing = client.get("/v1/jobs", headers=auth_headers)
    assert listing.status_code == 200
    assert listing.json()["total"] == 1

    detail = client.get(f"/v1/jobs/{job_id}", headers=auth_headers)
    assert detail.status_code == 200
    assert detail.json()["type"] == "resolve"


def test_openapi_contains_expected_routes(client):
    response = client.get("/openapi.json")
    assert response.status_code == 200
    paths = response.json()["paths"]
    for path in ["/health", "/v1/search", "/v1/tracks/{track_id}", "/v1/playback/{track_id}", "/v1/favorites", "/v1/playlists", "/v1/jobs", "/v1/providers"]:
        assert path in paths
