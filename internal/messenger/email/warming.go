package email

import (
    "context"
    "math/rand"
    "fmt"
    "net/mail"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/knadh/listmonk/models"
    "github.com/knadh/smtppool/v2"
)

// WarmingManager manages SMTP server warming
type WarmingManager struct {
    emailer *Emailer
    ctx     context.Context
    cancel  context.CancelFunc
    wg      sync.WaitGroup
    mu      sync.RWMutex
    servers map[string]*WarmingServer
    fetchCampaign func(int) (*models.Campaign, error)
    // Working hours ranges [start,end) in hours 0-24.
    hours [][2]int
    // paused servers won't start warming until resumed
    paused map[string]bool
    // enable DB-backed scheduling of single-email slots
    useSchedule bool
}

// WarmingServer represents a server being warmed
type WarmingServer struct {
    Server        *Server
    CurrentLimit  int
    DailyStep     int
    CampaignID    int
    LastResetDate string
    Running       bool
    StopChan      chan struct{}
    CompletedToday bool
    // Per-server working hours
    HoursEnabled bool
    hours        [][2]int
    // Last forced tick time (for UI/status)
    LastForcedAt time.Time
    // Current status for UI
    Status       string
    StatusDetail string
    StatusAt     time.Time
    NextAt       time.Time
    // Tracks last activity timestamp for stale-run detection
    LastActivityAt time.Time
}

type scheduleSegment struct {
    start time.Time
    end   time.Time
}

func (wm *WarmingManager) setStatus(ws *WarmingServer, status, detail string) {
    if ws == nil { return }
    wm.mu.Lock()
    ws.Status = status
    ws.StatusDetail = detail
    ws.StatusAt = time.Now()
    ws.LastActivityAt = ws.StatusAt
    if status != "sleep" {
        ws.NextAt = time.Time{}
    }
    wm.mu.Unlock()
}

// NewWarmingManager creates a new warming manager
func NewWarmingManager(emailer *Emailer) *WarmingManager {
    ctx, cancel := context.WithCancel(context.Background())
    return &WarmingManager{
        emailer: emailer,
        ctx:     ctx,
        cancel:  cancel,
        servers: make(map[string]*WarmingServer),
        paused:  make(map[string]bool),
        useSchedule: true,
        }
}

// Start starts the warming manager
func (wm *WarmingManager) Start() {
    wm.wg.Add(1)
    go wm.run()
    // Start schedule loops
    // Initialize server entries
    wm.initServers()
    wm.wg.Add(1)
    go wm.scheduleCoordinator()
    wm.wg.Add(1)
    go wm.scheduleWorker()
}

func (wm *WarmingManager) initServers() {
    wm.mu.Lock()
    defer wm.mu.Unlock()
    currentDate := time.Now().Format("2006-01-02")
    for _, server := range wm.emailer.Servers() {
        if !server.WarmCloudEnabled { continue }
        if _, ok := wm.servers[server.Name]; ok { continue }
        cur := server.WarmCloudCurrentLimit
        if cur <= 0 { cur = server.WarmCloudDailyLimit }
        ws := &WarmingServer{
            Server: server,
            CurrentLimit: cur,
            DailyStep: server.WarmCloudDailyStep,
            CampaignID: server.WarmCloudCampaignID,
            LastResetDate: currentDate,
            StopChan: make(chan struct{}),
            HoursEnabled: server.WarmCloudWorkingHoursEnabled,
        }
        if ws.HoursEnabled { ws.hours = parseWorkingHours(server.WarmCloudWorkingHours) }
        if server.WarmCloudSentToday >= ws.CurrentLimit { ws.CompletedToday = true }
        wm.servers[server.Name] = ws
    }
}

// Stop stops the warming manager
func (wm *WarmingManager) Stop() {
	wm.cancel()
	wm.wg.Wait()
}

// SetCampaignFetcher sets a callback to fetch a campaign by ID from the DB.
func (wm *WarmingManager) SetCampaignFetcher(fn func(int) (*models.Campaign, error)) {
    wm.mu.Lock()
    defer wm.mu.Unlock()
    wm.fetchCampaign = fn
}

// run is the main loop of the warming manager
func (wm *WarmingManager) run() {
	defer wm.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-wm.ctx.Done():
			return
		case <-ticker.C:
			wm.checkAndStartWarming()
		}
	}
}

// checkAndStartWarming checks if it's time to start warming and starts it for enabled servers
func (wm *WarmingManager) checkAndStartWarming() {
    if wm.useSchedule {
        // scheduling mode: worker loop pulls due slots; skip minute-based starter
        return
    }
    now := time.Now()
    currentDate := now.Format("2006-01-02")

    wm.mu.Lock()
    defer wm.mu.Unlock()

    for _, server := range wm.emailer.Servers() {
        if !server.WarmCloudEnabled {
            continue
        }

        serverKey := server.Name
        warmingServer, exists := wm.servers[serverKey]

        if !exists {
            // Создать контекст прогрева для сервера
            // Если известен актуальный лимит на сегодня (восстановлен при старте), используем его; иначе базовый лимит.
            cur := server.WarmCloudCurrentLimit
            if cur <= 0 { cur = server.WarmCloudDailyLimit }
            warmingServer = &WarmingServer{
                Server:        server,
                CurrentLimit:  cur,
                DailyStep:     server.WarmCloudDailyStep,
                CampaignID:    server.WarmCloudCampaignID,
                LastResetDate: currentDate,
                StopChan:      make(chan struct{}),
                HoursEnabled:  server.WarmCloudWorkingHoursEnabled,
            }
            if warmingServer.HoursEnabled {
                warmingServer.hours = parseWorkingHours(server.WarmCloudWorkingHours)
                if len(warmingServer.hours) == 0 {
                    elogf("WarmCloud: у сервера %s включены рабочие часы, но не заданы — используем глобальные", server.Name)
                }
            }
            // Если уже достигнут лимит на момент создания, отметить как завершенный.
            if server.WarmCloudSentToday >= warmingServer.CurrentLimit {
                warmingServer.CompletedToday = true
            }
            wm.servers[serverKey] = warmingServer
        }

        // Синхронизировать пер‑серверные рабочие часы с текущими настройками сервера.
        warmingServer.HoursEnabled = server.WarmCloudWorkingHoursEnabled
        if warmingServer.HoursEnabled {
            warmingServer.hours = parseWorkingHours(server.WarmCloudWorkingHours)
        } else {
            warmingServer.hours = nil
        }

        // Смена дня: увеличить текущий дневной лимит и сбросить счетчики, независимо от рабочих часов.
        if warmingServer.LastResetDate != currentDate {
            old := warmingServer.CurrentLimit
            warmingServer.CurrentLimit += warmingServer.DailyStep
            warmingServer.LastResetDate = currentDate
            server.WarmCloudSentToday = 0
            server.WarmCloudSubmittedToday = 0
            warmingServer.CompletedToday = false
            // Сохранить новый лимит на сегодня для устойчивости.
            if wm.emailer != nil && wm.emailer.warmLimitHook != nil && server.Name != "" {
                wm.emailer.warmLimitHook(server.Name, warmingServer.CurrentLimit)
            }
            elogf("WarmCloud: новый день — увеличиваю лимит для %s: %d -> %d (шаг %d)", server.Name, old, warmingServer.CurrentLimit, warmingServer.DailyStep)
        }
        // Обновить текущее значение лимита на сервере для UI.
        server.WarmCloudCurrentLimit = warmingServer.CurrentLimit

        // Учитывать рабочие часы на уровне сервера (если выключены — без ограничений).
        if !wm.withinWorkingHoursFor(warmingServer, now) {
            wm.setStatus(warmingServer, "wait_hours", "ожидание рабочего окна")
            continue
        }

        // Пауза сервера
        if wm.paused[serverKey] {
            wm.setStatus(warmingServer, "paused", "поставлен на паузу")
            continue
        }

        // Лимит на сегодня достигнут — не запускаем прогрев
        if server.WarmCloudSentToday >= warmingServer.CurrentLimit {
            if !warmingServer.CompletedToday {
                elogf("WarmCloud: сервер %s достиг дневного лимита (%d), ожидание следующего дня", server.Name, warmingServer.CurrentLimit)
                warmingServer.CompletedToday = true
            }
            wm.setStatus(warmingServer, "limit_reached", "дневной лимит достигнут")
            continue
        }

        // Запустить прогрев, если еще не запущен
        if !warmingServer.Running {
            elogf("WarmCloud: запуск прогрева для %s (лимит=%d, шаг=%d, кампания=%d)",
                server.Name, warmingServer.CurrentLimit, warmingServer.DailyStep, warmingServer.CampaignID)
            warmingServer.Running = true
            wm.setStatus(warmingServer, "starting", "запуск процесса прогрева")
            wm.wg.Add(1)
            go wm.warmServer(warmingServer)
        } else {
            // Debug: already running; skip this minute
            elogf("WarmCloud: сервер %s уже выполняется, пропускаю запуск", server.Name)
        }
    }
}

