package main

import (
    "net/http"
    "github.com/labstack/echo/v4"
)

// ValidationLogsPage serves a minimal page that streams validation events without refreshing the subscribers page.
func (a *App) ValidationLogsPage(c echo.Context) error {
    html := `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Логи валидации</title>
  <style>
    body { font-family: system-ui, -apple-system, Segoe UI, Roboto, Arial, sans-serif; margin: 0; }
    header { padding: 12px 16px; background:#111; color:#fff; }
    main { padding: 12px 16px; }
    .row { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; white-space: pre-wrap; margin: 4px 0; }
    .muted { color: #666; }
    .ok { color: #2e7d32; }
    .warn { color: #ef6c00; }
    .err { color: #c62828; }
  </style>
  </head>
<body>
  <header>
    <strong>Логи валидации</strong>
  </header>
  <main>
    <div id="out" class="rows"></div>
  </main>
  <script>
  (function() {
    const out = document.getElementById('out');
    function line(txt, cls) {
      const d = document.createElement('div');
      d.className = 'row ' + (cls||'');
      d.textContent = txt;
      out.appendChild(d);
      // Keep last 1000 lines
      const max = 1000; if (out.children.length > max) { out.removeChild(out.firstChild); }
      window.scrollTo(0, document.body.scrollHeight);
    }
    function fmt(obj) { try { return JSON.stringify(obj); } catch(e) { return String(obj); } }
    line('Ожидание событий…', 'muted');
    const es = new EventSource('/api/events');
    es.onmessage = function(ev) {
      try {
        const e = JSON.parse(ev.data);
        if (e && e.type === 'validation') {
          if (e.message === 'started') {
            line('Старт проверки: всего=' + (e.data.total||'?') + ', потоки=' + (e.data.concurrency||'?'), 'ok');
          } else if (e.message === 'progress') {
            line('Прогресс: обработано=' + (e.data.processed||0) + '/' + (e.data.total||0) + ', невалидных=' + (e.data.invalid||0), 'muted');
          } else if (e.message === 'finished') {
            line('Готово: обработано=' + (e.data.processed||0) + ', в ЧС=' + (e.data.blocklisted||0), 'ok');
          } else if (e.message === 'item_start') {
            line('> Начал: ' + (e.data.email||'') + ' (id=' + (e.data.id||'') + ')', 'muted');
          } else if (e.message === 'mx_lookup_start') {
            line('.. MX lookup: ' + (e.data.domain||''), 'muted');
          } else if (e.message === 'mx_lookup_done') {
            const mx = (e.data.mx_hosts||[]).join(', ');
            line('.. MX найдено: ' + (e.data.mx_ok ? 'да' : 'нет') + (mx? ' ['+mx+']':''), e.data.mx_ok ? 'ok':'warn');
          } else if (e.message === 'smtp_probe_start') {
            line('.. SMTP попытка: ' + (e.data.host||''), 'muted');
          } else if (e.message === 'smtp_probe_rcpt') {
            line('.. SMTP ответ: ' + (e.data.host||'') + ' | ' + (e.data.response||''), 'muted');
          } else if (e.message === 'item_done') {
            const r = e.data || {}; const cls = (r.status==='valid') ? 'ok' : (r.status==='invalid'?'err':'warn');
            line('< Завершил: ' + (r.email||'') + ' — ' + (r.status||'?') + (r.reason? ' ('+r.reason+')':'' ) + ' — ' + (r.elapsed_ms||'?') + ' ms', cls);
          } else {
            line('Событие: ' + e.message + ' ' + fmt(e.data), 'muted');
          }
        }
      } catch (e) { /* ignore */ }
    };
    es.onerror = function() { line('Поток событий оборвался, переподключение…', 'warn'); };
  })();
  </script>
</body></html>`
    return c.HTML(http.StatusOK, html)
}
