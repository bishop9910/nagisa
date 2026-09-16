package server

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"nagisa/internal/conf"

	"github.com/go-kratos/kratos/v3/log"
)

// NewWebHandler serves the built netdisk front end from the Kratos HTTP
// server. It is the attachment point for the SPA: everything the API does not
// claim is resolved against the configured directory, and an unknown path
// falls back to the index document so client side routing works.
//
// When the directory is missing the handler answers with a short notice
// instead of failing startup, so the API can be deployed before the front end
// is built.
func NewWebHandler(c *conf.Web) http.Handler {
	root := c.GetRoot()
	if root == "" {
		root = "./web/dist"
	}
	index := c.GetIndex()
	if index == "" {
		index = "index.html"
	}
	maxAge := c.GetCacheMaxAge().AsDuration()
	spa := c.GetSpaFallback()

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	if _, err := os.Stat(absRoot); err != nil {
		log.Warn("web root is not present, the API is served without a front end", "root", absRoot)
	}

	indexPath := filepath.Join(absRoot, filepath.FromSlash(index))
	fileServer := http.FileServer(http.Dir(absRoot))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		requested := path.Clean("/" + r.URL.Path)
		// Reject traversal before it reaches the file server.
		if strings.Contains(requested, "..") {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		// An unknown API path must stay a 404. Falling back to the index
		// document here would answer a typo in a client with 200 and HTML,
		// which is far harder to diagnose.
		if isAPIPath(requested) {
			http.NotFound(w, r)
			return
		}
		full := filepath.Join(absRoot, filepath.FromSlash(requested))
		info, err := os.Stat(full)
		switch {
		case err == nil && !info.IsDir():
			// Hashed asset names are safe to cache for a long time; anything
			// else is revalidated so a deploy is picked up immediately.
			if maxAge > 0 && isImmutable(requested) {
				w.Header().Set("Cache-Control", "public, max-age="+itoaSeconds(maxAge))
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		case err == nil && info.IsDir():
			serveIndex(w, r, indexPath, maxAge)
			return
		case spa:
			serveIndex(w, r, indexPath, maxAge)
			return
		default:
			http.NotFound(w, r)
		}
	})
}

// isAPIPath reports whether a path belongs to the API rather than to the front
// end, and therefore must never fall back to the index document.
func isAPIPath(p string) bool {
	for _, prefix := range []string{"/v1/", "/docs/"} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// serveIndex writes the index document, or a placeholder when the front end
// has not been built yet.
func serveIndex(w http.ResponseWriter, r *http.Request, indexPath string, maxAge time.Duration) {
	if _, err := os.Stat(indexPath); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(placeholderPage))
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, indexPath)
}

// isImmutable reports whether a path looks like a content addressed asset.
func isImmutable(p string) bool {
	base := path.Base(p)
	dot := strings.LastIndex(base, ".")
	if dot <= 0 {
		return false
	}
	stem := base[:dot]
	dash := strings.LastIndex(stem, ".")
	if dash < 0 {
		dash = strings.LastIndex(stem, "-")
	}
	if dash < 0 || dash == len(stem)-1 {
		return false
	}
	hash := stem[dash+1:]
	if len(hash) < 8 {
		return false
	}
	for _, r := range hash {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func itoaSeconds(d time.Duration) string {
	seconds := int64(d.Seconds())
	if seconds <= 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for seconds > 0 {
		i--
		buf[i] = byte('0' + seconds%10)
		seconds /= 10
	}
	return string(buf[i:])
}

// placeholderPage is served while the front end has not been built.
const placeholderPage = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Nagisa 网盘</title>
<style>
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;
background:#0f1115;color:#e6e8eb;font:16px/1.7 system-ui,-apple-system,"Segoe UI",sans-serif}
main{max-width:36rem;padding:3rem 2rem;text-align:center}
h1{font-size:1.5rem;margin:0 0 1rem}
code{background:#1b1f27;border-radius:6px;padding:.15rem .4rem}
a{color:#7aa2f7}
p{color:#9aa4b2}
</style>
</head>
<body>
<main>
<h1>Nagisa 网盘后端已就绪</h1>
<p>前端尚未构建。把构建产物放入 <code>web/dist</code>，或在 <code>configs/config.yaml</code> 的 <code>web.root</code> 指向其他目录。</p>
<p>接口文档：<a href="/docs/">/docs/</a> · OpenAPI：<a href="/docs/openapi.yaml">/docs/openapi.yaml</a></p>
</main>
</body>
</html>
`