// SetWorkingHours sets working hours ranges from a string like "9-12" or "9-12,14-18".
// Allowed bounds 0-24 (24 only as end). Empty/invalid keeps default.
func (wm *WarmingManager) SetWorkingHours(spec string) {
    parsed := parseWorkingHours(spec)
    wm.mu.Lock()
    wm.hours = parsed
    wm.mu.Unlock()
}

func (wm *WarmingManager) withinWorkingHours(t time.Time) bool {
    wm.mu.RLock()
    hrs := wm.hours
    wm.mu.RUnlock()
    if len(hrs) == 0 {
        return true
    }
    h := t.Hour()
    for _, r := range hrs {
        if r[0] == 0 && r[1] == 24 {
            return true
        }
        if h >= r[0] && h < r[1] {
            return true
        }
    }
    return false
}

// withinWorkingHoursFor evaluates per-server hours if enabled; otherwise no restriction.
func (wm *WarmingManager) withinWorkingHoursFor(ws *WarmingServer, t time.Time) bool {
    if ws == nil {
        return wm.withinWorkingHours(t)
    }
    if !ws.HoursEnabled {
        return true
    }
    hrs := ws.hours
    if len(hrs) == 0 {
        return wm.withinWorkingHours(t)
    }
    h := t.Hour()
    for _, r := range hrs {
        if r[0] == 0 && r[1] == 24 {
            return true
        }
        if h >= r[0] && h < r[1] {
            return true
        }
    }
    return false
}

func parseWorkingHours(spec string) [][2]int {
    var out [][2]int
    s := strings.TrimSpace(spec)
    if s == "" {
        return out
    }
    for _, seg := range strings.Split(s, ",") {
        seg = strings.TrimSpace(seg)
        if seg == "" {
            continue
        }
        parts := strings.Split(seg, "-")
        if len(parts) != 2 {
            continue
        }
        a, errA := strconv.Atoi(strings.TrimSpace(parts[0]))
        b, errB := strconv.Atoi(strings.TrimSpace(parts[1]))
        if errA != nil || errB != nil {
            continue
        }
        if a < 0 || a > 23 || b < 0 || b > 24 || b <= a {
            continue
        }
        out = append(out, [2]int{a, b})
    }
    return out
}

func (wm *WarmingManager) workingSegments(ws *WarmingServer, dayStart time.Time) []scheduleSegment {
    var ranges [][2]int
    if ws != nil && ws.HoursEnabled && len(ws.hours) > 0 {
        ranges = ws.hours
    } else {
        wm.mu.RLock()
        if len(wm.hours) > 0 {
            ranges = wm.hours
        }
        wm.mu.RUnlock()
        if len(ranges) == 0 {
            ranges = [][2]int{{0, 24}}
        }
    }

    dayEnd := dayStart.Add(24 * time.Hour)
    segments := make([]scheduleSegment, 0, len(ranges))
    for _, r := range ranges {
        if r[0] < 0 || r[1] > 24 || r[1] <= r[0] {
            continue
        }
        start := dayStart.Add(time.Duration(r[0]) * time.Hour)
        end := dayStart.Add(time.Duration(r[1]) * time.Hour)
        if end.Before(start) {
            continue
        }
        if end.After(dayEnd) {
            end = dayEnd
        }
        segments = append(segments, scheduleSegment{start: start, end: end})
    }
    if len(segments) == 0 {
        segments = append(segments, scheduleSegment{start: dayStart, end: dayEnd})
    }
    return segments
}

func (wm *WarmingManager) refreshServerConfig(ws *WarmingServer) {
    if ws == nil || ws.Server == nil {
        return
    }
    wm.mu.Lock()
    defer wm.mu.Unlock()
    ws.DailyStep = ws.Server.WarmCloudDailyStep
    ws.CampaignID = ws.Server.WarmCloudCampaignID
    if ws.Server.WarmCloudCurrentLimit > 0 {
        ws.CurrentLimit = ws.Server.WarmCloudCurrentLimit
    } else if ws.Server.WarmCloudDailyLimit > 0 && ws.CurrentLimit <= 0 {
        ws.CurrentLimit = ws.Server.WarmCloudDailyLimit
    }
    ws.HoursEnabled = ws.Server.WarmCloudWorkingHoursEnabled
    if ws.HoursEnabled {
        ws.hours = parseWorkingHours(ws.Server.WarmCloudWorkingHours)
    } else {
        ws.hours = nil
    }
}

