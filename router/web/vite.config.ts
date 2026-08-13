import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [tailwindcss(), react()],
  build: {
    sourcemap: true,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
