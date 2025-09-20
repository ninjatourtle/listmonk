<template>
  <section>
    <div class="is-flex is-justify-content-space-between is-align-items-center mb-3">
      <h1 class="title mb-0">
        <b-icon icon="chart-timeline-variant" class="mr-2" />
        {{ $t('smtp.stats.title') }}
      </h1>
      <div>
        <b-button class="mr-2" size="is-small" @click="openWarmcloudLogs">
          {{ $t('smtp.stats.warmcloudLogs') || 'Логи прогрева' }}
        </b-button>
        <router-link :to="{ name: 'warmcloudSends' }">
          <b-button class="mr-2" size="is-small">
            {{ $t('warmcloud.sends.title') || 'Последние отправки прогрева' }}
          </b-button>
        </router-link>
        <b-button size="is-small" type="is-info" :loading="checkingBalance" @click="checkWarmcloudBalance">
          {{ $t('smtp.stats.checkBalance') || 'Проверить баланс Warmcloud' }}
        </b-button>
      </div>
    </div>
    <b-table :data="stats" :striped="true" :row-class="smtpRowClass">
      <b-table-column field="name" :label="$t('globals.fields.name')">
        <template #default="props">
          {{ props.row.name || '—' }}
        </template>
      </b-table-column>
      <b-table-column field="sentTotal" :label="$t('smtp.stats.total')">
        <template #default="props">
          <router-link :to="{ name: 'smtpSends', query: { server: props.row.normalizedName } }">
            {{ props.row.sentTotal || 0 }}
          </router-link>
        </template>
      </b-table-column>
      <b-table-column field="sentToday" :label="$t('smtp.stats.today')">
        <template #default="props">
          <router-link :to="{ name: 'smtpSends', query: { server: props.row.normalizedName, today: 1 } }">
            {{ props.row.sentToday || 0 }}
          </router-link>
        </template>
      </b-table-column>
      <b-table-column field="dailyLimit" :label="$t('smtp.stats.dailyLimit')">
        <template #default="props">
          <div v-if="limitMeta(props.row)" class="smtp-limit">
            <div class="smtp-limit__row">
              <strong>{{ limitMeta(props.row).sent }}/{{ limitMeta(props.row).limit }}</strong>
              <span :class="['smtp-limit__status', { 'is-warning': limitMeta(props.row).reached }]">
                {{ limitMeta(props.row).reached ? $t('dashboard.smtp.limitReached') : $t('dashboard.smtp.limitAvailable') }}
              </span>
            </div>
            <div class="smtp-limit__bar" role="progressbar"
              :aria-valuenow="limitMeta(props.row).sent"
              aria-valuemin="0"
              :aria-valuemax="limitMeta(props.row).limit"
            >
              <div class="smtp-limit__bar-fill" :style="{ width: `${limitMeta(props.row).percentage}%` }" />
            </div>
          </div>
          <span v-else class="has-text-grey-light">—</span>
        </template>
      </b-table-column>
      <b-table-column field="warmcloudEnabled" :label="$t('smtp.stats.warmcloud')">
        <template #default="props">
          <b-switch v-model="props.row.warmcloudEnabled" @input="toggleWarmCloud(props.row)" size="is-small" />
        </template>
      </b-table-column>
      <b-table-column field="warmcloudProgress" :label="$t('smtp.stats.warmcloudProgress')">
        <template #default="props">
          <span v-if="props.row.warmcloudEnabled">
            <router-link :to="{ name: 'warmcloudSchedule', query: { server: props.row.normalizedName } }">
              <span v-html="getWarmcloudProgress(props.row)" />
            </router-link>
          </span>
          <span v-else class="has-text-grey-light">—</span>
        </template>
      </b-table-column>
      <b-table-column field="warmcloudRead" :label="$t('smtp.stats.warmcloudRead') || 'Прочитано, шт'">
        <template #default="props">
          <span v-if="props.row.warmcloudEnabled">
            {{ getWarmcloudReadCount(props.row) }}
          </span>
          <span v-else class="has-text-grey-light">—</span>
        </template>
      </b-table-column>
      <b-table-column field="warmcloudState" :label="$t('smtp.stats.warmcloudState') || 'Статус прогрева'">
        <template #default="props">
          <span v-if="props.row.warmcloudEnabled">
            <span :class="getWarmStateClasses(props.row)">
              {{ getWarmStateText(props.row) }}
              <small v-if="getWarmStateDetail(props.row)" class="has-text-grey">({{ getWarmStateDetail(props.row) }})</small>
            </span>
            <span v-if="getWarmStateNextTime(props.row)" :class="getWarmStateNextClasses()">
              {{ getWarmStateNextText(props.row) }}
            </span>
          </span>
          <span v-else class="has-text-grey-light">—</span>
        </template>
      </b-table-column>
      <b-table-column field="actions" :label="$t('globals.terms.actions')">
        <template #default="props">
          <b-dropdown position="is-bottom-left">
            <template #trigger>
              <b-button size="is-small">{{ $t('globals.terms.actions') }}</b-button>
            </template>
            <b-dropdown-item @click.native="openSettings(props.row)">{{ $t('globals.buttons.settings') }}</b-dropdown-item>
            <b-dropdown-item @click.native="openCheck(props.row)">{{ $t('smtp.stats.check') || 'Проверка' }}</b-dropdown-item>
            <b-dropdown-item @click.native="togglePause(props.row)">
              {{ isPaused(props.row) ? ($t('globals.buttons.resume') || 'Возобновить') : ($t('globals.buttons.pause') || 'Пауза') }}
            </b-dropdown-item>
            <b-dropdown-item @click.native="forceTick(props.row)">{{ $t('smtp.stats.force') || 'Запустить сейчас' }}</b-dropdown-item>
            <b-dropdown-item @click.native="resetDaily(props.row)">{{ $t('smtp.stats.resetDaily') || 'Сбросить дневной' }}</b-dropdown-item>
            <b-dropdown-item @click.native="finishWarmcloudToday(props.row)">{{ $t('warmcloud.finishToday') || 'Закончить на сегодня' }}</b-dropdown-item>
            <b-dropdown-item>
              <router-link :to="{ name: 'warmcloudSchedule', query: { server: props.row.normalizedName } }">{{ $t('warmcloud.schedule') || 'Расписание' }}</router-link>
            </b-dropdown-item>
            <b-dropdown-item @click.native="rebuildSchedule(props.row)">{{ $t('smtp.stats.rebuild') || 'Перестроить' }}</b-dropdown-item>
            <b-dropdown-item @click.native="clearSchedule(props.row)">{{ $t('smtp.stats.clearSchedule') || 'Очистить расписание' }}</b-dropdown-item>
          </b-dropdown>
        </template>
      </b-table-column>
    </b-table>
    <b-modal :active.sync="showModal">
      <div class="box">
        <h4 class="title is-4">{{ $t('globals.buttons.settings') }} - {{ current.name }}</h4>

        <!-- Basic Settings -->
        <h5 class="title is-5">{{ $t('smtp.stats.basicSettings') }}</h5>
        <b-field :label="$t('smtp.stats.dailyLimit')">
          <b-input id="daily-limit" name="daily-limit" v-model="current.dailyLimit" type="number" min="0" />
        </b-field>
        <b-field :label="$t('smtp.stats.sendPause')">
          <b-input id="send-pause" name="send-pause" v-model="current.sendPause" placeholder="0s" />
        </b-field>

        <!-- WarmCloud per-server settings (global settings reside under Settings) -->
        <hr />
        <h5 class="title is-5">{{ $t('settings.warmcloud.title') }}</h5>
        <b-field :label="$t('settings.warmcloud.enabled')">
          <b-switch v-model="current.warmcloudEnabled" />
        </b-field>

        <div v-if="current.warmcloudEnabled">
          <b-field :label="$t('settings.warmcloud.big') || 'Provider (big)'" :message="$t('settings.warmcloud.bigHelp') || 'm — mail.ru, g — gmail.com'">
            <b-select id="warmcloud-big" name="warmcloud-big" v-model="current.warmcloudBig" placeholder="m">
              <option value="m">mail.ru (m)</option>
              <option value="g">gmail.com (g)</option>
            </b-select>
          </b-field>
          <b-field :label="$t('settings.warmcloud.campaignId')">
            <b-input id="warmcloud-campaign-id" name="warmcloud-campaign-id" v-model="current.warmcloudCampaignId" type="number" min="1" placeholder="1" />
          </b-field>
          <b-field :label="$t('settings.warmcloud.workingHoursPerServer') || 'Рабочие часы (пер-сервер)'">
            <div>
              <b-switch v-model="current.warmcloudWorkingHoursEnabled" class="mr-3">
                {{ warmHoursLabel() }}
              </b-switch>
              <b-input
                id="warmcloud-working-hours"
                name="warmcloud-working-hours"
                v-model="current.warmcloudWorkingHours"
                :disabled="!current.warmcloudWorkingHoursEnabled"
                placeholder="например: 9-12,14-18"
              />
            </div>
          </b-field>
          <b-field :label="$t('settings.warmcloud.dailyLimit')">
            <b-input id="warmcloud-daily-limit" name="warmcloud-daily-limit" v-model="current.warmcloudDailyLimit" type="number" min="1" placeholder="10" />
          </b-field>
          <b-field :label="$t('settings.warmcloud.dailyStep')">
            <b-input id="warmcloud-daily-step" name="warmcloud-daily-step" v-model="current.warmcloudDailyStep" type="number" min="1" max="50" placeholder="5" />
          </b-field>
          <b-field :label="$t('settings.warmcloud.readPercent') || 'Read percent (%)'">
            <b-input id="warmcloud-read-percent" name="warmcloud-read-percent" v-model="current.warmcloudReadPercent" type="number" min="0" max="100" placeholder="50" />
          </b-field>
          <b-field :label="$t('settings.warmcloud.ratePerWindow') || 'Писем за окно'" :message="$t('settings.warmcloud.ratePerWindowHelp') || 'Сколько писем отправлять за окно времени'">
            <b-input id="warmcloud-rate-count" name="warmcloud-rate-count" v-model="current.warmcloudRateCount" type="number" min="0" placeholder="20" />
          </b-field>
          <b-field :label="$t('settings.warmcloud.rateWindowMinutes') || 'Длительность окна (мин)'" :message="$t('settings.warmcloud.rateWindowMinutesHelp') || 'Например, 5 = каждые 5 минут'">
            <b-input id="warmcloud-rate-window" name="warmcloud-rate-window" v-model="current.warmcloudRateWindowMinutes" type="number" min="0" placeholder="5" />
          </b-field>
          <b-field
            :label="$t('settings.warmcloud.rateJitterPct') || 'Джиттер (%)'"
            :message="$t('settings.warmcloud.rateJitterPctHelp') || 'Случайное размазывание нагрузки (0–50)'"
          >
            <b-input
              id="warmcloud-rate-jitter"
              name="warmcloud-rate-jitter"
              v-model="current.warmcloudRateJitterPct"
              type="number"
              min="0"
              max="50"
              placeholder="20"
            />
          </b-field>
          <!-- Removed legacy throttles: handled by rate per window -->
        </div>

        <div class="buttons">
          <b-button type="is-primary" @click="save" :loading="saving">{{ $t('globals.buttons.save') }}</b-button>
          <b-button @click="showModal = false">{{ $t('globals.buttons.cancel') }}</b-button>
        </div>
      </div>
    </b-modal>

    <b-modal :active.sync="showCheck" :width="640">
      <div class="box">
        <h4 class="title is-4">{{ $t('smtp.stats.checkTitle') || 'Проверка SMTP' }} - {{ current.name }}</h4>
        <b-loading :is-full-page="false" :active="checking" />
        <div v-if="check">
          <p><strong>Host:</strong> {{ check.host }}:{{ check.port }}</p>
          <p>
            <strong>{{ $t('smtp.stats.check.connection') || 'Соединение' }}:</strong>
            <span :class="check.connectionOk ? 'has-text-success' : 'has-text-danger'">
              {{ check.connectionOk ? 'OK' : 'Ошибка' }}
            </span>
          </p>
          <p v-if="check.banner"><strong>Banner:</strong> <code>{{ check.banner }}</code></p>
          <p><strong>Domain:</strong> {{ check.domain }}</p>
          <p><strong>SPF:</strong> <span :class="check.spfOk ? 'has-text-success' : 'has-text-danger'">{{ check.spfOk ? 'OK' : 'Нет' }}</span></p>
          <p v-if="check.spfRecord"><small><code>{{ check.spfRecord }}</code></small></p>
          <p><strong>DMARC:</strong> <span :class="check.dmarcOk ? 'has-text-success' : 'has-text-danger'">{{ check.dmarcOk ? 'OK' : 'Нет' }}</span></p>
          <p v-if="check.dmarcRecord"><small><code>{{ check.dmarcRecord }}</code></small></p>
          <p><strong>DKIM:</strong> {{ check.dkimInfo }}</p>
        </div>
        <div class="buttons mt-4">
          <b-button @click="showCheck = false">{{ $t('globals.buttons.close') }}</b-button>
        </div>
      </div>
    </b-modal>
  </section>
