def test_favorites_crud(client, auth_headers):
    created = client.post("/v1/favorites", headers=auth_headers, json={"track_id": "local:seed-1"})
    assert created.status_code == 201
    assert created.json()["track_id"] == "local:seed-1"

    listing = client.get("/v1/favorites", headers=auth_headers)
    assert listing.status_code == 200
    assert listing.json()["total"] == 1

    deleted = client.delete("/v1/favorites/local:seed-1", headers=auth_headers)
    assert deleted.status_code == 204
    assert client.get("/v1/favorites", headers=auth_headers).json()["total"] == 0


def test_playlists_crud_and_track_membership(client, auth_headers):
    created = client.post("/v1/playlists", headers=auth_headers, json={"name": "Road", "description": "demo"})
    assert created.status_code == 201
    playlist_id = created.json()["id"]

    add = client.post(f"/v1/playlists/{playlist_id}/tracks", headers=auth_headers, json={"track_id": "local:seed-1"})
    assert add.status_code == 201

    listing = client.get("/v1/playlists", headers=auth_headers)
    assert listing.status_code == 200
    assert listing.json()["items"][0]["track_count"] == 1

    detail = client.get(f"/v1/playlists/{playlist_id}", headers=auth_headers)
    assert detail.status_code == 200
    assert detail.json()["tracks"][0]["track_id"] == "local:seed-1"
