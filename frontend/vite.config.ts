import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [
    react(),
    {
      name: 'redirect-non-app-paths',
      configureServer(server) {
        server.middlewares.use((req, res, next) => {
          const url = req.url || '/'
          if (
            url.startsWith('/app') ||
            url.startsWith('/api') ||
            url.startsWith('/static') ||
            url.startsWith('/@') ||
            url.startsWith('/node_modules') ||
            url.startsWith('/favicon') ||
            url.startsWith('/login') ||
            url.startsWith('/logout') ||
            url.startsWith('/pre-enrolment') ||
            url.startsWith('/classes') ||
            url.startsWith('/finance')
          ) {
            next()
            return
          }
          res.statusCode = 302
          res.setHeader('Location', url === '/' ? '/app' : `/app${url}`)
          res.end()
        })
      },
    },
  ],
  base: '/app/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 3000,
    strictPort: true,
    proxy: {
      '/api': {
        target: 'http://localhost:3001',
        changeOrigin: true,
      },
      '/static': {
        target: 'http://localhost:3001',
        changeOrigin: true,
      },
      '/logout': {
        target: 'http://localhost:3001',
        changeOrigin: true,
      },
      '/login': {
        target: 'http://localhost:3001',
        changeOrigin: true,
      },
      '/pre-enrolment': {
        target: 'http://localhost:3001',
        changeOrigin: true,
      },
      '/classes': {
        target: 'http://localhost:3001',
        changeOrigin: true,
      },
      '/finance': {
        target: 'http://localhost:3001',
        changeOrigin: true,
      },
    },
  },
})
