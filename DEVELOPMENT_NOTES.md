# Заметки разработчика - Listmonk

## TODO WarmCloud
- [ ] Сохранять warmcloud_sent_today в БД после ручного «Закончить на сегодня», чтобы показатели сохранялись после рестарта.
- [ ] Добавить индикатор выполнения для кнопки завершения дня в SMTP статистике.
- [ ] Уточнить фильтры `/api/warmcloud/sends`, заменив `created_at::date` на диапазоны дат.
- [ ] Подобрать более устойчивый способ распределения senders при пересоздании расписания (сейчас пересортировка идёт по order по due_at).

## Алгоритм работы конвейера кампаний
1. **Создание кампании (UI → REST)**
   - `frontend/src/views/Campaign.vue` отправляет `POST /api/campaigns` (или `PUT` при редактировании). Поля `messagesPerDay` → `rate_window_count/1440`.
   - REST-хендлеры (`cmd/campaigns.go`) валидируют и передают в `core.CreateCampaign`.

2. **Запись в БД (Core)**
   - `internal/core/campaigns.go` обновляет таблицы `campaigns`, `campaign_lists`, `campaign_media`, сбрасывает счётчики.
   - Статус по умолчанию `draft`/`scheduled`. Старт кампании — `PATCH /api/campaigns/:id/status=running`.

3. **Подхват менеджером**
   - `manager.Run()` периодически вызывает `next_campaigns` (SQL) → вычисляет `to_send`, `max_subscriber_id` и возвращает активные кампании.
   - Для каждой — `newPipe`: компиляция шаблона, загрузка вложений, настройка persona.
   - Если установлен дневной лимит (`rate_window_count>0`) — `planAllRemainingSlots()` формирует слоты в `campaign_schedule` (помощник `slotTimePlan`).

4. **Загрузка подписчиков**
   - `pipe.NextSubscribers()`
     - При наличии слотов: читает ближайший `CampSchedNextDue`, назначает подписчика (`AssignSlotSubscriber`), ожидает `due` неблокирующе (`enqueueAfter`).
     - Без слотов: использует `NextSubscribers`/`NextSequenceSubscribers` (bulk).
     - На каждое письмо формируется `CampaignMessage` (subject/body рендерятся, slot ID и persona подставляются).

5. **Worker → отправка**
   - `manager.worker` забирает `CampaignMessage`, собирает `models.Message`, гарантирует `Message-ID`.
   - Передает в SMTP-мессенджер (`email.Push`).
   - При `ErrDailyLimitReached` → лог + `pipe.pauseForDailyLimit()` (кампания уходит в `paused`).
   - При успехе → `campaign_sends`, `smtp_send_events`, отметка слота `CampSchedMarkDoneEmail`. Для sequence — обновление прогресса.
   - При ошибке SMTP → `RecordSend(success=false)` + `RecordImmediateBounce`.

6. **Очередь и завершение**
   - `pipe.wg` отслеживает все отправки; когда очередь опустеет, `pipe.cleanup()` обновляет статусы (`finished`/`scheduled`), отправляет уведомление.

7. **Лимиты и счётчики**
   - `smtp_stats` хранит per-SMTP `sent`. При старте `init.go` вызывает `RestoreSentToday`. `email.Push` проверяет `DailyLimit` и возвращает типизированную ошибку.

8. **WarmCloud (прогрев)**
   - `internal/messenger/email/warming.go` управляет таблицей `warmcloud_schedule`, использует те же `schedule`-хуки (insert/delete/nextDue).
   - События прогрева (`warmcloud_events`) записываются через `SetWarmEventHook`.

9. **Ответы и POP3**
   - Ответы подписчиков попадают в `campaign_replies` (через POP3-менеджер). В `campaign_sends.reply_at` фиксируется факт ответа — используется в дашборде/аналитике.

10. **UI/аналитика**
   - Dashboard читает `mat_dashboard_counts/charts` (материализованные представления). Графики и KPI строятся из JSON (`campaign_sends`, `campaign_replies`, `campaign_bounces`, `link_clicks`, `campaign_views`).
   - `SMTP Stats` и `Warmcloud` экраны используют API `/api/settings/smtp/stats`, `/api/warmcloud/...` для отображения лимитов, слотов, статусов прогрева.

> Этот блок обновлять по мере доработок (лимиты, UI, обработки ошибок).