func clipSegments(segments []scheduleSegment, windowStart, windowEnd time.Time) []scheduleSegment {
    if windowEnd.Before(windowStart) {
        return nil
    }
    clipped := make([]scheduleSegment, 0, len(segments))
    for _, seg := range segments {
        start := seg.start
        end := seg.end
        if end.Before(windowStart) || start.After(windowEnd) {
            continue
        }
        if start.Before(windowStart) {
            start = windowStart
        }
        if end.After(windowEnd) {
            end = windowEnd
        }
        if !end.After(start) {
            continue
        }
        clipped = append(clipped, scheduleSegment{start: start, end: end})
    }
    return clipped
}

func segmentsTotalSeconds(segments []scheduleSegment) float64 {
    total := 0.0
    for _, seg := range segments {
        total += seg.end.Sub(seg.start).Seconds()
    }
    return total
}

func segmentOffsetToTime(segments []scheduleSegment, seconds float64) time.Time {
    remaining := seconds
    for _, seg := range segments {
        dur := seg.end.Sub(seg.start).Seconds()
        if remaining < dur {
            return seg.start.Add(time.Duration(remaining * float64(time.Second)))
        }
        remaining -= dur
    }
    if len(segments) == 0 {
        return time.Time{}
    }
    last := segments[len(segments)-1]
    return last.end.Add(-time.Millisecond)
}

func (wm *WarmingManager) getOrCreateServer(serverName string) *WarmingServer {
    wm.mu.Lock()
    if ws, ok := wm.servers[serverName]; ok {
        wm.mu.Unlock()
        return ws
    }
    if wm.emailer == nil {
        wm.mu.Unlock()
        return nil
    }
    for _, srv := range wm.emailer.Servers() {
        if srv.Name != serverName {
            continue
        }
        cur := srv.WarmCloudCurrentLimit
        if cur <= 0 {
            cur = srv.WarmCloudDailyLimit
        }
        ws := &WarmingServer{
            Server:        srv,
            CurrentLimit:  cur,
            DailyStep:     srv.WarmCloudDailyStep,
            CampaignID:    srv.WarmCloudCampaignID,
            LastResetDate: time.Now().Format("2006-01-02"),
            StopChan:      make(chan struct{}),
            HoursEnabled:  srv.WarmCloudWorkingHoursEnabled,
        }
        if ws.HoursEnabled {
            ws.hours = parseWorkingHours(srv.WarmCloudWorkingHours)
        }
        if srv.WarmCloudSentToday >= ws.CurrentLimit && ws.CurrentLimit > 0 {
            ws.CompletedToday = true
        }
        wm.servers[serverName] = ws
        wm.mu.Unlock()
        return ws
    }
    wm.mu.Unlock()
    return nil
}

