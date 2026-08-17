/// <reference types="node" />
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

const backend = process.env.ABP_BACKEND ?? 'http://127.0.0.1:8080';

// The dev server proxies API/auth/ingest/health to board-server so the
// frontend can use same-origin requests (spec 17.1: CORS off, same-origin).
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: true,
    port: 5173,
    proxy: {
      '/api': { target: backend, changeOrigin: true },
      '/auth': { target: backend, changeOrigin: true },
      '/ingest': { target: backend, changeOrigin: true },
      '/health': { target: backend, changeOrigin: true },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
  },
});