## WarmCloud 2025-09-17
- Настроено автоматическое обновление `from_email` и `persona` у существующих pending-слотов перед добавлением новых, чтобы адрес отправителя всегда соответствовал актуальной кампании/личности.
- SQL upsert для `warmcloud_schedule` теперь перезаписывает `from_email`, `subject`, `persona` вместо игнорирования существующих записей.
- API-хуки `updatePersona`/`listDay` доступны менеджеру прогрева для бэкенд‑синхронизации.
- UI расписания показывает только реальный адрес отправителя, колонка «Личность» убрана во избежание дублирования.

## Campaign Schedule 2025-09-17
- Реализован экран «Расписание кампаний» с фильтрами, пагинацией, экспортом и группировками по кампаниям по аналогии с WarmCloud.
- API `/api/campaigns/schedule` теперь возвращает название и статус кампании и поддерживает фильтр по статусу.
- В таблице показываются реальные отправители (`email_from`), прогресс по квоте и действия над слотами (перенос времени, пропуск, удаление).

## Последние изменения в интерфейсах расписания

### Файл: `frontend/src/views/CampaignSchedule.vue` - Расписание кампаний

#### Проблемы, которые были решены:
1. **undefined ID кампании** - данные приходили с API, но поля имели разные названия
2. **Пустые поля "От кого" и "Кому"** - аналогичная проблема с названиями полей
3. **504 ошибки в модальных окнах** - асинхронные API запросы вызывали таймауты
4. **Не работающие кнопки группировки** - сложные модальные окна с компонентами

#### Решение:
1. **Универсальные методы извлечения данных**:
   ```javascript
   getCampaignId(row) {
     return row.campaign_id || row.campaignId || row.campaign?.id || '—';
   }
   ```
   Проверяют все возможные варианты названий полей в API.

2. **Упрощение модальных окон**:
   - Заменили `this.$buefy.modal.open()` с компонентами на `this.$buefy.dialog.alert()`
   - Убрали асинхронные API запросы
   - Используют данные уже загруженные в таблице

3. **Улучшенная обработка ошибок**:
   - Добавлены проверки на существование данных
   - Graceful fallback для отсутствующих полей

#### Новые методы:
- `getCampaignId(row)` - ID кампании
- `getCampaignName(row)` - название кампании
- `getEmailFrom(row)` - email отправителя
- `getFromName(row)` - имя отправителя
- `getEmailTo(row)` - email получателя
- `getSubscriberName(row)` - имя получателя
- `filterByCampaign(campaignId)` - фильтрация по кампании
- `showCampaignStatus(row)` - статус рассылки
- `showCampaignGroups()` - группировка кампаний

#### Архитектурные решения:
1. **Отказ от API запросов в модальных окнах** - все данные берутся из уже загруженной таблицы
2. **Универсальная обработка данных API** - поддержка разных форматов ответов сервера
3. **Простота UI компонентов** - использование встроенных диалогов Buefy вместо кастомных модальных окон

#### Тестирование:
- Проверено на данных с `undefined` полями
- Проверено на различных форматах API ответов
- Проверена работа модальных окон без таймаутов
- Проверена фильтрация и группировка данных

---

### Файл: `frontend/src/views/WarmcloudSchedule.vue` - Расписание прогрева

#### Проблемы, которые были решены:
1. **Отсутствие времени следующей отправки** - не показывалось когда будет следующая отправка
2. **Нет контроля квот** - не было видно лимитов отправок для серверов
3. **Неполная информация о серверах** - отсутствовала статистика по серверам
4. **Проблема с превышением лимитов** - серверы отправляли больше писем чем разрешено

#### Решение:
1. **Отображение времени следующей отправки**:
   ```javascript
   getNextSendTime(row) {
     const nextTime = row.nextAt || row.next_send_time || row.nextSendTime;
     // Показывает время в формате HH:mm
   }
   ```

2. **Контроль квот и лимитов**:
   ```javascript
   getQuotaInfo(row) {
     const quota = row.quota || row.quota_limit || row.daily_limit;
     const used = row.quota_used || row.sent_today || row.sent_count;
     // Показывает использование квоты с предупреждениями
   }
   ```

3. **Статистика по серверам**:
   - Модальное окно с группировкой по серверам
   - Отображение статистики отправок по каждому серверу
   - Контроль использования квот

4. **Количество прочитанных писем**:
   ```javascript
   getReadCount(row) {
     const readCount = row.read_count || row.readCount || row.reads || row.opened_count || row.openedCount;
     // Возвращает количество прочитанных писем или 0/1 на основе статуса
   }
   ```

