import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const frontendPort = Number(env.VITE_DEV_PORT || env.FRONTEND_PORT || '3000')
  const backendOrigin = env.VITE_BACKEND_ORIGIN || env.BACKEND_ORIGIN || 'http://localhost:3001'

  return {
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
              url.startsWith('/private-track') ||
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
      port: frontendPort,
      strictPort: true,
      proxy: {
        '/api': {
          target: backendOrigin,
          changeOrigin: true,
        },
        '/static': {
          target: backendOrigin,
          changeOrigin: true,
        },
        '/logout': {
          target: backendOrigin,
          changeOrigin: true,
        },
        '/login': {
          target: backendOrigin,
          changeOrigin: true,
        },
        '/pre-enrolment': {
          target: backendOrigin,
          changeOrigin: true,
        },
        '/private-track': {
          target: backendOrigin,
          changeOrigin: true,
        },
        '/classes': {
          target: backendOrigin,
          changeOrigin: true,
        },
        '/finance': {
          target: backendOrigin,
          changeOrigin: true,
        },
      },
    },
  }
})