// warmServer performs the warming process for a single server
func (wm *WarmingManager) warmServer(ws *WarmingServer) {
    defer wm.wg.Done()
    defer func() { ws.Running = false }()

	client := NewWarmCloudClient(
		ws.Server.WarmCloudToken,
		ws.Server.WarmCloudUserID,
		ws.Server.WarmCloudRefID,
	)

	// Update current limit in server
	ws.Server.WarmCloudCurrentLimit = ws.CurrentLimit

	// Check balance first
    wm.setStatus(ws, "checking_balance", "проверка баланса")
    balance, err := client.GetBalance()
    if err != nil {
        elogf("WarmCloud: ошибка получения баланса для %s: %v", ws.Server.Name, err)
        wm.setStatus(ws, "error", "ошибка баланса: "+err.Error())
        return
    }

    elogf("WarmCloud: баланс для %s = %.2f", ws.Server.Name, balance)
    if balance <= 0 {
        elogf("WarmCloud: недостаточный баланс для %s: %.2f", ws.Server.Name, balance)
        wm.setStatus(ws, "no_balance", "недостаточный баланс")
        return
    }

    // Batch fetch/send/submit flow. No artificial sleeps; rate is controlled by batch size per minute.

    // Respect daily current limit.
    remaining := ws.CurrentLimit - ws.Server.WarmCloudSentToday
    if remaining <= 0 {
        ws.CompletedToday = true
        elogf("WarmCloud: server %s reached daily limit (%d), stopping for today", ws.Server.Name, ws.CurrentLimit)
        wm.setStatus(ws, "limit_reached", "дневной лимит достигнут")
        return
    }

    // Rate limiting: emails per window (minutes) -> approx per-minute budget
    perMinute := 0
    if ws.Server.WarmCloudRateWindowMinutes > 0 && ws.Server.WarmCloudRateCount > 0 {
        // Ceil division
        m := ws.Server.WarmCloudRateWindowMinutes
        c := ws.Server.WarmCloudRateCount
        perMinute = (c + m - 1) / m
        if perMinute < 1 { perMinute = 1 }
    }
    // Honor per-tick cap if set; default to 30 otherwise.
    maxPerTick := ws.Server.WarmCloudMaxPerTick
    if maxPerTick <= 0 { maxPerTick = 30 }
    toDo := remaining
    if perMinute > 0 && toDo > perMinute { toDo = perMinute }
    if toDo > maxPerTick { toDo = maxPerTick }

    // Validate 'big' param for API /get
    big := strings.ToLower(strings.TrimSpace(ws.Server.WarmCloudBig))
    if big != "m" && big != "g" {
        elogf("WarmCloud: у сервера %s некорректный warmcloud_big ('%s'), ожидается 'm' или 'g'", ws.Server.Name, big)
        wm.setStatus(ws, "invalid_big", "ожидается 'm' или 'g'")
        return
    }

    // Get campaign for warming
    campaign, err := wm.getCampaign(ws.CampaignID)
    if err != nil {
        elogf("Error getting campaign %d for server %s: %v", ws.CampaignID, ws.Server.Name, err)
        return
    }

    processed := 0
    // Fetch a batch first
    elogf("WarmCloud: запрашиваю %d e‑mail(ов) для %s user_id=%d ref_id=%s big=%s",
        toDo, ws.Server.Name, ws.Server.WarmCloudUserID, ws.Server.WarmCloudRefID, big)
    wm.setStatus(ws, "requesting", fmt.Sprintf("запрос %d адресов", toDo))
    batch, err := client.GetEmails(toDo, big)
    if err != nil {
        if strings.HasPrefix(err.Error(), "rotation_exhausted:") {
            elogf("WarmCloud: ротация исчерпана для %s — очищаю и пробую ещё раз", ws.Server.Name)
            wm.setStatus(ws, "clearing_rotation", "очистка ротации")
            if e2 := client.ClearRotation(); e2 != nil {
                elogf("WarmCloud: ошибка clear_rotation для %s: %v", ws.Server.Name, e2)
                wm.setStatus(ws, "error", "clear_rotation: "+e2.Error())
                return
            }
            wm.setStatus(ws, "requesting", fmt.Sprintf("повторный запрос %d адресов", toDo))
            batch, err = client.GetEmails(toDo, big)
            if err != nil {
                elogf("WarmCloud: повторный get после clear_rotation неудачен для %s: %v", ws.Server.Name, err)
                wm.setStatus(ws, "error", "get после clear_rotation: "+err.Error())
                return
            }
        } else {
            elogf("WarmCloud: ошибка получения адресов для %s: %v", ws.Server.Name, err)
            wm.setStatus(ws, "error", "ошибка get: "+err.Error())
            return
        }
    }
    if len(batch) == 0 {
        elogf("WarmCloud: нет доступных адресов для %s", ws.Server.Name)
        wm.setStatus(ws, "no_emails", "нет адресов от сервиса")
        return
    }
    if len(batch) > toDo { batch = batch[:toDo] }

    for processed < len(batch) {
        select {
        case <-ws.StopChan:
            processed++ // mark as progressed to avoid tight loop; will exit soon
            wm.setStatus(ws, "stopping", "прерывание тика")
            continue
        default:
        }

        emailAddr := batch[processed].Email
        token := batch[processed].SubmitToken
        elogf("WarmCloud: получен адрес email=%s token_len=%d", emailAddr, len(token))

        wm.setStatus(ws, "sending", "отправка на "+emailAddr)
        if err := wm.sendWarmingEmail(ws.Server, emailAddr, campaign); err != nil {
            elogf("WarmCloud: ошибка отправки письма прогрева на %s через %s: %v", emailAddr, ws.Server.Name, err)
            wm.setStatus(ws, "error", "ошибка отправки: "+err.Error())
            // Do not submit; let reservation expire automatically.
        } else {
            ws.Server.WarmCloudSentToday++
            // Record event after successful send (pre-submit) with token for later update.
            ev := WarmingEvent{
                CreatedAt: time.Now(),
                Server: ws.Server.Name,
                Email: emailAddr,
                Subject: campaign.Subject,
                Submitted: false,
                SubmitToken: token,
            }
            AddWarmingEvent(ev)
            if wm.emailer != nil && wm.emailer.warmEventHook != nil {
                wm.emailer.warmEventHook(ev)
            }

            // Decide on submit based on open-rate and remaining daily target.
            rpct := ws.Server.WarmCloudReadPercent
            if rpct < 0 { rpct = 0 }
            if rpct > 100 { rpct = 100 }

            targetSubmit := 0
            if rpct > 0 {
                targetSubmit = int((ws.CurrentLimit * rpct + 99) / 100)
            }
            shouldSubmit := false
            if ws.Server.WarmCloudSubmittedToday < targetSubmit && rpct > 0 {
                if rand.Float64() < (float64(rpct) / 100.0) {
                    shouldSubmit = true
                }
            }
            if shouldSubmit {
                subj := strings.TrimSpace(campaign.Subject)
                if subj == "" {
                    subj = "(no subject)"
                    elogf("WarmCloud: пустой subject кампании для %s — использую заглушку", ws.Server.Name)
                }
                // Лог размеров submit (без секретов)
                elogf("WarmCloud: submit попытка server=%s email=%s subject_len=%d token_len=%d", ws.Server.Name, emailAddr, len(subj), len(token))
                wm.setStatus(ws, "submitting", "подтверждение читаемости")
                payload := []WarmCloudEmail{{Email: emailAddr, Subject: subj, SubmitToken: token}}
                if err := client.SubmitEmailsWithBig(payload, big); err != nil {
                    elogf("WarmCloud: ошибка submit для %s: %v", ws.Server.Name, err)
                    wm.setStatus(ws, "error", "ошибка submit: "+err.Error())
                    UpdateWarmingEventByToken(token, false, err.Error())
                    if wm.emailer != nil && wm.emailer.warmEventHook != nil {
                        wm.emailer.warmEventHook(WarmingEvent{CreatedAt: time.Now(), SubmitToken: token, Submitted: false, Error: err.Error(), Server: ws.Server.Name, Email: emailAddr, Subject: subj})
                    }
                } else {
                    ws.Server.WarmCloudSubmittedToday++
                    if wm.emailer != nil && wm.emailer.onSent != nil && ws.Server.Name != "" {
                        wm.emailer.onSent("warmcloud_read:" + ws.Server.Name)
                    }
                    UpdateWarmingEventByToken(token, true, "")
                    if wm.emailer != nil && wm.emailer.warmEventHook != nil {
                        wm.emailer.warmEventHook(WarmingEvent{CreatedAt: time.Now(), SubmitToken: token, Submitted: true, Server: ws.Server.Name, Email: emailAddr, Subject: subj})
                    }
                    elogf("WarmCloud: submit OK server=%s email=%s token_len=%d", ws.Server.Name, emailAddr, len(token))
                    wm.setStatus(ws, "submitted", "подтверждено")
                }
            }
        }

        processed++
    }
    // Final status after finishing the run
    wm.setStatus(ws, "idle", "завершено")
}

// scheduleCoordinator ensures there are enough pending slots for near future.
func (wm *WarmingManager) scheduleCoordinator() {
    defer wm.wg.Done()
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    for {
        select {
        case <-wm.ctx.Done():
            return
        case now := <-ticker.C:
            // Pick up newly-enabled servers dynamically.
            wm.initServers()
            wm.mu.RLock()
            servers := make([]*WarmingServer, 0, len(wm.servers))
            for _, ws := range wm.servers { servers = append(servers, ws) }
            wm.mu.RUnlock()
            for _, ws := range servers {
                if ws == nil || ws.Server == nil || !ws.Server.WarmCloudEnabled { continue }
                if wm.paused[ws.Server.Name] { continue }
                // Perform daily rollover: advance limit, reset counters, rebuild today's schedule once per new day
                wm.ensureDailyRollover(ws, now)
                // Always ensure the rest of today is planned at least once (even outside working hours)
                dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
                if err := wm.createDailySchedule(ws, dayStart); err != nil {
                    elogf("WarmCloud: ошибка создания расписания для %s: %v", ws.Server.Name, err)
                }
                // If currently within working hours, also plan the immediate next window
                if wm.withinWorkingHoursFor(ws, now) {
                    wm.planNextWindow(ws, now)
                }
            }
        }
    }
}

// ensureDailyRollover resets per-day counters and schedule when the day changes.
func (wm *WarmingManager) ensureDailyRollover(ws *WarmingServer, now time.Time) {
    if ws == nil || ws.Server == nil { return }
    today := now.Format("2006-01-02")
    wm.mu.Lock()
    changed := false
    if ws.LastResetDate != today {
        ws.CurrentLimit += ws.DailyStep
        ws.LastResetDate = today
        ws.Server.WarmCloudSentToday = 0
        ws.Server.WarmCloudSubmittedToday = 0
        ws.CompletedToday = false
        ws.Server.WarmCloudCurrentLimit = ws.CurrentLimit
        if wm.emailer != nil && wm.emailer.warmLimitHook != nil && ws.Server.Name != "" {
            wm.emailer.warmLimitHook(ws.Server.Name, ws.CurrentLimit)
        }
        changed = true
    }
    wm.mu.Unlock()
    if !changed { return }
    // Clear today's pending and rebuild spread across the day with new logic
    if wm.emailer != nil {
        if wm.emailer.schedule.deleteDayPending != nil {
            _ = wm.emailer.schedule.deleteDayPending(ws.Server.Name, now)
        }
        // Use new automatic schedule creation
        if err := wm.createDailySchedule(ws, now); err != nil {
            elogf("WarmCloud: ошибка создания расписания для %s: %v", ws.Server.Name, err)
        }
    }
}

