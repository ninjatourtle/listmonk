<template>
  <section class="warmcloud-schedule">
    <header class="columns page-header">
      <div class="column is-8">
        <h1 class="title is-4">
          <b-icon icon="calendar" class="mr-2" />
          {{ $t('warmcloud.schedule.title') || 'Расписание прогрева' }}
        </h1>
      </div>
    </header>

    <b-field grouped>
      <b-input
        id="server-filter"
        name="server-filter"
        v-model="filters.server"
        :placeholder="$t('globals.fields.server') || 'Сервер'"
      />
      <b-datepicker
        id="date-filter"
        name="date-filter"
        v-model="filters.day"
        :date-formatter="formatDate"
        :mobile-native="false"
      />
      <b-select
        id="limit-select"
        name="limit-select"
        v-model="filters.limit"
        :placeholder="$t('sends.limit') || 'Записей на странице'"
      >
        <option value="10">10</option>
        <option value="50">50</option>
        <option value="100">100</option>
        <option value="500">500</option>
        <option value="1000">1000</option>
        <option value="-1">Все</option>
      </b-select>
      <b-select
        id="status-filter"
        name="status-filter"
        v-model="filters.status"
        placeholder="Фильтр по статусу"
      >
        <option value="">Все статусы</option>
        <option value="pending">Ожидает</option>
        <option value="done">Отправлено</option>
        <option value="read">Прочитано</option>
        <option value="error">Ошибка</option>
      </b-select>
      <b-button @click="refresh" type="is-primary">{{ $t('globals.buttons.refresh') }}</b-button>
      <b-button @click="prevDay" type="is-light">{{ $t('warmcloud.schedule.prevDay') || 'Вчера' }}</b-button>
      <b-button @click="setToday" type="is-light">{{ $t('warmcloud.schedule.today') || 'Сегодня' }}</b-button>
      <b-button @click="nextDay" type="is-light">{{ $t('warmcloud.schedule.nextDay') || 'Завтра' }}</b-button>
    </b-field>

    <!-- Дополнительные фильтры и статистика -->
    <div class="level">
      <div class="level-left">
        <div class="level-item">
          <b-field grouped>
            <b-button @click="clearFilters" type="is-light" icon-left="close">Очистить фильтры</b-button>
            <b-button @click="showServerStats" type="is-info" icon-left="chart-bar">Статистика серверов</b-button>
          </b-field>
        </div>
      </div>
      <div class="level-right">
        <div class="level-item">
          <div class="tags">
            <span class="tag is-info">Всего: {{ rows.length }}</span>
            <span class="tag is-warning">Ожидает: {{ getStatusCount('pending') }}</span>
            <span class="tag is-success">Отправлено: {{ getStatusCount('done') + getStatusCount('read') + getStatusCount('opened') }}</span>
            <span class="tag is-info">Прочитано: {{ getStatusCount('read') + getStatusCount('opened') }}</span>
            <span class="tag is-danger">Ошибки: {{ getStatusCount('error') }}</span>
          </div>
        </div>
      </div>
    </div>

    <b-table :data="paginatedRows" :striped="true">
      <b-table-column v-slot="p" field="dueAt" :label="$t('globals.fields.createdAt') || 'Дата'">
        <span>{{ p.row.dueAt ? ($utils && $utils.niceDate ? $utils.niceDate(p.row.dueAt, true) : formatDateTime(p.row.dueAt)) : '—' }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="server" :label="$t('globals.fields.server') || 'Сервер'">
        <div>
          <div class="has-text-weight-bold">{{ p.row.server || '—' }}</div>
          <div class="is-size-7 has-text-grey" v-if="getServerStatus(p.row)">
            {{ getServerStatus(p.row) }}
          </div>
        </div>
      </b-table-column>
      <b-table-column v-slot="p" field="email_from" :label="$t('campaigns.fromAddress') || 'Откуда'">
        <span>{{ getSenderEmail(p.row) }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="email" :label="$t('subscribers.email')">
        <span>{{ p.row.email || '—' }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="subject" :label="$t('campaigns.subject')">
        <span>{{ (p.row.status === 'done' || p.row.status === 'read') ? (p.row.subject || '—') : '—' }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="status" :label="$t('globals.fields.status') || 'Статус'">
        <div>
          <b-tag :type="statusType(p.row.status || 'pending')" @click="p.row.status === 'error' ? showErrorDetails(p.row) : null" :class="{ 'is-clickable': p.row.status === 'error' }">
            {{ statusLabel(p.row.status || 'pending') }}
          </b-tag>
          <div class="mt-1" v-if="getNextSendTime(p.row) && p.row.status === 'pending'">
            <span class="tag is-light is-info is-small">
              {{ $t('warmcloud.schedule.nextSend') || 'След. в' }}: {{ getNextSendTime(p.row) }}
            </span>
          </div>
          <div class="mt-1" v-if="getQuotaInfo(p.row)">
            <span class="tag is-light is-warning is-small">
              {{ getQuotaInfo(p.row) }}
            </span>
          </div>
        </div>
      </b-table-column>
      <b-table-column v-slot="p" field="actions" :label="$t('globals.terms.actions')">
        <template v-if="p.row.status === 'pending'">
          <b-dropdown position="is-bottom-left">
            <template #trigger>
              <b-button size="is-small">{{ $t('globals.terms.actions') }}</b-button>
            </template>
            <b-dropdown-item @click.native="promptReschedule(p.row)">{{ $t('globals.buttons.edit') || 'Изменить время' }}</b-dropdown-item>
            <b-dropdown-item @click.native="skipSlot(p.row)">{{ $t('globals.buttons.skip') || 'Пропустить' }}</b-dropdown-item>
            <b-dropdown-item @click.native="deleteSlot(p.row)">{{ $t('globals.buttons.delete') }}</b-dropdown-item>
          </b-dropdown>
        </template>
        <span v-else class="has-text-grey-light">—</span>
      </b-table-column>
    </b-table>

    <!-- Пагинация -->
    <div class="level mt-4">
      <div class="level-left">
        <div class="level-item">
          <p class="is-size-7 has-text-grey">
            Показано {{ paginatedRows.length }} из {{ displayRows.length }} записей (всего на дату: {{ rows.length }})
          </p>
        </div>
      </div>
      <div class="level-right">
        <div class="level-item">
          <b-pagination
            v-model="currentPage"
            :total="displayRows.length"
            :per-page="perPage"
            :range-before="2"
            :range-after="2"
            order="is-centered"
            size="is-small"
            :simple="false"
            :rounded="true"
            :has-next="true"
            :has-previous="true"
            icon-prev="chevron-left"
            icon-next="chevron-right"
          />
        </div>
      </div>
    </div>
  </section>
</template>

<script>
import dayjs from 'dayjs';

export default {
  data() {
    return {
      filters: {
        server: (this.$route && this.$route.query && this.$route.query.server) || '',
        day: new Date(),
        limit: -1, // По умолчанию показываем все записи
        status: '',
      },
      rows: [],
      currentPage: 1,
    };
  },
  computed: {
    displayRows() {
      if (!this.filters.status) {
        return this.rows;
      }
      return this.rows.filter((row) => row.status === this.filters.status);
    },
    perPage() {
      return this.filters.limit === -1 ? this.displayRows.length : parseInt(this.filters.limit, 10);
    },
    paginatedRows() {
      const start = (this.currentPage - 1) * this.perPage;
      const end = start + this.perPage;
      return this.displayRows.slice(start, end);
    },
  },
  methods: {
    formatDate(d) {
      if (!d) return '';
      try {
        return dayjs(d).format('YYYY-MM-DD');
      } catch (e) {
        // eslint-disable-next-line no-console
        console.error('Error formatting date:', e, d);
        return '';
      }
    },
    formatDateTime(s) {
      if (!s) return '';
      try {
        return dayjs(s).format('YYYY-MM-DD HH:mm');
      } catch (e) {
        // eslint-disable-next-line no-console
        console.error('Error formatting datetime:', e, s);
        return '';
      }
    },
    statusType(st) {
      if (st === 'done') return 'is-success';
      if (st === 'error') return 'is-danger';
      if (st === 'read' || st === 'opened') return 'is-info';
      return 'is-warning'; // pending
    },
    statusLabel(st) {
      if (st === 'done') return this.$t('warmcloud.schedule.status.done') || 'Отправлено';
      if (st === 'error') return this.$t('warmcloud.schedule.status.error') || 'Ошибка';
      if (st === 'read' || st === 'opened') return this.$t('warmcloud.schedule.status.read') || 'Прочитано';
      return this.$t('warmcloud.schedule.status.pending') || 'Ожидает';
    },
    getNextSendTime(row) {
      if (!row) return null;

      // Пытаемся получить время следующей отправки из разных возможных полей
      const timeFields = [
        row.nextAt,
        row.next_send_time,
        row.nextSendTime,
        row.nextSend,
        row.scheduled_time,
        row.scheduledTime,
        row.dueAt, // Время запланированной отправки
        row.scheduled_at,
        row.scheduledAt,
      ];

      let foundTime = null;
      timeFields.some((timeField) => {
        if (timeField) {
          try {
            if (this.$utils && this.$utils.getDate) {
              foundTime = this.$utils.getDate(timeField).format('HH:mm');
              return true; // Останавливаем поиск
            }
            // Fallback to dayjs if $utils is not available
            foundTime = dayjs(timeField).format('HH:mm');
            return true;
          } catch (e) {
            // eslint-disable-next-line no-console
            console.error('Error formatting time field:', e, timeField);
            // Продолжаем поиск в следующих полях
            return false;
          }
        }
        return false;
      });

      return foundTime;
    },
    getQuotaInfo(row) {
      // Получаем информацию о квоте
      const quota = row.quota || row.quota_limit || row.daily_limit;
      const used = row.quota_used || row.sent_today || row.sent_count;

      if (quota && used !== undefined) {
        const remaining = quota - used;
        const percentage = Math.round((used / quota) * 100);

        if (remaining <= 0) {
          return `Лимит исчерпан (${used}/${quota})`;
        }
        if (percentage >= 80) {
          return `Лимит почти исчерпан (${used}/${quota}, ${percentage}%)`;
        }
        return `Квота: ${used}/${quota} (${percentage}%)`;
      }

      // Если есть только лимит, показываем его
      if (quota) {
        return `Лимит: ${quota}/день`;
      }

      return null;
    },
    getServerStatus(row) {
      if (!row) return null;

      // Получаем дополнительную информацию о сервере (без статуса)
      const {
        last_sent: lastSentRaw,
        lastSent,
        total_sent: totalSentRaw,
        sent_count: sentCount,
      } = row;
      const lastSentTime = lastSentRaw || lastSent;
      const totalSent = totalSentRaw || sentCount;

      let statusText = '';

      if (lastSentTime) {
        try {
          let time;
          if (this.$utils && this.$utils.getDate) {
            time = this.$utils.getDate(lastSentTime).format('HH:mm');
          } else {
            time = dayjs(lastSentTime).format('HH:mm');
          }
          statusText += `Последняя отправка: ${time}`;
        } catch (e) {
          // eslint-disable-next-line no-console
          console.error('Error formatting last sent time:', e, lastSentTime);
          // игнорируем ошибки парсинга даты
        }
      }

      if (totalSent !== undefined) {
        statusText += statusText ? ` | Всего отправлено: ${totalSent}` : `Всего отправлено: ${totalSent}`;
      }

      return statusText || null;
    },
    getReadCount(row) {
      if (!row) return 0;

      // Получаем количество прочитанных писем из разных возможных полей
      const readCount = row.read_count || row.readCount || row.reads || row.opened_count || row.openedCount;

      if (readCount !== undefined && readCount !== null) {
        return readCount;
      }

      // Если есть информация о том, что письмо прочитано
      if (row.submitted || row.opened || row.read) {
        return 1;
      }

      return 0;
    },
    getSenderEmail(row) {
      if (!row) return '—';

      const senderEmail = row.email_from || row.emailFrom || row.from_email || row.fromEmail || row.sender_email || row.senderEmail || row.from || row.fromAddress || row.from_address;
      if (senderEmail) {
        return senderEmail;
      }

      if (row.campaign && row.campaign.from_email) {
        return row.campaign.from_email;
      }

      if (row.campaign_template && row.campaign_template.from_email) {
        return row.campaign_template.from_email;
      }

      return '—';
    },
    showErrorDetails(row) {
      if (!row || !row.error) return;

      let errorMessage = row.error;
      if (row.error_details) {
        errorMessage += `\n\nДетали: ${row.error_details}`;
      }

      this.$buefy.dialog.alert({
        title: 'Детали ошибки',
        message: errorMessage,
        confirmText: 'Закрыть',
        type: 'is-danger',
        hasIcon: true,
        icon: 'alert-circle',
        ariaModal: true,
        trapFocus: true,
      });
    },
    async refresh() {
      try {
        const params = {
          server: this.filters.server,
          date: this.filters.day ? dayjs(this.filters.day).format('YYYY-MM-DD') : dayjs().format('YYYY-MM-DD'),
          limit: this.filters.limit === -1 ? 999999 : (this.filters.limit || 100),
        };
        const resp = await this.$api.getWarmCloudSchedule(params);
        this.rows = resp || [];
        // Сбрасываем страницу при обновлении данных
        this.currentPage = 1;
      } catch (e) {
        // eslint-disable-next-line no-console
        console.error('Error loading warmcloud schedule:', e);
        let errorMessage = this.$t('globals.messages.internalError') || 'Внутренняя ошибка сервера';
        if (e.response && e.response.data && e.response.data.message) {
          errorMessage = e.response.data.message;
        } else if (e.message) {
          errorMessage = e.message;
        }
        this.$utils.toast(errorMessage, 'is-danger');
      }
    },
    prevDay() {
      if (this.filters.day) {
        this.filters.day = dayjs(this.filters.day).subtract(1, 'day').toDate();
      } else {
        this.filters.day = dayjs().subtract(1, 'day').toDate();
      }
      this.refresh();
    },
    setToday() {
      this.filters.day = new Date();
      this.refresh();
    },
    nextDay() {
      if (this.filters.day) {
        this.filters.day = dayjs(this.filters.day).add(1, 'day').toDate();
      } else {
        this.filters.day = dayjs().add(1, 'day').toDate();
      }
      this.refresh();
    },
    promptReschedule(row) {
      if (!row || !row.dueAt) return;
      const cur = dayjs(row.dueAt).format('YYYY-MM-DD HH:mm');
      this.$utils.prompt(
        this.$t('warmcloud.schedule.reschedulePrompt') || 'Новое время (YYYY-MM-DD HH:mm):',
        { value: cur, placeholder: 'YYYY-MM-DD HH:mm' },
        (val) => {
          const v = String(val || '').trim();
          if (!v) return;
          this.$api.rescheduleWarmCloudSlot(row.id, v).then(() => this.refresh());
        },
      );
    },
    deleteSlot(row) {
      this.$utils.confirm(this.$t('globals.messages.confirmDelete'), () => {
        this.$api.deleteWarmCloudSlot(row.id).then(() => this.refresh());
      });
    },
    skipSlot(row) {
      this.$api.skipWarmCloudSlot(row.id).then(() => this.refresh());
    },
    getStatusCount(status) {
      return this.rows.filter((row) => row.status === status).length;
    },
    getTotalReadCount() {
      // Подсчитываем общее количество прочитанных писем по всем записям
      return this.rows.reduce((total, row) => total + this.getReadCount(row), 0);
    },
    clearFilters() {
      this.filters.status = '';
      this.filters.server = '';
      this.refresh();
    },
    showServerStats() {
      const hasServerFilter = (this.filters.server || '').toString().trim() !== '';

      // Если выбран конкретный сервер — показываем статистику по отправителям этого сервера.
      if (hasServerFilter) {
        const rowsForServer = this.rows.filter((r) => r.server === this.filters.server);

        if (rowsForServer.length === 0) {
          this.$utils.toast('Нет данных для выбранного сервера', 'is-info');
          return;
        }

        const senderGroups = {};
        rowsForServer.forEach((row) => {
          const sender = this.getSenderEmail(row) || '—';
          if (!senderGroups[sender]) {
            senderGroups[sender] = {
              sender,
              total: 0,
              pending: 0,
              done: 0,
              error: 0,
            };
          }
          senderGroups[sender].total += 1;
          const status = row.status || 'pending';
          senderGroups[sender][status] = (senderGroups[sender][status] || 0) + 1;
        });

        const senderData = Object.values(senderGroups);

        let statsHtml = '<div class="content">';
        senderData.forEach((s) => {
          statsHtml += `
            <div class="box mb-3">
              <div class="columns">
                <div class="column is-6">
                  <h4 class="subtitle is-6 has-text-weight-bold">${s.sender}</h4>
                  <p class="is-size-7 has-text-grey">Всего записей: ${s.total}</p>
                </div>
                <div class="column is-6">
                  <div class="tags">
                    <span class="tag is-warning">Ожидает: ${s.pending}</span>
                    <span class="tag is-success">Отправлено: ${s.done}</span>
                    <span class="tag is-danger">Ошибки: ${s.error}</span>
                  </div>
                </div>
              </div>
            </div>
          `;
        });
        statsHtml += '</div>';

        this.$buefy.dialog.alert({
          title: `Статистика отправителей прогрева (${this.filters.server})`,
          message: statsHtml,
          confirmText: 'Закрыть',
          type: 'is-info',
          hasIcon: true,
          icon: 'chart-bar',
          iconPack: 'fas',
          ariaModal: true,
          trapFocus: true,
        });

        return;
      }

      // Иначе — как раньше: агрегирование по серверам (общая сводка)
      const serverGroups = {};
      this.rows.forEach((row) => {
        const { server } = row;
        if (!serverGroups[server]) {
          serverGroups[server] = {
            server,
            total: 0,
            pending: 0,
            done: 0,
            error: 0,
            quota_used: 0,
            quota_limit: 0,
          };
        }
        serverGroups[server].total += 1;
        serverGroups[server][row.status] = (serverGroups[server][row.status] || 0) + 1;

        if (row.quota_used) {
          serverGroups[server].quota_used += row.quota_used;
        }
        if (row.quota_limit) {
          serverGroups[server].quota_limit = row.quota_limit;
        }
      });

      const serverData = Object.values(serverGroups);

      if (serverData.length === 0) {
        this.$utils.toast('Нет данных для статистики', 'is-info');
        return;
      }

      let statsHtml = '<div class="content">';
      serverData.forEach((server) => {
        const quotaPercent = server.quota_limit > 0 ? Math.round((server.quota_used / server.quota_limit) * 100) : 0;
        statsHtml += `
          <div class="box mb-3">
            <div class="columns">
              <div class="column is-6">
                <h4 class="subtitle is-6 has-text-weight-bold">${server.server}</h4>
                <p class="is-size-7 has-text-grey">Всего записей: ${server.total}</p>
              </div>
              <div class="column is-3">
                <div class="tags">
                  <span class="tag is-warning">Ожидает: ${server.pending}</span>
                  <span class="tag is-success">Отправлено: ${server.done}</span>
                  <span class="tag is-danger">Ошибки: ${server.error}</span>
                </div>
              </div>
              <div class="column is-3">
                ${server.quota_limit > 0 ? `
                  <div class="tags">
                    <span class="tag is-info">Квота: ${server.quota_used}/${server.quota_limit}</span>
                    <span class="tag ${quotaPercent >= 80 ? 'is-warning' : 'is-light'}">${quotaPercent}%</span>
                  </div>
                ` : ''}
              </div>
            </div>
          </div>
        `;
      });
      statsHtml += '</div>';

      this.$buefy.dialog.alert({
        title: 'Статистика серверов прогрева',
        message: statsHtml,
        confirmText: 'Закрыть',
        type: 'is-info',
        hasIcon: true,
        icon: 'chart-bar',
        iconPack: 'fas',
        ariaModal: true,
        trapFocus: true,
      });
    },
  },
  created() { this.refresh(); },
};
</script>

<style scoped>
.is-clickable {
  cursor: pointer;
}

.is-clickable:hover {
  opacity: 0.8;
}
</style>
