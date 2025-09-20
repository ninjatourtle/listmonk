package main

import (
    "net/http"
    "strings"
    "github.com/labstack/echo/v4"
)

// WarmCloudLogsPage serves a minimal page that polls /api/logs and shows only WarmCloud-related lines.
func (a *App) WarmCloudLogsPage(c echo.Context) error {
    html := `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Логи прогрева (WarmCloud)</title>
  <style>
    body { font-family: system-ui, -apple-system, Segoe UI, Roboto, Arial, sans-serif; margin: 0; }
    header { padding: 12px 16px; background:#111; color:#fff; display:flex; align-items:center; justify-content:space-between; }
    main { padding: 12px 16px; }
    .row { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; white-space: pre-wrap; margin: 4px 0; }
    .muted { color: #666; }
    .ok { color: #2e7d32; }
    .warn { color: #ef6c00; }
    .err { color: #c62828; }
    .controls { font-size: 14px; }
    input[type=text] { padding: 6px 8px; border: 1px solid #ccc; border-radius: 4px; }
  </style>
  </head>
<body>
  <header>
    <strong>Логи прогрева (WarmCloud)</strong>
    <div class="controls">
      Фильтр: <input id="flt" type="text" value="WarmCloud:" />
    </div>
  </header>
  <main>
    <div id="out" class="rows"></div>
  </main>
  <script>
  (function() {
    const out = document.getElementById('out');
    const flt = document.getElementById('flt');
    let last = [];
    function clear() { out.innerHTML = ''; }
    function line(txt, cls) {
      const d = document.createElement('div');
      d.className = 'row ' + (cls||'');
      d.textContent = txt;
      out.appendChild(d);
      // Keep last 1000 lines
      const max = 1000; if (out.children.length > max) { out.removeChild(out.firstChild); }
    }
    function render(lines) {
      clear();
      const q = (flt.value || '').toLowerCase();
      for (const s of lines) {
        if (!s || typeof s !== 'string') continue;
        if (q && s.toLowerCase().indexOf(q) === -1) continue;
        let cls = '';
        if (s.indexOf('error') !== -1 || s.indexOf('Error') !== -1) cls = 'err';
        else if (s.indexOf('warn') !== -1 || s.indexOf('Warn') !== -1) cls = 'warn';
        else if (s.indexOf('OK') !== -1) cls = 'ok';
        line(s, cls);
      }
      window.scrollTo(0, document.body.scrollHeight);
    }
    function fetchLogs() {
      fetch('/api/logs').then(r => r.json()).then(j => {
        const arr = (j && j.data) || [];
        if (Array.isArray(arr)) { last = arr; render(last); }
      }).catch(() => { /* ignore */ });
    }
    flt.addEventListener('input', () => render(last));
    // initial + poll every 3s
    fetchLogs();
    setInterval(fetchLogs, 3000);
  })();
  </script>
</body></html>`

    // Small optimization: if the buffer is empty, add a hint so the page isn't blank.
    lines := a.bufLog.Lines()
    hasWarm := false
    for _, s := range lines { if strings.Contains(s, "WarmCloud:") { hasWarm = true; break } }
    if !hasWarm {
        a.log.Println("WarmCloud: откройте SMTP → включите прогрев и задайте параметры, затем вернитесь на эту страницу")
    }
    return c.HTML(http.StatusOK, html)
}