// planNextWindow creates schedule slots for the next rate window based on server config.
func (wm *WarmingManager) planNextWindow(ws *WarmingServer, now time.Time) {
    if err := wm.createDailySchedule(ws, now); err != nil {
        elogf("WarmCloud: ошибка обновления окна для %s: %v", ws.Server.Name, err)
    }
}

// planFullDay spreads remaining daily budget across the rest of the day within working hours
func (wm *WarmingManager) planFullDay(ws *WarmingServer, day time.Time) {
    if err := wm.createDailySchedule(ws, day); err != nil {
        elogf("WarmCloud: ошибка создания расписания для %s: %v", ws.Server.Name, err)
    }
}

// scheduleWorker polls due slots and processes one email per slot.
func (wm *WarmingManager) scheduleWorker() {
    defer wm.wg.Done()
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-wm.ctx.Done():
            return
        case <-ticker.C:
            wm.mu.RLock()
            servers := make([]*WarmingServer, 0, len(wm.servers))
            for _, ws := range wm.servers { servers = append(servers, ws) }
            wm.mu.RUnlock()
            for _, ws := range servers {
                if ws == nil || ws.Server == nil || !ws.Server.WarmCloudEnabled { continue }
                if wm.paused[ws.Server.Name] { continue }
                if wm.emailer == nil || wm.emailer.schedule.nextDueByServerWithFromEmail == nil { continue }
                id, due, fromEmail, ok := wm.emailer.schedule.nextDueByServerWithFromEmail(ws.Server.Name)
                if !ok { continue }
                if time.Now().Before(due) { continue }
                // Process one email for this slot with sender email from schedule
                ok2, sentEmail, errMsg, wasSubmitted, senderEmail := wm.processOneWithSender(ws, fromEmail)
                if ok2 {
                    // Update sender email in database
                    if wm.emailer.schedule.updateFromEmail != nil && senderEmail != "" {
                        _ = wm.emailer.schedule.updateFromEmail(id, senderEmail)
                    }
                    
                    if wasSubmitted {
                        // If email was submitted to WarmCloud, mark as read and persist the actual recipient email
                        if wm.emailer.schedule.markReadEmail != nil && sentEmail != "" {
                            _ = wm.emailer.schedule.markReadEmail(id, sentEmail)
                        } else if wm.emailer.schedule.markRead != nil {
                            _ = wm.emailer.schedule.markRead(id)
                        }
                    } else {
                        // If email was sent but not submitted, mark as done
                        if wm.emailer.schedule.markDoneEmail != nil && sentEmail != "" {
                            _ = wm.emailer.schedule.markDoneEmail(id, sentEmail)
                        } else if wm.emailer.schedule.markDone != nil {
                            _ = wm.emailer.schedule.markDone(id)
                        }
                    }
                } else {
                    if wm.emailer.schedule.markError != nil {
                        if errMsg == "" { errMsg = "send failed" }
                        _ = wm.emailer.schedule.markError(id, errMsg)
                    }
                }
            }
        }
    }
}

// getSenderEmailFromCampaign returns the sender email address from campaign or server settings
func (wm *WarmingManager) getSenderEmailFromCampaign(campaign *models.Campaign, server *Server) string {
    // First priority: campaign.from_email
    if campaign.FromEmail != "" {
        return campaign.FromEmail
    }
    
    // Second priority: server username
    if server.Username != "" {
        return server.Username
    }
    
    // Third priority: From header in email_headers
    if fromHeader, exists := server.EmailHeaders["From"]; exists && fromHeader != "" {
        return fromHeader
    }
    
    // Fallback: generate no-reply@domain
    domain := strings.TrimPrefix(server.Host, "smtp.")
    return fmt.Sprintf("no-reply@%s", domain)
}

func personaAllowsServer(serverName, candidate string) bool {
    cand := strings.ToLower(strings.TrimSpace(candidate))
    if cand == "" {
        return false
    }
    srv := strings.ToLower(strings.TrimSpace(serverName))
    if srv == cand {
        return true
    }
    srvNoPrefix := strings.TrimPrefix(srv, "email-")
    candNoPrefix := strings.TrimPrefix(cand, "email-")
    if srvNoPrefix == cand || srv == candNoPrefix || srvNoPrefix == candNoPrefix {
        return true
    }
    return false
}

// getSenderEmailsFromPersona returns sender emails from persona accounts if campaign uses personas
func (wm *WarmingManager) getSenderEmailsFromPersona(campaign *models.Campaign, server *Server) []string {
    if !campaign.PersonaDomain.Valid || campaign.PersonaDomain.String == "" {
        return nil
    }
    
    // Get persona accounts for this domain
    // We need access to settings to get personas
    if wm.emailer == nil || wm.emailer.settings == nil {
        elogf("WarmCloud: нет доступа к настройкам для получения личностей (server=%s campaign=%d)", server.Name, campaign.ID)
        return nil
    }
    
    domain := strings.ToLower(strings.TrimSpace(campaign.PersonaDomain.String))
    if domain == "" {
        return nil
    }
    
    // Find persona for this domain
    for _, persona := range wm.emailer.settings.Personas {
        if strings.EqualFold(persona.Domain, domain) {
            // Check if this server is allowed for this persona
            serverAllowed := len(persona.SMTP) == 0
            if !serverAllowed {
                for _, allowedSMTP := range persona.SMTP {
                    if personaAllowsServer(server.Name, allowedSMTP) {
                        serverAllowed = true
                        break
                    }
                }
            }
            
            if !serverAllowed {
                elogf("WarmCloud: сервер %s не разрешен для личности %s (allowed=%v)", server.Name, domain, persona.SMTP)
                continue
            }
            
            // Build email addresses from persona accounts
            var emails []string
            for _, account := range persona.Accounts {
                name := strings.TrimSpace(account.Name)
                if name == "" {
                    continue
                }
                email := name + "@" + persona.Domain
                emails = append(emails, email)
            }
            
            if len(emails) > 0 {
                elogf("WarmCloud: найдено %d адресов личности для домена %s (server=%s)", len(emails), domain, server.Name)
                return emails
            }
            
            elogf("WarmCloud: в личности %s нет аккаунтов", domain)
            break
        }
    }
    
    elogf("WarmCloud: личность для домена %s не найдена", domain)
    return nil
}