5. **Пагинация**:
   ```javascript
   perPage() {
     return this.filters.limit === -1 ? this.displayRows.length : parseInt(this.filters.limit, 10);
   },
   paginatedRows() {
     const start = (this.currentPage - 1) * this.perPage;
     const end = start + this.perPage;
     return this.displayRows.slice(start, end);
   }
   ```

6. **Email отправителя**:
   ```javascript
   getSenderEmail(row) {
     const senderEmail = row.email_from || row.from_email || row.sender_email || row.from || row.fromAddress;
     // Возвращает email отправителя или "—" если не найден
   }
   ```

7. **Настройки по умолчанию**:
   ```javascript
   filters: {
     limit: -1, // По умолчанию показываем все записи
     day: new Date(), // Сегодняшняя дата
   }
   ```

8. **Модальное окно ошибок**:
   ```javascript
   showErrorDetails(row) {
     // Показывает модальное окно с деталями ошибки
     // При клике на статус "Ошибка"
   }
   ```

9. **Условная кнопка действий**:
   ```javascript
   // В шаблоне:
   <template v-if="p.row.status === 'pending'">
     <b-dropdown position="is-bottom-left">
       <!-- Действия только для ожидающих -->
     </b-dropdown>
   </template>
   <span v-else class="has-text-grey-light">—</span>
   ```

10. **Статистика SMTP - прогресс прогрева**:
    ```javascript
    getWarmcloudProgress(row) {
      const serverStatus = this.wstatus && this.wstatus[row.name];
      if (serverStatus) {
        const sent = serverStatus.sent_today || 0;
        const limit = serverStatus.daily_limit || row.warmcloudDailyLimit || 0;
        return `${sent}/${limit}`;
      }
      return `${row.warmcloudSentToday || 0}/${row.warmcloudDailyLimit || 0}`;
    }
    ```

11. **Улучшенное время следующей отправки**:
    ```javascript
    // В renderWarmState:
    if (s.nextAt) {
      const t = this.$utils.getDate(s.nextAt).format('HH:mm');
      next = ` <span class="tag is-light is-info">След. в: ${t}</span>`;
    } else if (s.next_send_time) {
      const t = this.$utils.getDate(s.next_send_time).format('HH:mm');
      next = ` <span class="tag is-light is-info">След. в: ${t}</span>`;
    }
    ```

#### Новые функции:
- **Время следующей отправки**: Показывается для каждого сервера
- **Контроль квот**: Предупреждения при приближении к лимиту
- **Статистика серверов**: Детальная информация по всем серверам
- **Фильтрация**: По статусу отправок
- **Улучшенная информация о серверах**: Статус, последняя отправка, общее количество
- **Количество прочитанных**: Отображение статистики прочтения писем
- **Пагинация**: Постраничная навигация для больших объемов данных
- **Выбор количества записей**: Выпадающий список (10, 50, 100, 500, 1000, все)
- **Улучшенное отображение email отправителя**: Корректное извлечение и отображение адресов отправителей
- **Настройки по умолчанию**: По умолчанию показываются все записи за сегодняшний день
- **Модальное окно ошибок**: Клик по статусу "Ошибка" открывает модальное окно с деталями
- **Условная кнопка действий**: Активна только для записей со статусом "ожидает"
- **Выпадающий список действий**: В стиле статистики SMTP
- **Ссылки в статистике SMTP**: Процесс прогрева теперь ведет на расписание прогрева
- **Исправленные лимиты квоты**: Данные берутся из расписания для корректного отображения
- **Улучшенное время отправки**: Показывается для всех серверов в статистике SMTP

#### Рекомендации для будущих изменений:
1. При работе с API всегда использовать универсальные методы извлечения данных
2. Избегать сложных модальных окон с асинхронными запросами
3. Предпочитать простые UI компоненты для лучшей производительности
4. Добавлять отладочные логи для понимания структуры данных API
5. Всегда отображать информацию о лимитах и квотах для контроля отправок
6. Предоставлять детальную статистику для мониторинга работы серверов

---

## Исправления ESLint - 2025-01-15

### Проблема
При сборке Docker контейнера возникали ошибки ESLint, которые блокировали процесс сборки:

```
/app/frontend/src/views/SmtpStats.vue
  341:6  error  Parsing error: Unexpected token if
  39:16  error  Trailing spaces not allowed  no-trailing-spaces
```

