import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

import pkg from './package.json';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  define: { __APP_VERSION__: JSON.stringify(pkg.version) },
  resolve: { alias: { '@': new URL('./src', import.meta.url).pathname } },
  preview: {
    host: '0.0.0.0',
    port: 5173,
    allowedHosts: ['music.mibombopussiclat.ru', 'localhost', '127.0.0.1'],
  },
  server: {
    port: 5173,
    allowedHosts: ['music.mibombopussiclat.ru', 'localhost', '127.0.0.1'],
    proxy: {
      '/health': 'http://127.0.0.1:18080',
      '/openapi.json': 'http://127.0.0.1:18080',
      '/v1': 'http://127.0.0.1:18080',
      '/media': 'http://127.0.0.1:18080',
    },
  },
});
