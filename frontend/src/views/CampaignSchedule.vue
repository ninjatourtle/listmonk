<template>
  <section class="campaign-schedule">
    <div class="columns page-header is-vcentered mb-4">
      <div class="column is-8">
        <h1 class="title is-4 mb-0">
          <b-icon icon="calendar" class="mr-2" />
          {{ $t('campaigns.schedule.title') || 'Расписание кампаний' }}
        </h1>
      </div>
      <div class="column is-4 has-text-right">
        <b-button size="is-small" type="is-primary" icon-left="refresh" @click="refresh">
          {{ $t('globals.buttons.refresh') || 'Обновить' }}
        </b-button>
      </div>
    </div>

    <b-field grouped class="mb-4">
      <b-input
        v-model.number="filters.campaignId"
        :placeholder="$t('globals.terms.campaign') + ' ID'"
        type="number"
        min="0"
      />
      <b-datepicker
        v-model="filters.day"
        :date-formatter="formatDate"
        :mobile-native="false"
      />
      <b-select v-model.number="filters.limit" placeholder="Показывать по" @change="onLimitChange">
        <option :value="25">25</option>
        <option :value="50">50</option>
        <option :value="100">100</option>
        <option :value="250">250</option>
        <option :value="500">500</option>
        <option :value="-1">{{ $t('warmcloud.schedule.all') || 'Все' }}</option>
      </b-select>
      <b-select v-model="filters.status" placeholder="{{ $t('globals.fields.status') || 'Статус' }}">
        <option value="">{{ $t('globals.terms.all') || 'Все' }}</option>
        <option value="pending">{{ $t('warmcloud.schedule.status.pending') || 'Ожидает' }}</option>
        <option value="done">{{ $t('warmcloud.schedule.status.done') || 'Отправлено' }}</option>
        <option value="error">{{ $t('warmcloud.schedule.status.error') || 'Ошибка' }}</option>
      </b-select>
      <b-button @click="prevDay" type="is-light">{{ $t('warmcloud.schedule.prevDay') || 'Вчера' }}</b-button>
      <b-button @click="setToday" type="is-light">{{ $t('warmcloud.schedule.today') || 'Сегодня' }}</b-button>
      <b-button @click="nextDay" type="is-light">{{ $t('warmcloud.schedule.nextDay') || 'Завтра' }}</b-button>
      <b-button @click="clearFilters" type="is-light" icon-left="close">
        {{ $t('globals.buttons.reset') || 'Сбросить' }}
      </b-button>
    </b-field>

    <div class="level mb-4">
      <div class="level-left">
        <div class="level-item">
          <div class="tags">
            <span class="tag is-info">{{ $t('warmcloud.schedule.total') || 'Всего' }}: {{ statusCounts.total }}</span>
            <span class="tag is-warning">{{ $t('warmcloud.schedule.status.pending') || 'Ожидает' }}: {{ statusCounts.pending }}</span>
            <span class="tag is-success">{{ $t('warmcloud.schedule.status.done') || 'Отправлено' }}: {{ statusCounts.done }}</span>
            <span class="tag is-danger">{{ $t('globals.fields.errors') || 'Ошибки' }}: {{ statusCounts.error }}</span>
          </div>
        </div>
      </div>
      <div class="level-right">
        <div class="level-item buttons">
          <b-button size="is-small" type="is-success" icon-left="download" @click="exportSchedule">
            {{ $t('globals.buttons.export') || 'Экспорт' }}
          </b-button>
          <b-button size="is-small" type="is-info" icon-left="layers" @click="showCampaignGroups">
            {{ $t('campaigns.schedule.groups') || 'Группы' }}
          </b-button>
        </div>
      </div>
    </div>

    <b-table :data="paginatedRows" :striped="true" :loading="loading">
      <b-table-column field="due_at" :label="$t('globals.fields.createdAt') || 'Дата'" v-slot="{ row }">
        {{ formatDateTime(row.due_at || row.dueAt) }}
      </b-table-column>
      <b-table-column field="campaign" :label="$t('globals.terms.campaign') || 'Кампания'" v-slot="{ row }">
        <div>
          <div class="has-text-weight-semibold">{{ getCampaignName(row) }}</div>
          <div class="is-size-7 has-text-grey">ID: {{ getCampaignId(row) }}</div>
          <div class="mt-1">
            <b-button size="is-small" icon-left="filter" @click="filterByCampaign(getCampaignId(row))">
              {{ $t('campaigns.schedule.filter') || 'Показать только эту' }}
            </b-button>
          </div>
        </div>
      </b-table-column>
      <b-table-column field="email_from" :label="$t('campaigns.fromAddress') || 'От кого'" v-slot="{ row }">
        <div v-if="getEmailFrom(row)">
          <div class="has-text-weight-medium">{{ getEmailFrom(row) }}</div>
        </div>
        <span v-else class="has-text-grey-light">—</span>
      </b-table-column>
      <b-table-column field="email_to" :label="$t('subscribers.email') || 'Кому'" v-slot="{ row }">
        <div v-if="getEmailTo(row)">{{ getEmailTo(row) }}</div>
        <span v-else class="has-text-grey-light">—</span>
      </b-table-column>
      <b-table-column field="subject" :label="$t('campaigns.subject') || 'Тема'" v-slot="{ row }">
        <span>{{ row.subject || '—' }}</span>
      </b-table-column>
      <b-table-column field="quota" :label="$t('sends.quota') || 'Квота'" v-slot="{ row }">
        <span v-if="getQuotaInfo(row)">{{ getQuotaInfo(row) }}</span>
        <span v-else class="has-text-grey-light">—</span>
      </b-table-column>
      <b-table-column field="status" :label="$t('globals.fields.status') || 'Статус'" v-slot="{ row }">
        <b-tag :type="statusType(row.status)">{{ statusLabel(row.status) }}</b-tag>
        <div class="is-size-7 has-text-grey">{{ getCampaignStatus(row) }}</div>
      </b-table-column>
      <b-table-column field="actions" :label="$t('globals.terms.actions') || 'Действия'" v-slot="{ row }">
        <template v-if="row.status === 'pending'">
          <b-dropdown position="is-bottom-left">
            <template #trigger>
              <b-button size="is-small">{{ $t('globals.terms.actions') }}</b-button>
            </template>
            <b-dropdown-item @click.native="promptReschedule(row)">{{ $t('globals.buttons.edit') || 'Изменить время' }}</b-dropdown-item>
            <b-dropdown-item @click.native="skipSlot(row)">{{ $t('globals.buttons.skip') || 'Пропустить' }}</b-dropdown-item>
            <b-dropdown-item @click.native="deleteSlot(row)">{{ $t('globals.buttons.delete') }}</b-dropdown-item>
          </b-dropdown>
        </template>
        <span v-else class="has-text-grey-light">—</span>
      </b-table-column>
      <b-table-column field="error" :label="$t('globals.fields.error') || 'Ошибка'" v-slot="{ row }">
        <span v-if="row.error" class="has-text-danger">{{ row.error }}</span>
        <span v-else class="has-text-grey-light">—</span>
      </b-table-column>
    </b-table>

    <div class="level mt-4" v-if="hasPagination">
      <div class="level-left">
        <div class="level-item is-size-7 has-text-grey">
          {{ $t('globals.pagination.summary') || 'Записей на странице' }}: {{ paginatedRows.length }} / {{ displayRows.length }}
        </div>
      </div>
      <div class="level-right">
        <div class="level-item">
          <b-pagination
            v-model="currentPage"
            :total="displayRows.length"
            :per-page="perPage"
            size="is-small"
            order="is-right"
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
        campaignId: this.$route.query.campaign_id ? parseInt(this.$route.query.campaign_id, 10) : 0,
        day: new Date(),
        limit: 100,
        status: '',
      },
      rows: [],
      loading: false,
      currentPage: 1,
    };
  },
  computed: {
    displayRows() {
      if (!this.filters.status) {
        return this.rows;
      }
      return this.rows.filter((row) => (row.status || '').toLowerCase() === this.filters.status.toLowerCase());
    },
    perPage() {
      if (this.filters.limit === -1) {
        return this.displayRows.length || 1;
      }
      if (!this.filters.limit || this.filters.limit <= 0) {
        return 100;
      }
      return parseInt(this.filters.limit, 10);
    },
    paginatedRows() {
      const start = (this.currentPage - 1) * this.perPage;
      const end = start + this.perPage;
      return this.displayRows.slice(start, end);
    },
    hasPagination() {
      return this.displayRows.length > this.perPage;
    },
    statusCounts() {
      const counters = {
        total: this.displayRows.length,
        pending: 0,
        done: 0,
        error: 0,
      };
      this.displayRows.forEach((row) => {
        const st = (row.status || 'pending').toLowerCase();
        if (st === 'error') counters.error += 1;
        else if (st === 'done') counters.done += 1;
        else counters.pending += 1;
      });
      return counters;
    },
  },
  watch: {
    'filters.day': function (val) {
      if (val) {
        this.currentPage = 1;
        this.refresh();
      }
    },
    'filters.status': function () {
      this.currentPage = 1;
      this.refresh();
    },
  },
  created() {
    this.refresh();
  },
  methods: {
    formatDate(date) {
      return dayjs(date).format('YYYY-MM-DD');
    },
    formatDateTime(value) {
      if (!value) return '—';
      return dayjs(value).format('YYYY-MM-DD HH:mm');
    },
    statusLabel(status) {
      const st = (status || 'pending').toLowerCase();
      if (st === 'done') return this.$t('warmcloud.schedule.status.done') || 'Отправлено';
      if (st === 'error') return this.$t('warmcloud.schedule.status.error') || 'Ошибка';
      return this.$t('warmcloud.schedule.status.pending') || 'Ожидает';
    },
    statusType(status) {
      const st = (status || 'pending').toLowerCase();
      if (st === 'done') return 'is-success';
      if (st === 'error') return 'is-danger';
      return 'is-warning';
    },
    getCampaignStatus(row) {
      return row.campaign_status || '';
    },
    getCampaignId(row) {
      return row.campaign_id || row.campaignId || row.campaign?.id || '—';
    },
    getCampaignName(row) {
      return row.campaign_name || row.campaignName || row.campaign?.name || `${this.$t('globals.terms.campaign') || 'Кампания'} #${this.getCampaignId(row)}`;
    },
    getEmailFrom(row) {
      return row.email_from || row.emailFrom || row.from_email || row.fromEmail || row.campaign?.from_email || null;
    },
    getEmailTo(row) {
      return row.email_to || row.emailTo || row.subscriber_email || row.subscriber?.email || null;
    },
    getQuotaInfo(row) {
      if (!row || !row.quota) return '';
      return `${row.used || 0}/${row.quota}`;
    },
    onLimitChange() {
      if (this.filters.limit === 0) {
        this.filters.limit = 100;
      }
      this.currentPage = 1;
      this.refresh();
    },
    async refresh() {
      this.loading = true;
      try {
        const params = {
          campaign_id: this.filters.campaignId || 0,
          date: dayjs(this.filters.day).format('YYYY-MM-DD'),
          limit: this.filters.limit,
          status: this.filters.status || undefined,
          offset: 0,
        };
        const resp = await this.$api.getCampaignSchedule(params);
        this.rows = Array.isArray(resp) ? resp : [];
        this.currentPage = 1;
      } catch (err) {
        this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
      } finally {
        this.loading = false;
      }
    },
    prevDay() {
      this.filters.day = dayjs(this.filters.day).subtract(1, 'day').toDate();
    },
    setToday() {
      this.filters.day = new Date();
    },
    nextDay() {
      this.filters.day = dayjs(this.filters.day).add(1, 'day').toDate();
    },
    promptReschedule(row) {
      const current = this.formatDateTime(row.due_at || row.dueAt);
      this.$utils.prompt(
        this.$t('warmcloud.schedule.reschedulePrompt') || 'Новое время (YYYY-MM-DD HH:mm):',
        { value: current, placeholder: 'YYYY-MM-DD HH:mm' },
        (value) => {
          const v = String(value || '').trim();
          if (!v) return;
          this.$api.rescheduleCampaignSlot(row.id, v).then(() => this.refresh());
        },
      );
    },
    deleteSlot(row) {
      this.$utils.confirm(
        this.$t('globals.messages.confirmDelete') || 'Удалить запись?',
        () => this.$api.deleteCampaignSlot(row.id).then(() => this.refresh()),
      );
    },
    skipSlot(row) {
      this.$api.skipCampaignSlot(row.id).then(() => this.refresh());
    },
    clearFilters() {
      this.filters.status = '';
      this.filters.campaignId = 0;
      this.currentPage = 1;
      this.refresh();
    },
    filterByCampaign(campaignId) {
      this.filters.campaignId = parseInt(campaignId, 10) || 0;
      this.currentPage = 1;
      this.refresh();
    },
    exportSchedule() {
      if (!this.displayRows.length) {
        this.$utils.toast(this.$t('globals.messages.noData') || 'Нет данных для экспорта', 'is-info');
        return;
      }
      const data = this.displayRows.map((row) => ({
        Date: this.formatDateTime(row.due_at || row.dueAt),
        CampaignID: this.getCampaignId(row),
        Campaign: this.getCampaignName(row),
        From: this.getEmailFrom(row) || '',
        To: this.getEmailTo(row) || '',
        Subject: row.subject || '',
        Quota: this.getQuotaInfo(row) || '',
        Status: this.statusLabel(row.status),
        Error: row.error || '',
      }));
      const filename = `campaign_schedule_${dayjs(this.filters.day).format('YYYYMMDD')}.csv`;
      this.$utils.downloadCSV(data, filename);
      this.$utils.toast(this.$t('globals.messages.exported') || 'Экспорт выполнен', 'is-success');
    },
    showCampaignGroups() {
      if (!this.displayRows.length) {
        this.$utils.toast(this.$t('globals.messages.noData') || 'Нет данных', 'is-info');
        return;
      }
      const groups = {};
      this.displayRows.forEach((row) => {
        const id = this.getCampaignId(row);
        const name = this.getCampaignName(row);
        if (!groups[id]) {
          groups[id] = {
            name,
            id,
            total: 0,
            pending: 0,
            done: 0,
            error: 0,
          };
        }
        const bucket = groups[id];
        bucket.total += 1;
        const st = (row.status || 'pending').toLowerCase();
        if (st === 'error') bucket.error += 1;
        else if (st === 'done') bucket.done += 1;
        else bucket.pending += 1;
      });
      let html = '<div class="content">';
      Object.values(groups).forEach((group) => {
        html += `
          <div class="box mb-3">
            <div class="columns">
              <div class="column is-8">
                <h4 class="subtitle is-6 has-text-weight-semibold">${group.name}</h4>
                <p class="is-size-7 has-text-grey">ID: ${group.id}</p>
              </div>
              <div class="column is-4">
                <div class="tags has-addons">
                  <span class="tag is-info">${group.total}</span>
                  <span class="tag is-warning">${group.pending}</span>
                  <span class="tag is-success">${group.done}</span>
                  <span class="tag is-danger">${group.error}</span>
                </div>
              </div>
            </div>
          </div>`;
      });
      html += '</div>';
      this.$buefy.dialog.alert({
        title: this.$t('campaigns.schedule.groups') || 'Группы кампаний',
        message: html,
        confirmText: this.$t('globals.buttons.close') || 'Закрыть',
        type: 'is-info',
        hasIcon: true,
        icon: 'layers',
        iconPack: 'fas',
        ariaModal: true,
        trapFocus: true,
      });
    },
  },
};
</script>

<style scoped>
.campaign-schedule .page-header {
  border-bottom: 1px solid #f0f1f5;
}
</style>
