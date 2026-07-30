import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, MusicAPIError } from './client';

const jsonResponse = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' }, ...init });

describe('api client', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('builds search query with comma-separated provider filters', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      jsonResponse({ query: 'demo', limit: 20, offset: 0, total: 0, items: [] }),
    );

    await api.search('demo', ['local', 'youtube_stream'], 10, 5);

    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).toContain('/v1/search?');
    expect(url).toContain('q=demo');
    expect(url).toContain('limit=10');
    expect(url).toContain('offset=5');
    expect(decodeURIComponent(url)).toContain('providers=local,youtube_stream');
  });

  it('sends API key header only for protected download endpoint', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      jsonResponse({ id: 'job_1', type: 'download', status: 'succeeded', payload: {}, created_at: '', updated_at: '' }),
    );

    await api.createDownload('youtube_stream:abc', 'mp3');

    const init = fetchMock.mock.calls[0][1] as RequestInit;
    const headers = init.headers as Headers;
    expect(headers.get('Content-Type')).toBe('application/json');
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ track_id: 'youtube_stream:abc', format: 'mp3' }));
  });

  it('posts first-run owner registration payload', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      jsonResponse({ authenticated: true, setup_required: false, totp_required: false, totp_enabled: false, login_enabled: true }),
    );

    await api.register('admin', 'strong-pass-123', 'JBSWY3DPEHPK3PXP');

    const url = String(fetchMock.mock.calls[0][0]);
    const init = fetchMock.mock.calls[0][1] as RequestInit;
    expect(url).toContain('/v1/auth/register');
    expect(init.method).toBe('POST');
    expect(init.credentials).toBe('include');
    expect(init.body).toBe(JSON.stringify({ username: 'admin', password: 'strong-pass-123', totp_secret: 'JBSWY3DPEHPK3PXP' }));
  });

  it('normalizes backend error responses', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      jsonResponse({ error: { code: 'bad_request', message: 'Invalid track id' } }, { status: 400 }),
    );

    await expect(api.playback('bad')).rejects.toMatchObject(
      new MusicAPIError('bad_request', 'Invalid track id'),
    );
  });
});