// createDailySchedule creates a complete daily schedule for warming with sender emails
func (wm *WarmingManager) createDailySchedule(ws *WarmingServer, day time.Time) error {
    if wm.emailer == nil || ws == nil || ws.Server == nil {
        return fmt.Errorf("invalid parameters")
    }

    wm.refreshServerConfig(ws)

    campaign, err := wm.getCampaign(ws.CampaignID)
    if err != nil {
        return fmt.Errorf("failed to get campaign %d: %v", ws.CampaignID, err)
    }

    personaTag := ""
    senderEmails := []string{wm.getSenderEmailFromCampaign(campaign, ws.Server)}
    if personaEmails := wm.getSenderEmailsFromPersona(campaign, ws.Server); len(personaEmails) > 0 {
        senderEmails = personaEmails
        if campaign.PersonaDomain.Valid {
            personaTag = strings.ToLower(strings.TrimSpace(campaign.PersonaDomain.String))
        }
    } else if campaign.PersonaDomain.Valid {
        personaTag = strings.ToLower(strings.TrimSpace(campaign.PersonaDomain.String))
    }

    cleaned := make([]string, 0, len(senderEmails))
    for _, em := range senderEmails {
        em = strings.TrimSpace(em)
        if em != "" {
            cleaned = append(cleaned, em)
        }
    }
    senderEmails = cleaned
    if len(senderEmails) == 0 {
        return fmt.Errorf("no sender email available for server %s", ws.Server.Name)
    }

    loc := day.Location()
    if loc == nil {
        loc = time.Local
    }
    dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
    dayEnd := dayStart.Add(24 * time.Hour)

    wm.ensurePendingSenders(ws, dayStart, senderEmails, personaTag)

    now := time.Now().In(loc)
    scheduleStart := dayStart
    if !now.Before(dayStart) && now.Before(dayEnd) {
        scheduleStart = now.Add(5 * time.Second)
    }
    if !scheduleStart.Before(dayEnd) {
        return nil
    }

    segments := wm.workingSegments(ws, dayStart)
    segments = clipSegments(segments, scheduleStart, dayEnd)
    if len(segments) == 0 {
        return fmt.Errorf("no working hours configured for server %s", ws.Server.Name)
    }
    totalSeconds := segmentsTotalSeconds(segments)
    if totalSeconds <= 0 {
        return fmt.Errorf("no available time for scheduling on %s", ws.Server.Name)
    }

    needed := ws.CurrentLimit
    if needed <= 0 {
        needed = ws.Server.WarmCloudDailyLimit
    }
    if needed <= 0 {
        return fmt.Errorf("no daily limit defined for %s", ws.Server.Name)
    }

    if wm.emailer.schedule.getSentToday != nil {
        if sentToday, err := wm.emailer.schedule.getSentToday(ws.Server.Name, dayStart); err == nil {
            needed -= int(sentToday)
        }
    }
    if wm.emailer.schedule.countPendingWindow != nil {
        if pending, err := wm.emailer.schedule.countPendingWindow(ws.Server.Name, dayStart, dayEnd); err == nil {
            needed -= int(pending)
        }
    }
    if needed <= 0 {
        return nil
    }

    step := totalSeconds / float64(needed)
    if step <= 0 {
        step = totalSeconds / float64(needed)
    }

    senderCount := len(senderEmails)
    inserted := 0
    for i := 0; i < needed; i++ {
        offset := float64(i)*step + rand.Float64()*step
        if offset > totalSeconds {
            offset = totalSeconds
        }
        dueTime := segmentOffsetToTime(segments, offset)
        if dueTime.IsZero() {
            dueTime = scheduleStart
        }
        if dueTime.Before(scheduleStart) {
            dueTime = scheduleStart
        }
        dueTime = dueTime.Add(time.Duration(i) * time.Millisecond)

        senderEmail := senderEmails[i%senderCount]
        if wm.emailer.schedule.insertWithFromEmailSubject != nil {
            if err := wm.emailer.schedule.insertWithFromEmailSubject(ws.Server.Name, dueTime, senderEmail, campaign.Subject, personaTag); err != nil {
                elogf("WarmCloud: ошибка добавления слота в расписание %s: %v", ws.Server.Name, err)
            } else {
                inserted++
            }
        }
    }

    if inserted > 0 {
        elogf("WarmCloud: создано расписание для %s: %d слотов на %s", ws.Server.Name, inserted, dayStart.Format("2006-01-02"))
    }
    return nil
}

func (wm *WarmingManager) ensurePendingSenders(ws *WarmingServer, dayStart time.Time, senderEmails []string, personaTag string) {
    if wm.emailer == nil || wm.emailer.schedule.listDay == nil || len(senderEmails) == 0 {
        return
    }
    slots, err := wm.emailer.schedule.listDay(ws.Server.Name, dayStart)
    if err != nil || len(slots) == 0 {
        return
    }
    assigned := 0
    total := len(senderEmails)
    for _, slot := range slots {
        if slot.Status != "pending" {
            continue
        }
        if wm.emailer.schedule.updateFromEmail != nil {
            if strings.TrimSpace(slot.FromEmail) == "" {
                desired := senderEmails[assigned%total]
                assigned++
                _ = wm.emailer.schedule.updateFromEmail(slot.ID, desired)
                slot.FromEmail = desired
            }
        }
        if personaTag != "" && wm.emailer.schedule.updatePersona != nil {
            if strings.TrimSpace(slot.Persona) == "" {
                _ = wm.emailer.schedule.updatePersona(slot.ID, personaTag)
            }
        }
    }
}

// processOne sends a single warming email using existing logic (subset of warmServer)
func (wm *WarmingManager) processOne(ws *WarmingServer) (bool, string, string, bool, string) {
    return wm.processOneWithSender(ws, "")
}

