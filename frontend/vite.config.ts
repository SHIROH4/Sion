import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import { resolve } from 'path'

declare let process: { env: Record<string, string | undefined> };
const isTest = process.env.VITEST !== undefined

export default defineConfig({
  plugins: [!isTest && react()].filter(Boolean),
  build: {
    rollupOptions: {
      input: {
        pet: resolve(__dirname, 'index.html'),
        settings: resolve(__dirname, 'settings.html'),
      },
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/test-setup.ts',
  },
})