### Решение
Исправлены все ошибки ESLint в файлах Vue.js компонентов.

### Детальные исправления в SmtpStats.vue:

#### 1. Синтаксические ошибки (строки 340, 328, 434, 579, 608, 359)
**Проблема**: Комментарии слипались с кодом без переноса строки
```javascript
// Было:
// Fallback на старые данныеconst serverStatus = this.wstatus && this.wstatus[serverName];

// Стало:
// Fallback на старые данные
const serverStatus = this.wstatus && this.wstatus[serverName];
```

#### 2. Ошибки форматирования (строка 39 в WarmcloudSchedule.vue)
**Проблема**: Лишние пробелы в конце строки
```vue
<!-- Было: -->
<b-select 

<!-- Стало: -->
<b-select
```

#### 3. Операторы инкремента (строки 287, 291, 293, 310, 312, 314)
**Проблема**: ESLint запрещает операторы `++`
```javascript
// Было:
grouped[server].total++;
grouped[server].done++;
grouped[server].pending++;
grouped[server].read++;
grouped[server].error++;

// Стало:
grouped[server].total += 1;
grouped[server].done += 1;
grouped[server].pending += 1;
grouped[server].read += 1;
grouped[server].error += 1;
```

#### 4. Стрелочные функции (строка 272)
**Проблема**: Отсутствие скобок вокруг аргумента
```javascript
// Было:
scheduleData.forEach(item => {

// Стало:
scheduleData.forEach((item) => {
```

#### 5. Цепочки методов (строка 261)
**Проблема**: Отсутствие переноса строки перед `.catch()`
```javascript
// Было:
      }).catch((err) => {

// Стало:
      })
        .catch((err) => {
```

#### 6. Массивы (строки 441, 591)
**Проблема**: Отсутствие запятых в конце массивов
```javascript
// Было:
        s.scheduledTime
      ];

// Стало:
        s.scheduledTime,
      ];
```

#### 7. Возвращаемые значения (строка 350)
**Проблема**: Метод без явного возвращаемого значения
```javascript
// Было:
    getWarmcloudReadCount(row) {
      const serverName = row.name;
      if (this.warmcloudScheduleData && this.warmcloudScheduleData[serverName]) {
        const scheduleData = this.warmcloudScheduleData[serverName];
        return scheduleData.read || 0;
      }
      // Fallback на старые данныеreturn row.warmcloudSubmittedToday || 0;
    },

// Стало:
    getWarmcloudReadCount(row) {
      const serverName = row.name;
      if (this.warmcloudScheduleData && this.warmcloudScheduleData[serverName]) {
        const scheduleData = this.warmcloudScheduleData[serverName];
        return scheduleData.read || 0;
      }
      // Fallback на старые данные
      return row.warmcloudSubmittedToday || 0;
    },
```

#### 8. Отступы (строки 262, 264, 265, 266)
**Проблема**: Неправильные отступы в блоке `.catch()`
```javascript
// Было:
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.error('Error fetching SMTP stats:', err);
        this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
      });

// Стало:
        .catch((err) => {
          // eslint-disable-next-line no-console
          console.error('Error fetching SMTP stats:', err);
          this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
        });
```

### Команды для проверки ESLint:
```bash
# Проверка всех файлов
cd frontend && yarn lint

# Проверка конкретного файла
cd frontend && yarn eslint src/views/SmtpStats.vue

# Автоматическое исправление (если возможно)
cd frontend && yarn eslint --fix src/views/SmtpStats.vue
```

### Рекомендации для будущих изменений:
1. **Всегда запускать ESLint перед коммитом** - это предотвратит ошибки сборки
2. **Использовать автоматическое исправление** - `yarn eslint --fix` исправляет многие ошибки автоматически
3. **Проверять отступы** - ESLint строго следит за правильными отступами
4. **Избегать операторов `++` и `--`** - использовать `+= 1` и `-= 1`
5. **Добавлять скобки в стрелочные функции** - даже для одного аргумента
6. **Проверять запятые в массивах** - ESLint требует trailing commas
7. **Убеждаться в возвращаемых значениях** - все методы должны иметь явный return

### Результат:
- ✅ Успешная сборка Docker контейнера
- ✅ 0 ошибок ESLint
- ✅ Соответствие стандартам кодирования
- ✅ Улучшенная читаемость кода

---

*Создано: 2025-01-15*
*Автор: AI Assistant*
*Обновлено: 2025-01-15 - Добавлены исправления ESLint*