// processOneWithSender sends a single warming email with specific sender email
func (wm *WarmingManager) processOneWithSender(ws *WarmingServer, senderEmail string) (bool, string, string, bool, string) {
    client := NewWarmCloudClient(ws.Server.WarmCloudToken, ws.Server.WarmCloudUserID, ws.Server.WarmCloudRefID)
    // Basic checks
    big := strings.ToLower(strings.TrimSpace(ws.Server.WarmCloudBig))
    if big != "m" && big != "g" { return false, "", "invalid provider", false, "" }
    remaining := ws.CurrentLimit - ws.Server.WarmCloudSentToday
    if remaining <= 0 { return false, "", "limit reached", false, "" }
    // Fetch one
    wm.setStatus(ws, "requesting", "запрос адреса")
    items, err := client.GetEmails(1, big)
    if err != nil || len(items) == 0 { return false, "", "no emails", false, "" }
    emailAddr := items[0].Email
    token := items[0].SubmitToken
    // Campaign
    camp, err := wm.getCampaign(ws.CampaignID)
    if err != nil { return false, "", err.Error(), false, "" }
    wm.setStatus(ws, "sending", "отправка на "+emailAddr)
    usedFrom, err := wm.sendWarmingEmailWithSender(ws.Server, emailAddr, camp, senderEmail)
    if err != nil {
        return false, "", err.Error(), false, ""
    }
    ws.Server.WarmCloudSentToday++
    // Record event after successful send (pre-submit) with token
    ev := WarmingEvent{
        CreatedAt: time.Now(),
        Server: ws.Server.Name,
        Email: emailAddr,
        Subject: camp.Subject,
        Submitted: false,
        SubmitToken: token,
    }
    AddWarmingEvent(ev)
    if wm.emailer != nil && wm.emailer.warmEventHook != nil {
        wm.emailer.warmEventHook(ev)
    }
    // Submit per read %
    rpct := ws.Server.WarmCloudReadPercent
    if rpct < 0 { rpct = 0 }
    if rpct > 100 { rpct = 100 }
    targetSubmit := 0
    if rpct > 0 { targetSubmit = int((ws.CurrentLimit * rpct + 99) / 100) }
    wasSubmitted := false
    if ws.Server.WarmCloudSubmittedToday < targetSubmit && rpct > 0 {
        if rand.Float64() < (float64(rpct) / 100.0) {
            subj := strings.TrimSpace(camp.Subject)
            if subj == "" { subj = "(no subject)" }
            if err := client.SubmitEmailsWithBig([]WarmCloudEmail{{Email: emailAddr, Subject: subj, SubmitToken: token}}, big); err != nil {
                // persist error against token
                UpdateWarmingEventByToken(token, false, err.Error())
                if wm.emailer != nil && wm.emailer.warmEventHook != nil {
                    wm.emailer.warmEventHook(WarmingEvent{CreatedAt: time.Now(), SubmitToken: token, Submitted: false, Error: err.Error(), Server: ws.Server.Name, Email: emailAddr, Subject: subj})
                }
            } else {
                ws.Server.WarmCloudSubmittedToday++
                UpdateWarmingEventByToken(token, true, "")
                if wm.emailer != nil && wm.emailer.warmEventHook != nil {
                    wm.emailer.warmEventHook(WarmingEvent{CreatedAt: time.Now(), SubmitToken: token, Submitted: true, Server: ws.Server.Name, Email: emailAddr, Subject: subj})
                }
                wasSubmitted = true
            }
        }
    }
    wm.setStatus(ws, "idle", "завершено")

    return true, emailAddr, "", wasSubmitted, usedFrom
}

// Pause pauses warming for the given server.
func (wm *WarmingManager) Pause(serverName string) {
    wm.mu.Lock()
    defer wm.mu.Unlock()
    wm.paused[serverName] = true
}

// Resume resumes warming for the given server.
func (wm *WarmingManager) Resume(serverName string) {
    wm.mu.Lock()
    defer wm.mu.Unlock()
    delete(wm.paused, serverName)
}

// MarkFinishedToday marks the current day as completed for the server without sending remaining e-mails.
func (wm *WarmingManager) MarkFinishedToday(serverName string) {
    var target *WarmingServer
    wm.mu.Lock()
    if ws, ok := wm.servers[serverName]; ok && ws != nil && ws.Server != nil {
        if ws.CurrentLimit > ws.Server.WarmCloudSentToday {
            ws.Server.WarmCloudSentToday = ws.CurrentLimit
        }
        ws.CompletedToday = true
        target = ws
    }
    wm.mu.Unlock()
    if target != nil {
        wm.setStatus(target, "idle", "завершено руками")
    }
}

// ForceTick triggers a single warming run immediately if not already running.
func (wm *WarmingManager) ForceTick(serverName string) bool {
    wm.mu.Lock()
    ws, ok := wm.servers[serverName]
    if !ok {
        // initialize entry if the server exists in emailer config
        for _, s := range wm.emailer.Servers() {
            if s.Name == serverName {
                cur := s.WarmCloudCurrentLimit
                if cur <= 0 { cur = s.WarmCloudDailyLimit }
                ws = &WarmingServer{
                    Server:        s,
                    CurrentLimit:  cur,
                    DailyStep:     s.WarmCloudDailyStep,
                    CampaignID:    s.WarmCloudCampaignID,
                    LastResetDate: time.Now().Format("2006-01-02"),
                    StopChan:      make(chan struct{}),
                    HoursEnabled:  s.WarmCloudWorkingHoursEnabled,
                }
                if ws.HoursEnabled { ws.hours = parseWorkingHours(s.WarmCloudWorkingHours) }
                wm.servers[serverName] = ws
                break
            }
        }
    }
    if ws == nil || wm.paused[serverName] {
        elogf("WarmCloud: ручной запуск тика для %s — отклонено (paused/нет)", serverName)
        wm.mu.Unlock()
        return false
    }
    if ws.Running {
        // Try to preempt a potentially stale run: close StopChan and reset running flag.
        // Ignore panic if channel already closed.
        func() { defer func(){ _ = recover() }(); close(ws.StopChan) }()
        ws.Running = false
        ws.StopChan = make(chan struct{})
        elogf("WarmCloud: прерываю текущий тик %s для ручного перезапуска", serverName)
    }
    // Check limit
    if ws.Server.WarmCloudSentToday >= ws.CurrentLimit {
        elogf("WarmCloud: ручной запуск тика для %s — лимит достигнут (%d)", serverName, ws.CurrentLimit)
        wm.mu.Unlock()
        return false
    }
    elogf("WarmCloud: ручной запуск тика для %s", serverName)
    ws.Running = true
    wm.wg.Add(1)
    ws.LastForcedAt = time.Now()
    ws.Status = "starting"
    ws.StatusDetail = "ручной запуск"
    ws.StatusAt = time.Now()
    ws.LastActivityAt = ws.StatusAt
    wm.mu.Unlock()
    go wm.warmServer(ws)
    return true
}

// ResetDaily resets daily counters and advances current limit for a server.
func (wm *WarmingManager) ResetDaily(serverName string) {
    wm.mu.Lock()
    defer wm.mu.Unlock()
    if ws, ok := wm.servers[serverName]; ok {
        today := time.Now().Format("2006-01-02")
        if ws.LastResetDate != today {
            ws.CurrentLimit += ws.DailyStep
            ws.LastResetDate = today
        }
        ws.Server.WarmCloudSentToday = 0
        ws.Server.WarmCloudSubmittedToday = 0
        ws.CompletedToday = false
        ws.Server.WarmCloudCurrentLimit = ws.CurrentLimit
        if wm.emailer != nil && wm.emailer.warmLimitHook != nil && ws.Server.Name != "" {
            wm.emailer.warmLimitHook(ws.Server.Name, ws.CurrentLimit)
        }
        // Rebuild today's schedule: clear pending today and spread remaining across the day
        if wm.emailer != nil {
            // delete today's pending
            if wm.emailer.schedule.deleteDayPending != nil {
                day := time.Now()
                _ = wm.emailer.schedule.deleteDayPending(ws.Server.Name, day)
            }
            // plan for the rest of the day from now
            go wm.planFullDay(ws, time.Now())
        }
    }
}

