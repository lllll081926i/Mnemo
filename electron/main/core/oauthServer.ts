import { createServer } from 'http'
import type { BrowserWindow } from 'electron'

let server: ReturnType<typeof createServer> | null = null
let currentPort = 0

export function getOAuthPort(): number {
  return currentPort
}

export async function startOAuthServer(win: BrowserWindow): Promise<number> {
  stopOAuthServer()

  return new Promise((resolve) => {
    server = createServer((req, res) => {
      const url = req.url || ''
      if (url.startsWith('/callback')) {
        // Supabase redirects with hash fragment, but the browser strips it.
        // Supabase actually uses query params when redirect_uri doesn't match SPA origin.
        // Try query params first, then hash via JS redirect.
        const respond = (html: string) => {
          res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
          res.end(html)
        }

        respond(`<!DOCTYPE html><html><head><meta charset="utf-8"></head><body>
          <script>
            if (location.hash) {
              var p = new URLSearchParams(location.hash.slice(1));
              fetch('http://localhost:${currentPort}/token?access_token=' + encodeURIComponent(p.get('access_token') || '') + '&refresh_token=' + encodeURIComponent(p.get('refresh_token') || ''));
            } else {
              var q = new URLSearchParams(location.search);
              fetch('http://localhost:${currentPort}/token?access_token=' + encodeURIComponent(q.get('access_token') || '') + '&refresh_token=' + encodeURIComponent(q.get('refresh_token') || ''));
            }
          </script>
          登录成功，正在返回 App...
        </body></html>`)
      } else if (url.startsWith('/token')) {
        const params = new URLSearchParams(url.split('?')[1] || '')
        const access_token = params.get('access_token') || ''
        const refresh_token = params.get('refresh_token') || ''
        if (access_token) {
          win.webContents.send('auth-callback', { access_token, refresh_token })
        }
        res.writeHead(200, { 'Content-Type': 'text/plain' })
        res.end('ok')
      } else {
        res.writeHead(404)
        res.end('not found')
      }
    })

    server.listen(3003, '127.0.0.1', () => {
      const addr = server!.address()
      if (addr && typeof addr === 'object') {
        currentPort = addr.port
        resolve(currentPort)
      } else {
        resolve(0)
      }
    })
  })
}

export function stopOAuthServer() {
  if (server) {
    server.close()
    server = null
    currentPort = 0
  }
}
