import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // Same-origin dev proxy: session cookies stay first-party and the API
      // never needs CORS for local development. Only the /admin/v1 API
      // namespace is proxied — /admin, /console/... are SPA routes that must
      // hit Vite's index.html fallback instead of the backend.
      '/v1': 'http://localhost:8000',
      '/admin/v1': 'http://localhost:8000',
      '/healthz': 'http://localhost:8000',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/test/setup.ts',
  },
});