// ResetAllDaily resets daily counters for all servers.
func (wm *WarmingManager) ResetAllDaily() {
    wm.mu.Lock()
    defer wm.mu.Unlock()
    today := time.Now().Format("2006-01-02")
    for _, ws := range wm.servers {
        if ws.LastResetDate != today {
            ws.CurrentLimit += ws.DailyStep
            ws.LastResetDate = today
        }
        ws.Server.WarmCloudSentToday = 0
        ws.Server.WarmCloudSubmittedToday = 0
        ws.CompletedToday = false
        ws.Server.WarmCloudCurrentLimit = ws.CurrentLimit
        if wm.emailer != nil && wm.emailer.warmLimitHook != nil && ws.Server.Name != "" {
            wm.emailer.warmLimitHook(ws.Server.Name, ws.CurrentLimit)
        }
        // Clear today's pending schedule and replan in a goroutine
        if wm.emailer != nil {
            if wm.emailer.schedule.deleteDayPending != nil {
                day := time.Now()
                _ = wm.emailer.schedule.deleteDayPending(ws.Server.Name, day)
            }
            go wm.planFullDay(ws, time.Now())
        }
    }
}

// RebuildSchedule regenerates the schedule for a server, optionally clearing pending slots for the given day.
func (wm *WarmingManager) RebuildSchedule(serverName string, day time.Time, clear bool) error {
    if wm == nil {
        return fmt.Errorf("warming not initialized")
    }
    if day.IsZero() {
        day = time.Now()
    }
    ws := wm.getOrCreateServer(serverName)
    if ws == nil || ws.Server == nil {
        return fmt.Errorf("server %s not found", serverName)
    }
    if !ws.Server.WarmCloudEnabled {
        return fmt.Errorf("server %s warming disabled", serverName)
    }
    if clear && wm.emailer != nil && wm.emailer.schedule.deleteDayPending != nil {
        if err := wm.emailer.schedule.deleteDayPending(serverName, day); err != nil {
            return err
        }
    }
    return wm.createDailySchedule(ws, day)
}

// getCampaign gets a campaign by ID (placeholder - needs to be implemented)
func (wm *WarmingManager) getCampaign(campaignID int) (*models.Campaign, error) {
    wm.mu.RLock()
    fetch := wm.fetchCampaign
    wm.mu.RUnlock()
    if fetch != nil {
        if camp, err := fetch(campaignID); err == nil && camp != nil {
            return camp, nil
        }
    }
    // Fallback dummy campaign.
    return &models.Campaign{
        Base: models.Base{
            ID: campaignID,
        },
        FromEmail: "",
        Subject:   "Warming email",
        Body:      "This is a warming email.",
    }, nil
}

// sendWarmingEmail sends a warming email using the specified server
func (wm *WarmingManager) sendWarmingEmail(server *Server, toEmail string, campaign *models.Campaign) error {
    _, err := wm.sendWarmingEmailWithSender(server, toEmail, campaign, "")
    return err
}

func (wm *WarmingManager) sendWarmingEmailWithSender(server *Server, toEmail string, campaign *models.Campaign, senderEmail string) (string, error) {
    // Determine a valid From address. Priority: schedule sender > campaign.FromEmail > server settings
    var from string
    
    // First priority: sender email from schedule
    if senderEmail != "" {
        if _, err := mail.ParseAddress(senderEmail); err == nil {
            from = senderEmail
        } else {
            elogf("WarmCloud: invalid schedule sender email '%s' for server %s: %v", senderEmail, server.Name, err)
        }
    }
    
    // Second priority: campaign.FromEmail
    if from == "" && campaign.FromEmail != "" {
        if _, err := mail.ParseAddress(campaign.FromEmail); err == nil {
            from = campaign.FromEmail
        } else {
            elogf("WarmCloud: invalid campaign From '%s' for server %s: %v", campaign.FromEmail, server.Name, err)
        }
    }
    
    // Third priority: server username
    if from == "" && server.Username != "" {
        if _, err := mail.ParseAddress(server.Username); err == nil {
            from = server.Username
        }
    }
    
    // Fourth priority: From header from server.EmailHeaders
    if from == "" {
        if fromHeader, exists := server.EmailHeaders["From"]; exists && fromHeader != "" {
            if _, err := mail.ParseAddress(fromHeader); err == nil {
                from = fromHeader
            }
        }
    }
    
    // Fallback: generate no-reply@domain
    if from == "" {
        domain := server.Host
        domain = strings.TrimPrefix(domain, "smtp.")
        from = fmt.Sprintf("no-reply@%s", domain)
        elogf("WarmCloud: using fallback From '%s' for server %s", from, server.Name)
    }

    // Build a minimal email and send via the specific server's pool.
    em := smtppool.Email{
        From:    from,
        To:      []string{toEmail},
        Subject: campaign.Subject,
        HTML:    []byte(campaign.Body),
    }

    if err := server.pool.Send(em); err != nil {
        return "", err
    }
    // Persist warmcloud-specific sent count via the emailer's onSent hook using a namespaced key.
    if wm.emailer != nil && wm.emailer.onSent != nil && server.Name != "" {
        wm.emailer.onSent("warmcloud:" + server.Name)
    }
    elogf("WarmCloud: отправлено письмо прогрева на %s через %s", toEmail, server.Name)
    return from, nil
}

// Campaign represents a campaign for warming (placeholder)

// StopWarming stops warming for a specific server
func (wm *WarmingManager) StopWarming(serverName string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if ws, exists := wm.servers[serverName]; exists {
		close(ws.StopChan)
		ws.Running = false
	}
}

// GetWarmingStatus returns the warming status for all servers
func (wm *WarmingManager) GetWarmingStatus() map[string]interface{} {
    wm.mu.RLock()
    defer wm.mu.RUnlock()

    status := make(map[string]interface{})
    for serverName, ws := range wm.servers {
        status[serverName] = map[string]interface{}{
            "running":         ws.Running,
            "current_limit":   ws.CurrentLimit,
            "daily_step":      ws.DailyStep,
            "campaign_id":     ws.CampaignID,
            "last_reset_date": ws.LastResetDate,
            "paused":          wm.paused[serverName],
            "hours_enabled":   ws.HoursEnabled,
            "last_forced_at":  ws.LastForcedAt,
            "status":          ws.Status,
            "status_detail":   ws.StatusDetail,
            "status_at":       ws.StatusAt,
            "next_at":         ws.NextAt,
        }
    }
    return status
}