</template>

<script>
export default {
  data() {
    return {
      stats: [],
      wstatus: {},
      checkingBalance: false,
      showModal: false,
      showCheck: false,
      current: {},
      saving: false,
      checking: false,
      check: null,
      warmcloudScheduleData: {},
      isLoadingScheduleData: true,
    };
  },
  methods: {
    warmHoursLabel() {
      if (this.current && this.current.warmcloudWorkingHoursEnabled) {
        return this.$t('globals.buttons.enabled') || 'Вкл';
      }
      return this.$t('globals.buttons.disabled') || 'Выкл';
    },
    async checkWarmcloudBalance() {
      try {
        this.checkingBalance = true;
        const resp = await this.$api.getWarmCloudBalance();
        const bal = (resp && resp.balance != null) ? resp.balance : 0;
        this.$utils.toast(`${this.$t('smtp.stats.balance') || 'Баланс'}: ${bal}`);
      } catch (e) {
        const msg = (e && e.response && e.response.data && e.response.data.message)
          ? e.response.data.message
          : (this.$t('globals.messages.internalError') || 'Внутренняя ошибка сервера');
        this.$utils.toast(msg, 'is-danger');
      } finally {
        this.checkingBalance = false;
      }
    },
    openWarmcloudLogs() {
      window.open('/admin/warmcloud-logs', '_blank');
    },
    fetch() {
      this.isLoadingScheduleData = true;
      this.$api.getSMTPStats().then((resp) => {
        this.stats = resp;
        return this.$api.getWarmCloudStatus();
      }).then((st) => {
        if (st) this.wstatus = st;
        // Получаем данные расписания для каждого сервера отдельно
        const today = this.$utils.getDate().format('YYYY-MM-DD');
        const schedulePromises = this.stats
          .filter((server) => server.warmcloudEnabled)
          .map((server) => this.$api.getWarmCloudSchedule({
            server: server.normalizedName,
            limit: -1,
            date: today,
          }).catch((err) => {
            // eslint-disable-next-line no-console
            console.warn(`Failed to fetch schedule for server ${server.name}:`, err);
            return [];
          }));

        return Promise.all(schedulePromises);
      }).then((scheduleDataArray) => {
        // Объединяем данные от всех серверов
        const allScheduleData = scheduleDataArray.flat();
        this.processWarmcloudScheduleData(allScheduleData);
        this.isLoadingScheduleData = false;
      })
        .catch((err) => {
          // eslint-disable-next-line no-console
          console.error('Error fetching SMTP stats:', err);
          this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
          this.isLoadingScheduleData = false;
        });
    },
    processWarmcloudScheduleData(scheduleData) {
      // Группируем данные расписания по серверам
      const grouped = {};
      const today = this.$utils.getDate().format('YYYY-MM-DD');

      if (scheduleData && Array.isArray(scheduleData)) {
        scheduleData.forEach((item) => {
          const server = item.server || item.server_name;
          if (!server) return;

          // Проверяем, что запись относится к сегодняшнему дню
          try {
            const itemDate = this.$utils.getDate(item.due_at || item.dueAt).format('YYYY-MM-DD');
            if (itemDate !== today) return;
          } catch (e) {
            // Игнорируем записи с некорректными датами
            return;
          }

          if (!grouped[server]) {
            grouped[server] = {
              total: 0,
              done: 0,
              pending: 0,
              error: 0,
              read: 0,
              nextSendTime: null,
            };
          }

          grouped[server].total += 1;

          const status = item.status || item.state;
          if (status === 'done' || status === 'submitted') {
            grouped[server].done += 1;
          } else if (status === 'pending' || status === 'waiting') {
            grouped[server].pending += 1;
            // Для pending записей ищем время следующей отправки
            const nextTime = item.dueAt || item.due_at || item.scheduled_time || item.next_send_time;
            if (nextTime && !grouped[server].nextSendTime) {
              try {
                const parsedTime = this.$utils.getDate(nextTime);
                const now = this.$utils.getDate();
                // Проверяем, что время в будущем (в течение 24 часов)
                const diffHours = parsedTime.diff(now, 'hours');
                if (diffHours >= 0 && diffHours <= 24) {
                  grouped[server].nextSendTime = nextTime;
                }
              } catch (e) {
                // Игнорируем ошибки парсинга
              }
            }
          } else if (status === 'error' || status === 'failed') {
            grouped[server].error += 1;
          } else if (status === 'read' || status === 'opened') {
            grouped[server].read += 1;
            // Прочитанные НЕ считаются отправленными отдельно, они уже включены в done
          }
        });
      }

      this.warmcloudScheduleData = grouped;
    },
    normalizedName(row) {
      // Normalize to backend server key: lowercased, non-alnum -> '-', with 'email-' prefix
      const s = String(row.name || '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
      return s && !s.startsWith('email-') ? `email-${s}` : s;
    },
    getWarmcloudProgress(row) {
      // Показываем индикатор загрузки пока данные не загружены
      if (this.isLoadingScheduleData) {
        return '<span class="has-text-grey">Загрузка...</span>';
      }

      const serverName = row.name;
      const normalizedServerName = this.normalizedName(row);
      // Получаем данные из расписания прогрева для этого сервера
      if (this.warmcloudScheduleData && this.warmcloudScheduleData[normalizedServerName]) {
        const scheduleData = this.warmcloudScheduleData[normalizedServerName];

        // Первое число: сумма отправленных (done) и прочитанных (read)
        const sent = (scheduleData.done || 0) + (scheduleData.read || 0);

        // Второе число: все записи на сегодня
        const total = scheduleData.total || 0;

        // Количество ошибок в скобках (красным цветом)
        const errors = scheduleData.error || 0;
        if (errors > 0) {
          return `${sent}/${total} <span class="has-text-danger">(${errors})</span>`;
        }

        return `${sent}/${total}`;
      }

      // Fallback на старые данные
      const serverStatus = this.wstatus && this.wstatus[serverName];
      if (serverStatus) {
        const sent = serverStatus.sent_today || serverStatus.sentToday || 0;
        const limit = serverStatus.daily_limit || serverStatus.dailyLimit || row.warmcloudDailyLimit || 0;
        return `${sent}/${limit}`;
      }
      return `${row.warmcloudSentToday || 0}/${row.warmcloudCurrentLimit || row.warmcloudDailyLimit || 0}`;
    },
    getWarmcloudReadCount(row) {
      // Показываем индикатор загрузки пока данные не загружены
      if (this.isLoadingScheduleData) {
        return '—';
      }

      const normalizedServerName = this.normalizedName(row);
      // Получаем данные из расписания прогрева для этого сервера
      if (this.warmcloudScheduleData && this.warmcloudScheduleData[normalizedServerName]) {
        const scheduleData = this.warmcloudScheduleData[normalizedServerName];
        return scheduleData.read || 0;
      }

      // Fallback на старые данные
      return row.warmcloudSubmittedToday || 0;
    },
    isPaused(row) {
      const n = row.name;
      const s = this.wstatus && this.wstatus[n];
      return !!(s && s.paused);
    },
    renderWarmState(row) {
      const s = (this.wstatus && this.wstatus[row.name]) || {};
      let code = s.status;
      if (!code) {
        if (s.running) code = 'running';
        else if (s.paused) code = 'paused';
        else code = 'idle';
      }

      // Временная отладочная информация для проблемных серверов
      if (row.name === 'tetktorg-ru' || row.name === 'kbslogistic-ru') {
        // eslint-disable-next-line no-console
        console.log(`Debug ${row.name}:`, {
          status: s,
          code,
          nextAt: s.nextAt,
          next_send_time: s.next_send_time,
          nextSendTime: s.nextSendTime,
          nextSend: s.nextSend,
          scheduled_time: s.scheduled_time,
          scheduledTime: s.scheduledTime,
        });
      }
      const labels = {
        running: this.$t('smtp.state.running') || 'Отправляет',
        starting: this.$t('smtp.state.starting') || 'Запуск',
        sending: this.$t('smtp.state.sending') || 'Отправляет',
        submitting: this.$t('smtp.state.submitting') || 'Подтверждает',
        submitted: this.$t('smtp.state.submitted') || 'Подтверждено',
        sleep: this.$t('smtp.state.sleep') || 'Пауза',
        wait_hours: this.$t('smtp.state.wait_hours') || 'Ожидает окно',
        paused: this.$t('smtp.state.paused') || 'На паузе',
        limit_reached: this.$t('smtp.state.limit_reached') || 'Лимит достигнут',
        requesting: this.$t('smtp.state.requesting') || 'Запрашивает адрес',
        clearing_rotation: this.$t('smtp.state.clearing_rotation') || 'Чистит ротацию',
        no_emails: this.$t('smtp.state.no_emails') || 'Нет адресов',
        checking_balance: this.$t('smtp.state.checking_balance') || 'Проверка баланса',
        no_balance: this.$t('smtp.state.no_balance') || 'Нет баланса',
        invalid_big: this.$t('smtp.state.invalid_big') || 'Неверный провайдер',
        error: this.$t('smtp.state.error') || 'Ошибка',
        idle: this.$t('smtp.state.idle') || 'Ожидает',
      };
      const typeMap = {
        submitted: 'is-success',
        running: 'is-info',
        starting: 'is-info',
        sending: 'is-info',
        submitting: 'is-info',
        requesting: 'is-info',
        clearing_rotation: 'is-info',
        sleep: 'is-warning',
        wait_hours: 'is-warning',
        paused: 'is-warning',
        limit_reached: 'is-warning',
        no_emails: 'is-warning',
        checking_balance: 'is-info',
        no_balance: 'is-danger',
        invalid_big: 'is-danger',
        error: 'is-danger',
        idle: 'is-light',
      };
      const text = labels[code] || code;
      const cls = typeMap[code] || 'is-light';
      const detail = s.status_detail ? ` <small class="has-text-grey">(${s.status_detail})</small>` : '';

      // Next action badge
      let next = '';
      let nextTime = null;

      // Пробуем разные поля для получения времени следующей отправки
      const timeFields = [
        s.nextAt,
        s.next_send_time,
        s.nextSendTime,
        s.nextSend,
        s.scheduled_time,
        s.scheduledTime,
      ];

      timeFields.some((timeField) => {
        if (timeField) {
          try {
            nextTime = this.$utils.getDate(timeField).format('HH:mm');
            return true; // Останавливаем поиск
          } catch (e) {
            // Продолжаем поиск в следующих полях
            return false;
          }
        }
        return false;
      });

      if (nextTime) {
        const lbl = this.$t('smtp.stats.nextAt') || 'След. в';
        next = ` <span class="tag is-light is-info">${lbl}: ${nextTime}</span>`;
      } else if (code !== 'submitted' && code !== 'done' && code !== 'error') {
        // Если время не найдено, пробуем получить из расписания прогрева
        // Это может помочь для серверов, где данные приходят из другого источника
        const lbl = this.$t('smtp.stats.nextAt') || 'След. в';

        // Попробуем получить время из дополнительных полей
        const additionalTimeFields = [
          s.scheduled_time,
          s.scheduledTime,
          s.due_time,
          s.dueTime,
          s.planned_time,
          s.plannedTime,
        ];

        let fallbackTime = null;
        additionalTimeFields.some((timeField) => {
          if (timeField) {
            try {
              const parsedTime = this.$utils.getDate(timeField);
              // Проверяем, что время в будущем (в течение 24 часов)
              const now = this.$utils.getDate();
              const diffHours = parsedTime.diff(now, 'hours');
              if (diffHours >= 0 && diffHours <= 24) {
                fallbackTime = parsedTime.format('HH:mm');
                return true; // Останавливаем поиск
              }
            } catch (e) {
              // Продолжаем поиск
            }
          }
          return false;
        });

        if (fallbackTime) {
          next = ` <span class="tag is-light is-info">${lbl}: ${fallbackTime}</span>`;
        } else {
          next = ` <span class="tag is-light is-info">${lbl}: --:--</span>`;
        }
      }

      return `<span class="tag ${cls} is-light">${text}${detail}</span>${next}`;
    },
    getWarmStateClasses(row) {
      // Показываем стиль загрузки пока данные не загружены
      if (this.isLoadingScheduleData) {
        return 'tag is-light';
      }

      const s = (this.wstatus && this.wstatus[row.name]) || {};
      let code = s.status;
      if (!code) {
        if (s.running) code = 'running';
        else if (s.paused) code = 'paused';
        else code = 'idle';
      }

      // Проверяем, есть ли pending записи на сегодня в расписании прогрева
      const normalizedServerName = this.normalizedName(row);
      if (this.warmcloudScheduleData && this.warmcloudScheduleData[normalizedServerName]) {
        const scheduleData = this.warmcloudScheduleData[normalizedServerName];
        // Если нет pending записей на сегодня, показываем стиль для "Закончил"
        if (!scheduleData.pending || scheduleData.pending === 0) {
          return 'tag is-success is-light';
        }
      }

      const typeMap = {
        submitted: 'is-success',
        running: 'is-info',
        starting: 'is-info',
        sending: 'is-info',
        submitting: 'is-info',
        requesting: 'is-info',
        clearing_rotation: 'is-info',
        sleep: 'is-warning',
        wait_hours: 'is-warning',
        paused: 'is-warning',
        limit_reached: 'is-warning',
        no_emails: 'is-warning',
        checking_balance: 'is-info',
        no_balance: 'is-danger',
        invalid_big: 'is-danger',
        error: 'is-danger',
        idle: 'is-light',
      };

      const cls = typeMap[code] || 'is-light';
      return `tag ${cls} is-light`;
    },
    getWarmStateText(row) {
      // Показываем индикатор загрузки пока данные не загружены
      if (this.isLoadingScheduleData) {
        return 'Загрузка...';
      }

      const s = (this.wstatus && this.wstatus[row.name]) || {};
      let code = s.status;
      if (!code) {
        if (s.running) code = 'running';
        else if (s.paused) code = 'paused';
        else code = 'idle';
      }

      // Проверяем, есть ли pending записи на сегодня в расписании прогрева
      const normalizedServerName = this.normalizedName(row);
      if (this.warmcloudScheduleData && this.warmcloudScheduleData[normalizedServerName]) {
        const scheduleData = this.warmcloudScheduleData[normalizedServerName];
        // Если нет pending записей на сегодня, показываем "Закончил"
        if (!scheduleData.pending || scheduleData.pending === 0) {
          return 'Закончил';
        }
      }

      const labels = {
        running: this.$t('smtp.state.running') || 'Отправляет',
        starting: this.$t('smtp.state.starting') || 'Запуск',
        sending: this.$t('smtp.state.sending') || 'Отправляет',
        submitting: this.$t('smtp.state.submitting') || 'Подтверждает',
        submitted: this.$t('smtp.state.submitted') || 'Подтверждено',
        sleep: this.$t('smtp.state.sleep') || 'Пауза',
        wait_hours: this.$t('smtp.state.wait_hours') || 'Ожидает окно',
        paused: this.$t('smtp.state.paused') || 'На паузе',
        limit_reached: this.$t('smtp.state.limit_reached') || 'Лимит достигнут',
        requesting: this.$t('smtp.state.requesting') || 'Запрашивает адрес',
        clearing_rotation: this.$t('smtp.state.clearing_rotation') || 'Чистит ротацию',
        no_emails: this.$t('smtp.state.no_emails') || 'Нет адресов',
        checking_balance: this.$t('smtp.state.checking_balance') || 'Проверка баланса',
        no_balance: this.$t('smtp.state.no_balance') || 'Нет баланса',
        invalid_big: this.$t('smtp.state.invalid_big') || 'Неверный провайдер',
        error: this.$t('smtp.state.error') || 'Ошибка',
        idle: this.$t('smtp.state.idle') || 'Ожидает',
      };

      return labels[code] || code;
    },
    getWarmStateDetail(row) {
      const s = (this.wstatus && this.wstatus[row.name]) || {};
      return s.status_detail || null;
    },
    getWarmStateNextTime(row) {
      // Не показываем время следующей отправки пока данные не загружены
      if (this.isLoadingScheduleData) {
        return null;
      }

      const s = (this.wstatus && this.wstatus[row.name]) || {};
      let code = s.status;
      if (!code) {
        if (s.running) code = 'running';
        else if (s.paused) code = 'paused';
        else code = 'idle';
      }

      // Проверяем только для активных статусов
      if (code === 'submitted' || code === 'done' || code === 'error') {
        return null;
      }

      // Проверяем, есть ли pending записи на сегодня в расписании прогрева
      const normalizedServerName = this.normalizedName(row);
      if (this.warmcloudScheduleData && this.warmcloudScheduleData[normalizedServerName]) {
        const scheduleData = this.warmcloudScheduleData[normalizedServerName];
        // Если нет pending записей на сегодня, не показываем время
        if (!scheduleData.pending || scheduleData.pending === 0) {
          return null;
        }
      }

      // Пробуем разные поля для получения времени следующей отправки
      const timeFields = [
        s.nextAt,
        s.next_send_time,
        s.nextSendTime,
        s.nextSend,
        s.scheduled_time,
        s.scheduledTime,
      ];

      let nextTime = null;
      timeFields.some((timeField) => {
        if (timeField) {
          try {
            nextTime = this.$utils.getDate(timeField).format('HH:mm');
            return true; // Останавливаем поиск
          } catch (e) {
            // Продолжаем поиск в следующих полях
            return false;
          }
        }
        return false;
      });

      // Если время не найдено в wstatus, пробуем найти в расписании прогрева
      if (!nextTime && this.warmcloudScheduleData && this.warmcloudScheduleData[normalizedServerName]) {
        const scheduleData = this.warmcloudScheduleData[normalizedServerName];
        if (scheduleData.nextSendTime) {
          try {
            nextTime = this.$utils.getDate(scheduleData.nextSendTime).format('HH:mm');
          } catch (e) {
            // Игнорируем ошибки парсинга
          }
        }
      }

      return nextTime || '--:--';
    },
    getWarmStateNextClasses() {
      return 'tag is-light is-info';
    },
    getWarmStateNextText(row) {
      const nextTime = this.getWarmStateNextTime(row);
      const lbl = this.$t('smtp.stats.nextAt') || 'След. в';
      return `${lbl}: ${nextTime}`;
    },
    togglePause(row) {
      const n = this.normalizedName(row);
      const fn = this.isPaused(row) ? this.$api.resumeWarmCloud : this.$api.pauseWarmCloud;
      fn(n).then(() => this.fetch());
    },
    forceTick(row) {
      const n = this.normalizedName(row);
      this.$api.forceWarmCloud(n).then((resp) => {
        const started = resp && resp.started;
        if (started) {
          this.$utils.toast((this.$t('smtp.stats.forceStarted') || 'Ручной тик запущен'));
        } else {
          this.$utils.toast((this.$t('smtp.stats.forceDeclined') || 'Тик уже идёт или лимит достигнут'), 'is-warning');
        }
        this.fetch();
      }).catch(() => {
        this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
      });
    },
    resetDaily(row) {
      const n = this.normalizedName(row);
      this.$api.resetWarmCloudDaily(n).then(() => {
        this.$utils.toast((this.$t('smtp.stats.resetDone') || 'Дневной прогресс сброшен'));
        this.fetch();
      });
    },
    finishWarmcloudToday(row) {
      const n = this.normalizedName(row);
      this.$api.finishWarmCloudToday(n).then(() => {
        this.$utils.toast((this.$t('warmcloud.finishTodayDone') || 'Отметили рассылку как завершённую на сегодня'));
        this.fetch();
      }).catch(() => {
        this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
      });
    },
    rebuildSchedule(row) {
      const n = this.normalizedName(row);
      this.$api.rebuildWarmCloud(n).then(() => {
        this.$utils.toast((this.$t('smtp.stats.rebuilt') || 'Расписание перестроено'));
        this.fetch();
      }).catch(() => {
        this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
      });
    },
    clearSchedule(row) {
      const n = this.normalizedName(row);
      this.$utils.confirm(
        this.$t('smtp.stats.clearScheduleConfirm') || 'Вы уверены, что хотите очистить всё расписание для этого сервера? Это удалит все записи (отправленные и запланированные).',
        () => {
          this.$api.clearWarmCloudSchedule(n).then(() => {
            this.$utils.toast((this.$t('smtp.stats.scheduleCleared') || 'Расписание полностью очищено'));
            this.fetch();
          }).catch(() => {
            this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
          });
        },
      );
    },
    openSettings(row) {
      this.current = { ...row };
      this.showModal = true;
    },
    openCheck(row) {
      this.current = { ...row };
      this.showCheck = true;
      this.check = null;
      this.checking = true;
      this.$api.checkSMTPServer(this.current.name).then((resp) => {
        this.check = resp;
      }).catch((err) => {
        // eslint-disable-next-line no-console
        console.error('Error checking SMTP server:', err);
        this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
      }).finally(() => { this.checking = false; });
    },
    save() {
      this.saving = true;
      this.$api.updateSMTPServer(this.current.name, {
        daily_limit: parseInt(this.current.dailyLimit, 10) || 0,
        send_pause: this.current.sendPause || '0s',
        warmcloud_enabled: this.current.warmcloudEnabled,
        warmcloud_big: this.current.warmcloudBig || '',
        warmcloud_working_hours_enabled: !!this.current.warmcloudWorkingHoursEnabled,
        warmcloud_working_hours: this.current.warmcloudWorkingHours || '',
        warmcloud_daily_limit: parseInt(this.current.warmcloudDailyLimit, 10) || 10,
        warmcloud_daily_step: parseInt(this.current.warmcloudDailyStep, 10) || 5,
        warmcloud_campaign_id: parseInt(this.current.warmcloudCampaignId, 10) || 0,
        warmcloud_read_percent: parseInt(this.current.warmcloudReadPercent, 10) || 0,
        warmcloud_rate_count: parseInt(this.current.warmcloudRateCount, 10) || 0,
        warmcloud_rate_window_minutes: parseInt(this.current.warmcloudRateWindowMinutes, 10) || 0,
        warmcloud_rate_jitter_pct: parseInt(this.current.warmcloudRateJitterPct, 10) || 0,
        warmcloud_max_per_tick: parseInt(this.current.warmcloudMaxPerTick, 10) || 0,
        warmcloud_sleep_between: this.current.warmcloudSleepBetween || '',
      }).then(() => {
        this.showModal = false;
        this.fetch();
        this.$utils.toast(this.$t('globals.messages.updated', { name: this.current.name }));
      }).catch((err) => {
        // eslint-disable-next-line no-console
        console.error('Error updating SMTP server:', err);
        this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
      }).finally(() => {
        this.saving = false;
      });
    },

    limitMeta(row) {
      if (!row) {
        return null;
      }
      const limit = Number(row.dailyLimit || row.daily_limit || 0);
      if (!Number.isFinite(limit) || limit <= 0) {
        return null;
      }
      const sent = Number(row.sentToday || row.sent_today || 0);
      const percentage = Math.max(0, Math.min(100, limit === 0 ? 0 : (sent / limit) * 100));
      return {
        limit,
        sent,
        percentage: Number(percentage.toFixed(1)),
        reached: sent >= limit,
      };
    },

    toggleWarmCloud(server) {
      this.$api.updateSMTPServer(server.name, {
        warmcloud_enabled: server.warmcloudEnabled,
      }).then(() => {
        this.$utils.toast(this.$t('globals.messages.updated', { name: server.name }));
      }).catch((err) => {
        // eslint-disable-next-line no-console
        console.error('Error toggling WarmCloud:', err);
        this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
        // Revert the toggle
        // eslint-disable-next-line no-param-reassign
        server.warmcloudEnabled = !server.warmcloudEnabled;
      });
    },

    smtpRowClass(row) {
      const meta = this.limitMeta(row);
      return meta && meta.reached ? 'is-limit-reached' : '';
    },
  },
  created() {
    this.fetch();
  },
};
</script>

<style scoped>
.smtp-limit {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.smtp-limit__row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: #374151;
}

.smtp-limit__status {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 999px;
  background: rgba(79, 70, 229, 0.1);
  color: #4338ca;
  font-size: 11px;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  font-weight: 600;
}

.smtp-limit__status.is-warning {
  background: rgba(239, 68, 68, 0.12);
  color: #b91c1c;
}

.smtp-limit__bar {
  position: relative;
  height: 6px;
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.3);
  overflow: hidden;
}

.smtp-limit__bar-fill {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, #4f46e5, #6366f1);
  transition: width 0.3s ease;
}

.smtp-limit__status.is-warning ~ .smtp-limit__bar .smtp-limit__bar-fill,
[data-limit-reached="true"] .smtp-limit__bar-fill {
  background: linear-gradient(90deg, #fb923c, #ef4444);
}
</style>
.is-limit-reached td {
  background-color: rgba(254, 226, 226, 0.5);
}

.is-limit-reached .smtp-limit__status {
  background: rgba(239, 68, 68, 0.18);
  color: #b91c1c;
}
