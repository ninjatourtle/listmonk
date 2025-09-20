<template>
  <section class="dashboard-page">
    <div class="dashboard-page__hero">
      <div>
        <h1 class="dashboard-page__title">{{ $t('dashboard.hero.title') }}</h1>
        <p class="dashboard-page__subtitle">{{ $t('dashboard.hero.subtitle') }}</p>
      </div>
      <div class="dashboard-page__hero-actions">
        <b-dropdown v-model="isRangeOpen" position="is-bottom-left" append-to-body trap-focus>
          <template #trigger>
            <button class="dashboard-range" type="button">
              <b-icon icon="calendar" size="is-small" />
              <span>{{ dateRangeLabel }}</span>
              <b-icon icon="chevron-down" size="is-small" />
            </button>
          </template>
          <b-dropdown-item custom class="dashboard-range__dropdown">
            <div class="dashboard-range__content">
              <div class="dashboard-range__picker">
                <span class="dashboard-range__label">{{ $t('dashboard.range.start') }}</span>
                <b-datepicker v-model="filters.startDate" :max-date="filters.endDate" size="is-small" :trap-focus="false" />
              </div>
              <div class="dashboard-range__picker">
                <span class="dashboard-range__label">{{ $t('dashboard.range.end') }}</span>
                <b-datepicker v-model="filters.endDate" :min-date="filters.startDate" size="is-small" :trap-focus="false" />
              </div>
            </div>
            <div class="dashboard-range__footer">
              <b-button size="is-small" type="is-primary" @click="applyRange">{{ $t('globals.buttons.apply') }}</b-button>
            </div>
          </b-dropdown-item>
        </b-dropdown>
        <b-button class="dashboard-create" type="is-primary" rounded icon-left="plus"
          tag="router-link" :to="{ name: 'campaign', params: { id: 'new' } }">
          {{ $t('dashboard.actions.createCampaign') }}
        </b-button>
      </div>
    </div>

    <div class="dashboard-page__kpi-grid">
      <template v-if="isKpiLoading">
        <div v-for="n in 4" :key="`kpi-skeleton-${n}`" class="dashboard-card">
          <b-skeleton height="72px" :animated="true" />
        </div>
      </template>
      <template v-else>
        <article v-for="card in kpiCards" :key="card.key" class="dashboard-card" :data-accent="card.accent">
          <div class="dashboard-card__icon" :data-accent="card.accent" aria-hidden="true">
            <b-icon :icon="card.icon" size="is-medium" custom-class="dashboard-card__icon-svg" />
          </div>
          <div class="dashboard-card__body">
            <p class="dashboard-card__label">{{ card.label }}</p>
            <p class="dashboard-card__value">{{ card.displayValue }}</p>
            <p class="dashboard-card__delta" :class="`is-${card.change.direction}`">
              <span v-if="card.change.icon" class="dashboard-card__delta-icon">
                <b-icon :icon="card.change.icon" size="is-small" />
              </span>
              <span>{{ card.change.text }}</span>
            </p>
          </div>
        </article>
      </template>
    </div>

    <section class="dashboard-section">
      <header class="dashboard-section__header">
        <h2>{{ $t('dashboard.activity.title') }}</h2>
      </header>
      <div class="dashboard-card dashboard-section__chart">
        <b-loading v-if="isChartsLoading" :is-full-page="false" active />
        <chart v-else class="dashboard-line-chart" type="line" :data="multiChart" :options="multiChartOptions" />
      </div>
    </section>

    <section class="dashboard-section">
      <header class="dashboard-section__header">
        <h2>{{ $t('dashboard.smtp.title') }}</h2>
      </header>
      <div class="dashboard-card dashboard-card--list">
        <b-loading v-if="isSmtpLoading" :is-full-page="false" active />
        <ul v-else class="dashboard-smtp">
          <li
            v-for="row in smtp"
            :key="row.name"
            class="dashboard-smtp__item"
            :data-limit-reached="Boolean(limitMeta(row) && limitMeta(row).reached)"
          >
            <div class="dashboard-smtp__main">
              <h3 class="dashboard-smtp__name">{{ row.name || '—' }}</h3>
              <p class="dashboard-smtp__meta">
                {{ $t('dashboard.smtp.sentToday') }}: {{ $utils.niceNumber(row.sentToday || 0) }} •
                {{ $t('dashboard.smtp.sentTotal') }}: {{ $utils.niceNumber(row.sentTotal || 0) }}
              </p>
              <p
                v-if="limitMeta(row)"
                class="dashboard-smtp__meta dashboard-smtp__meta--limit"
              >
                <span>
                  {{ $t('dashboard.smtp.dailyLimit') }}:
                  <strong>{{ $utils.niceNumber(limitMeta(row).sent) }}/{{ $utils.niceNumber(limitMeta(row).limit) }}</strong>
                </span>
                <span
                  class="dashboard-smtp__limit-status"
                  :class="{ 'is-warning': limitMeta(row).reached }"
                >
                  <b-icon
                    v-if="limitMeta(row).reached"
                    icon="alert-circle-outline"
                    size="is-small"
                  />
                  {{ limitMeta(row).reached ? $t('dashboard.smtp.limitReached') : $t('dashboard.smtp.limitAvailable') }}
                </span>
              </p>
              <div
                v-if="limitMeta(row)"
                class="dashboard-smtp__limit-bar"
                role="progressbar"
                :aria-valuenow="limitMeta(row).sent"
                :aria-valuemin="0"
                :aria-valuemax="limitMeta(row).limit"
              >
                <div
                  class="dashboard-smtp__limit-bar-fill"
                  :style="{ width: `${limitMeta(row).percentage}%` }"
                />
              </div>
              <p class="dashboard-smtp__meta">
                {{ $t('dashboard.smtp.warmcloud') }}:
                <span v-if="row.warmcloudEnabled">
                  {{ (row.warmcloudSentToday || 0) }}/{{ row.warmcloudCurrentLimit || row.warmcloudDailyLimit || 0 }}
                </span>
                <span v-else>—</span>
              </p>
            </div>
            <div class="dashboard-smtp__status">
              <span class="dashboard-smtp__dot" :class="{ 'is-online': smtpChecks[row.name] }" />
              <span class="dashboard-smtp__status-text">
                {{ smtpChecks[row.name] ? $t('dashboard.smtp.status.online') : $t('dashboard.smtp.status.offline') }}
              </span>
            </div>
          </li>
        </ul>
      </div>
    </section>

    <section class="dashboard-section">
      <header class="dashboard-section__header">
        <h2>{{ $t('dashboard.latestCampaigns.title') }}</h2>
        <router-link class="dashboard-section__link" :to="{ name: 'campaigns' }">
          {{ $t('dashboard.latestCampaigns.viewAll') }}
        </router-link>
      </header>
      <div class="dashboard-card" v-if="isRecentLoading">
        <b-skeleton height="64px" :animated="true" :count="3" />
      </div>
      <div class="dashboard-card dashboard-empty" v-else-if="recentCampaigns.length === 0">
        <p>{{ $t('dashboard.latestCampaigns.empty') }}</p>
      </div>
      <div v-else class="dashboard-card dashboard-card--list">
        <ul class="dashboard-campaigns">
          <li v-for="campaign in recentCampaigns" :key="campaign.id" class="dashboard-campaigns__item">
            <div class="dashboard-campaigns__main">
              <router-link class="dashboard-campaigns__name"
                :to="{ name: 'campaign', params: { id: campaign.id } }">
                {{ campaign.name || $t('dashboard.latestCampaigns.untitled') }}
              </router-link>
              <p class="dashboard-campaigns__meta">{{ formatCampaignMeta(campaign) }}</p>
            </div>
            <div class="dashboard-campaigns__status">
              <b-tag :type="`is-${statusColor(campaign.status)}`" class="dashboard-campaigns__status-tag">
                {{ $t(`campaigns.status.${campaign.status}`) }}
              </b-tag>
              <router-link class="dashboard-campaigns__menu" :to="{ name: 'campaign', params: { id: campaign.id } }"
                :aria-label="$t('dashboard.latestCampaigns.openCampaign')">
                <b-icon icon="dots-horizontal" />
              </router-link>
            </div>
          </li>
        </ul>
      </div>
    </section>
  </section>
</template>

<script>
import dayjs from 'dayjs';
import 'dayjs/locale/ru';
import Vue from 'vue';
import Chart from '../components/Chart.vue';

const KPI_META = [
  {
    key: 'sent',
    labelKey: 'dashboard.metrics.sent',
    icon: 'email-outline',
    accent: 'violet',
  },
  {
    key: 'open',
    labelKey: 'dashboard.metrics.opened',
    icon: 'email-open-outline',
    accent: 'indigo',
  },
  {
    key: 'reply',
    labelKey: 'dashboard.metrics.replied',
    icon: 'message-processing-outline',
    accent: 'emerald',
  },
  {
    key: 'bounced',
    labelKey: 'dashboard.metrics.bounced',
    icon: 'alert-circle-outline',
    accent: 'rose',
  },
];

const CHART_COLORS = {
  sent: '#4F46E5',
  open: '#6366F1',
  reply: '#22C55E',
  bounced: '#F97316',
};

export default Vue.extend({
  name: 'DashboardView',

  components: {
    Chart,
  },

  data() {
    return {
      filters: {
        startDate: null,
        endDate: null,
      },
      isRangeOpen: false,
      isKpiLoading: true,
      isChartsLoading: true,
      isRecentLoading: true,
      isSmtpLoading: true,
      kpiTotals: {
        sent: 0,
        open: 0,
        reply: 0,
        bounced: 0,
      },
      kpiChanges: {
        sent: null,
        open: null,
        reply: null,
        bounced: null,
      },
      multiChart: null,
      multiChartOptions: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: {
            display: true,
            position: 'bottom',
            labels: {
              usePointStyle: true,
              padding: 20,
              boxWidth: 10,
            },
          },
          tooltip: {
            mode: 'index',
            intersect: false,
            backgroundColor: '#ffffff',
            borderColor: '#e2e8f0',
            borderWidth: 1,
            titleColor: '#0f172a',
            bodyColor: '#1f2937',
            bodySpacing: 6,
            padding: 10,
            callbacks: {
              label: (ctx) => {
                const label = ctx.dataset && ctx.dataset.label ? ctx.dataset.label : '';
                const value = (ctx.parsed && typeof ctx.parsed.y !== 'undefined') ? ctx.parsed.y : ctx.raw;
                return `${label}: ${Number(value || 0).toLocaleString()}`;
              },
            },
          },
        },
        elements: {
          line: {
            tension: 0.4,
            borderWidth: 2,
          },
          point: {
            radius: 0,
            hoverRadius: 4,
          },
        },
        interaction: { intersect: false, axis: 'x' },
        scales: {
          x: {
            grid: {
              display: false,
            },
            ticks: {
              color: '#64748b',
            },
          },
          y: {
            grid: {
              color: 'rgba(148, 163, 184, 0.15)',
            },
            ticks: {
              precision: 0,
              color: '#64748b',
            },
          },
        },
      },
      recentCampaigns: [],
      smtp: [],
      smtpChecks: {},
      smtpLastSent: {},
      smtpCheckTimer: null,
    };
  },

  computed: {
    currentLocale() {
      const loc = this.$i18n && this.$i18n.locale ? this.$i18n.locale : 'en';
      const normalized = loc.toLowerCase();
      if (normalized.startsWith('ru')) {
        return 'ru';
      }
      return 'en';
    },

    dateRangeLabel() {
      if (!this.filters.startDate || !this.filters.endDate) {
        return this.$t('dashboard.range.placeholder');
      }
      const startLabel = this.formatDayLabel(dayjs(this.filters.startDate));
      const endLabel = this.formatDayLabel(dayjs(this.filters.endDate));
      if (!startLabel || !endLabel) {
        return this.$t('dashboard.range.placeholder');
      }
      return `${startLabel} — ${endLabel}`;
    },

    rangeLength() {
      if (!this.filters.startDate || !this.filters.endDate) {
        return 0;
      }
      const start = dayjs(this.filters.startDate).startOf('day');
      const end = dayjs(this.filters.endDate).endOf('day');
      return Math.max(0, end.diff(start, 'day') + 1);
    },

    changeLabelKey() {
      return this.rangeLength === 7 ? 'dashboard.change.week' : 'dashboard.change.period';
    },

    kpiCards() {
      return KPI_META.map((meta) => {
        const total = this.kpiTotals[meta.key] || 0;
        const change = this.kpiChanges[meta.key] || { direction: 'none', text: '' };
        const displayValue = this.$utils.niceNumber(total);
        return {
          ...meta,
          label: this.$t(meta.labelKey),
          displayValue,
          change,
        };
      });
    },
  },

  methods: {
    async applyRange() {
      await this.refreshPeriod();
      this.isRangeOpen = false;
    },

    async refreshPeriod() {
      if (!this.filters.startDate || !this.filters.endDate) {
        return;
      }

      const start = dayjs(this.filters.startDate).startOf('day');
      const end = dayjs(this.filters.endDate).endOf('day');
      if (!start.isValid() || !end.isValid() || end.isBefore(start)) {
        this.$utils.toast(this.$t('dashboard.range.invalid'), 'is-danger');
        return;
      }

      const rangeDays = Math.max(1, end.diff(start, 'day') + 1);
      const prevEnd = start.clone().subtract(1, 'day').endOf('day');
      const prevStart = prevEnd.clone().subtract(rangeDays - 1, 'day').startOf('day');

      this.isKpiLoading = true;
      this.isChartsLoading = true;

      try {
        const chartsData = await this.$api.getDashboardCharts();

        const sends = Array.isArray(chartsData && chartsData.campaignSends) ? chartsData.campaignSends : [];
        const views = Array.isArray(chartsData && chartsData.campaignViews) ? chartsData.campaignViews : [];
        const replies = Array.isArray(chartsData && chartsData.campaignReplies) ? chartsData.campaignReplies : [];
        const bounces = Array.isArray(chartsData && chartsData.campaignBounces) ? chartsData.campaignBounces : [];

        const currentSends = this.filterByRange(sends, (row) => row.date, start, end, 'day');
        const previousSends = this.filterByRange(sends, (row) => row.date, prevStart, prevEnd, 'day');

        const currentViews = this.filterByRange(views, (row) => row.date, start, end, 'day');
        const previousViews = this.filterByRange(views, (row) => row.date, prevStart, prevEnd, 'day');

        const currentReplies = this.filterByRange(replies, (row) => row.date, start, end, 'day');
        const previousReplies = this.filterByRange(replies, (row) => row.date, prevStart, prevEnd, 'day');

        const currentBounces = this.filterByRange(bounces, (row) => row.date, start, end, 'day');
        const previousBounces = this.filterByRange(bounces, (row) => row.date, prevStart, prevEnd, 'day');

        const labelsMeta = this.buildLabels(start, end);
        const sentMap = this.buildDailyMap(currentSends, (row) => row.date, (row) => row.count || 0, 'day');
        const openMap = this.buildDailyMap(currentViews, (row) => row.date, (row) => row.count || 0, 'day');
        const replyMap = this.buildDailyMap(currentReplies, (row) => row.date, (row) => row.count || 0, 'day');
        const bounceMap = this.buildDailyMap(currentBounces, (row) => row.date, (row) => row.count || 0, 'day');

        const chartLabels = labelsMeta.map((item) => item.label);
        const sentSeries = labelsMeta.map((item) => sentMap[item.key] || 0);
        const openSeries = labelsMeta.map((item) => openMap[item.key] || 0);
        const replySeries = labelsMeta.map((item) => replyMap[item.key] || 0);
        const bounceSeries = labelsMeta.map((item) => bounceMap[item.key] || 0);

        this.multiChart = {
          labels: chartLabels,
          datasets: [
            {
              label: this.$t('dashboard.metrics.sent'),
              data: sentSeries,
              borderColor: CHART_COLORS.sent,
              backgroundColor: 'transparent',
              fill: false,
            },
            {
              label: this.$t('dashboard.metrics.opened'),
              data: openSeries,
              borderColor: CHART_COLORS.open,
              backgroundColor: 'transparent',
              fill: false,
            },
            {
              label: this.$t('dashboard.metrics.replied'),
              data: replySeries,
              borderColor: CHART_COLORS.reply,
              backgroundColor: 'transparent',
              fill: false,
            },
            {
              label: this.$t('dashboard.metrics.bounced'),
              data: bounceSeries,
              borderColor: CHART_COLORS.bounced,
              backgroundColor: 'transparent',
              fill: false,
            },
          ],
        };

        const sentTotal = this.sumBy(currentSends, (row) => row.count || 0);
        const previousSentTotal = this.sumBy(previousSends, (row) => row.count || 0);
        const openTotal = this.sumBy(currentViews, (row) => row.count || 0);
        const previousOpenTotal = this.sumBy(previousViews, (row) => row.count || 0);
        const replyTotal = this.sumBy(currentReplies, (row) => row.count || 0);
        const previousReplyTotal = this.sumBy(previousReplies, (row) => row.count || 0);
        const bouncedTotal = this.sumBy(currentBounces, (row) => row.count || 0);
        const previousBouncedTotal = this.sumBy(previousBounces, (row) => row.count || 0);

        this.kpiTotals.sent = sentTotal;
        this.kpiTotals.open = openTotal;
        this.kpiTotals.reply = replyTotal;
        this.kpiTotals.bounced = bouncedTotal;

        const changeLabel = this.$t(this.changeLabelKey);
        this.kpiChanges.sent = this.calculateChange(sentTotal, previousSentTotal, changeLabel);
        this.kpiChanges.open = this.calculateChange(openTotal, previousOpenTotal, changeLabel);
        this.kpiChanges.bounced = this.calculateChange(bouncedTotal, previousBouncedTotal, changeLabel, true);
        this.kpiChanges.reply = this.calculateChange(replyTotal, previousReplyTotal, changeLabel);
      } catch (e) {
        // Errors are toast-notified by the API layer.
      } finally {
        this.isKpiLoading = false;
        this.isChartsLoading = false;
      }
    },

    filterByRange(rows, dateGetter, from, to, precision = null) {
      return (rows || []).filter((row) => {
        const date = dayjs(dateGetter(row));
        if (!date.isValid()) {
          return false;
        }
        return date.isBetween(from, to, precision, '[]');
      });
    },

    buildLabels(from, to) {
      const labels = [];
      let cursor = from.clone();
      while (cursor.isSameOrBefore(to, 'day')) {
        labels.push({
          key: cursor.format('YYYY-MM-DD'),
          label: this.formatDayLabel(cursor),
        });
        cursor = cursor.add(1, 'day');
      }
      return labels;
    },

    buildDailyMap(rows, dateGetter, valueGetter, precision) {
      return (rows || []).reduce((acc, row) => {
        const date = dayjs(dateGetter(row));
        if (!date.isValid()) {
          return acc;
        }
        const key = precision === 'day' ? date.startOf('day').format('YYYY-MM-DD') : date.format('YYYY-MM-DD');
        const val = Number(valueGetter(row) || 0);
        acc[key] = (acc[key] || 0) + (Number.isFinite(val) ? val : 0);
        return acc;
      }, {});
    },

    sumBy(rows, getter) {
      return (rows || []).reduce((acc, row) => acc + Number(getter(row) || 0), 0);
    },

    calculateChange(current, previous, label, invert = false) {
      if (!previous && !current) {
        return {
          direction: 'flat',
          icon: 'minus',
          text: this.$t('dashboard.change.noChange'),
        };
      }

      if (!previous) {
        return {
          direction: invert ? 'up' : 'up',
          icon: 'trending-up',
          text: this.$t('dashboard.change.noBaseline'),
        };
      }

      const diff = current - previous;
      const percent = (diff / previous) * 100;
      const absPercent = Math.abs(percent);
      const rounded = absPercent >= 10 ? Math.round(absPercent) : Number(absPercent.toFixed(1));

      let sign = '';
      if (percent > 0) {
        sign = '+';
      } else if (percent < 0) {
        sign = '-';
      }

      let direction = 'flat';
      if (percent > 0) {
        direction = invert ? 'down' : 'up';
      } else if (percent < 0) {
        direction = invert ? 'up' : 'down';
      }

      let icon = 'minus';
      if (direction === 'up') {
        icon = 'trending-up';
      } else if (direction === 'down') {
        icon = 'trending-down';
      }

      return {
        direction,
        icon,
        text: `${sign}${rounded}% ${label}`,
      };
    },

    statusColor(status) {
      const map = {
        running: 'primary',
        finished: 'success',
        scheduled: 'info',
        paused: 'warning',
        draft: 'dark',
        cancelled: 'danger',
      };
      return map[status] || 'light';
    },

    resolveSentTotal(campaign) {
      if (campaign && campaign.stats && typeof campaign.stats.sent === 'number') {
        return campaign.stats.sent;
      }
      if (typeof campaign.sent === 'number') {
        return campaign.sent;
      }
      return 0;
    },

    formatRate(value) {
      if (!Number.isFinite(value) || value <= 0) {
        return '0%';
      }
      if (value < 1) {
        return `${value.toFixed(1)}%`;
      }
      return `${Math.round(value)}%`;
    },

    formatCampaignMeta(campaign) {
      const sent = this.resolveSentTotal(campaign);
      const opens = Number(campaign.views || 0);
      const openRate = sent > 0 ? (opens / sent) * 100 : 0;
      const parts = [
        `${this.$utils.niceNumber(sent)} ${this.$t('dashboard.latestCampaigns.sent')}`,
        `${this.formatRate(openRate)} ${this.$t('dashboard.latestCampaigns.openRate')}`,
        this.$t('dashboard.latestCampaigns.replyPlaceholder'),
      ];
      return parts.join(' • ');
    },

    formatDayLabel(date) {
      if (!date || !date.isValid || !date.isValid()) {
        return '';
      }
      const localized = date.locale(this.currentLocale).format('DD MMM');
      if (this.currentLocale !== 'ru') {
        return localized;
      }
      const cleaned = localized.replace('.', '');
      const parts = cleaned.split(' ').filter(Boolean);
      if (parts.length < 2) {
        return cleaned;
      }
      const [day, month] = parts;
      const capitalizedMonth = month.charAt(0).toUpperCase() + month.slice(1);
      return `${day} ${capitalizedMonth}`;
    },

    loadSmtp() {
      this.isSmtpLoading = true;
      this.$api.getSMTPStats().then((data) => {
        const prev = { ...this.smtpLastSent };
        this.smtp = data || [];
        (this.smtp || []).forEach((row) => {
          if (this.smtpChecks[row.name] === undefined) {
            this.$set(this.smtpChecks, row.name, false);
          }
          const last = prev[row.name] || 0;
          const curr = row.sentToday || 0;
          this.smtpLastSent[row.name] = curr;
          if (curr > last) {
            this.$set(this.smtpChecks, row.name, true);
          }
        });
        this.runSmtpChecks();
        this.scheduleSmtpChecks();
      }).catch(() => {
        this.smtp = [];
      }).finally(() => {
        this.isSmtpLoading = false;
      });
    },

    runSmtpChecks() {
      (this.smtp || []).forEach((row) => {
        if (!row || !row.name) {
          return;
        }
        this.$api.checkSMTPServer(row.name).then((resp) => {
          const connOk = !!(resp && (resp.connectionOk || (resp.banner && resp.banner.length > 0)));
          const wasGreen = this.smtpChecks[row.name] === true;
          const ok = wasGreen ? true : connOk;
          this.$set(this.smtpChecks, row.name, ok);
        }).catch(() => {
          this.$set(this.smtpChecks, row.name, false);
        });
      });
    },

    scheduleSmtpChecks() {
      if (this.smtpCheckTimer) {
        clearInterval(this.smtpCheckTimer);
      }
      this.smtpCheckTimer = setInterval(() => {
        this.loadSmtp();
      }, 60 * 60 * 1000);
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

    async loadRecentCampaigns() {
      this.isRecentLoading = true;
      try {
        const data = await this.$api.getCampaigns({
          page: 1,
          per_page: 3,
          order_by: 'created_at',
          order: 'desc',
          no_body: true,
        });
        this.recentCampaigns = data && Array.isArray(data.results) ? data.results : [];
      } catch (e) {
        this.recentCampaigns = [];
      } finally {
        this.isRecentLoading = false;
      }
    },
  },

  async mounted() {
    const end = dayjs();
    const start = end.clone().subtract(6, 'day');
    this.filters.startDate = start.toDate();
    this.filters.endDate = end.toDate();

    await Promise.all([
      this.refreshPeriod(),
      this.loadRecentCampaigns(),
    ]);

    this.loadSmtp();
  },

  beforeDestroy() {
    if (this.smtpCheckTimer) {
      clearInterval(this.smtpCheckTimer);
    }
  },
});
</script>

<style scoped lang="scss">
.dashboard-page {
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 24px;
}

.dashboard-page__hero {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.dashboard-page__title {
  font-size: 28px;
  font-weight: 700;
  margin: 0 0 4px;
  color: #111827;
}

.dashboard-page__subtitle {
  margin: 0;
  color: #6b7280;
  font-size: 15px;
}

.dashboard-page__hero-actions {
  display: flex;
  gap: 12px;
  align-items: center;
}

.dashboard-range {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: 1px solid #d9deef;
  background: #ffffff;
  border-radius: 12px;
  padding: 9px 18px;
  font-size: 14px;
  color: #1f2937;
  cursor: pointer;
  transition: all 0.2s ease;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.05);
  height: 41px;
}

.dashboard-range .icon {
  color: #94a3b8;
}

.dashboard-range:hover {
  border-color: #c7cff5;
  box-shadow: 0 6px 14px rgba(79, 70, 229, 0.12);
}

.dashboard-range:hover .icon {
  color: #64748b;
}

.dashboard-create {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-weight: 600;
  font-size: 14px;
  padding: 10px 20px;
  height: 40px;
  border-radius: 12px;
  background: linear-gradient(135deg, hsl(234, 89%, 62%), hsl(262, 83%, 58%));
  border: none;
  color: #ffffff;
  box-shadow: 0 10px 20px rgba(89, 80, 255, 0.25);
  transition: all 0.2s ease;
}

.dashboard-create:hover {
  background: linear-gradient(135deg, hsl(234, 89%, 58%), hsl(262, 83%, 54%));
  box-shadow: 0 14px 24px rgba(89, 80, 255, 0.3);
}

.dashboard-create.button.is-primary {
  background-color: transparent;
  border-color: transparent;
}

.dashboard-range__dropdown {
  padding: 16px;
}

.dashboard-range__content {
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(2, minmax(200px, 1fr));
}

.dashboard-range__label {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
  color: #6b7280;
}

.dashboard-range__footer {
  margin-top: 16px;
  text-align: right;
}

.dashboard-page__kpi-grid {
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  width: 100%;
}

.dashboard-card {
  position: relative;
  background: #ffffff;
  border-radius: 18px;
  padding: 20px;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.05);
  border: 1px solid #edf2f7;
  min-height: 110px;
  display: flex;
  flex-direction: column;
}

.dashboard-card__icon {
  position: absolute;
  top: 16px;
  right: 16px;
  width: 44px;
  height: 44px;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, rgba(79, 70, 229, 0.2), rgba(124, 58, 237, 0.2));
  color: #ffffff;
  font-size: 18px;
  box-shadow: 0 10px 20px rgba(15, 23, 42, 0.1);
}

.dashboard-card__icon .icon {
  color: #ffffff;
}

.dashboard-card__icon-svg {
  color: #ffffff;
  font-size: 20px;
}

.dashboard-card__icon[data-accent='violet'] {
  background: linear-gradient(135deg, rgba(30, 173, 250, 0.9), rgba(14, 145, 233, 0.9));
}

.dashboard-card__icon[data-accent='indigo'] {
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.9), rgba(79, 70, 229, 0.9));
}

.dashboard-card__icon[data-accent='emerald'] {
  background: linear-gradient(135deg, rgba(34, 197, 94, 0.9), rgba(16, 185, 129, 0.9));
}

.dashboard-card__icon[data-accent='rose'] {
  background: linear-gradient(135deg, rgba(244, 63, 94, 0.9), rgba(239, 68, 68, 0.9));
}

.dashboard-card__body {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.dashboard-card__label {
  font-size: 14px;
  color: #6b7280;
}

.dashboard-card__value {
  font-size: 28px;
  font-weight: 700;
  margin: 0 0 8px;
  color: #111827;
}

.dashboard-card__delta {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #6b7280;
}

.dashboard-card__delta-icon {
  display: inline-flex;
  align-items: center;
}

.dashboard-card__delta.is-up {
  color: #059669;
}

.dashboard-card__delta.is-down {
  color: #ef4444;
}

.dashboard-card__delta.is-flat {
  color: #6b7280;
}

.dashboard-card__delta.is-none {
  color: #9ca3af;
}

.dashboard-card--list {
  padding: 0;
}

.dashboard-section {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.dashboard-section__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.dashboard-section__header h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #111827;
}

.dashboard-section__link {
  font-size: 14px;
  color: #4f46e5;
}

.dashboard-section__chart {
  padding: 24px;
  min-height: 320px;
}

.dashboard-line-chart {
  height: 280px;
}

.dashboard-smtp {
  list-style: none;
  margin: 0;
  padding: 0;
}

.dashboard-smtp__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 24px;
  border-bottom: 1px solid #eef2ff;
}

.dashboard-smtp__item:last-child {
  border-bottom: none;
}

.dashboard-smtp__main {
  flex: 1 1 auto;
}

.dashboard-smtp__name {
  margin: 0 0 6px;
  font-size: 16px;
  font-weight: 600;
  color: #111827;
}

.dashboard-smtp__meta {
  margin: 0;
  font-size: 13px;
  color: #6b7280;
}

.dashboard-smtp__meta--limit {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  align-items: center;
}

.dashboard-smtp__limit-status {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-weight: 600;
  color: #16a34a;
  text-transform: uppercase;
  font-size: 11px;
  letter-spacing: 0.05em;
}

.dashboard-smtp__limit-status.is-warning {
  color: #dc2626;
}

.dashboard-smtp__limit-bar {
  position: relative;
  width: 220px;
  max-width: 100%;
  height: 6px;
  border-radius: 999px;
  background: rgba(79, 70, 229, 0.12);
  overflow: hidden;
  margin: 8px 0 4px;
}

.dashboard-smtp__limit-bar-fill {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, #4f46e5, #6366f1);
  transition: width 0.3s ease;
}

.dashboard-smtp__item[data-limit-reached="true"] .dashboard-smtp__limit-bar-fill {
  background: linear-gradient(90deg, #f97316, #ef4444);
}

.dashboard-smtp__status {
  display: flex;
  align-items: center;
  gap: 10px;
}

.dashboard-smtp__dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #f87171;
}

.dashboard-smtp__dot.is-online {
  background: #22c55e;
}

.dashboard-smtp__status-text {
  font-size: 13px;
  font-weight: 600;
  color: #1f2937;
}

.dashboard-campaigns {
  list-style: none;
  margin: 0;
  padding: 0;
}

.dashboard-campaigns__item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 24px;
  border-bottom: 1px solid #eef2ff;
}

.dashboard-campaigns__item:last-child {
  border-bottom: none;
}

.dashboard-campaigns__main {
  flex: 1 1 auto;
}

.dashboard-campaigns__name {
  font-weight: 600;
  color: #111827;
}

.dashboard-campaigns__meta {
  margin: 6px 0 0;
  font-size: 13px;
  color: #6b7280;
}

.dashboard-campaigns__status {
  display: flex;
  align-items: center;
  gap: 12px;
}

.dashboard-campaigns__status-tag {
  font-weight: 600;
  border-radius: 999px;
  padding: 4px 12px;
}

.dashboard-campaigns__menu {
  color: #9ca3af;
  display: inline-flex;
  align-items: center;
}

.dashboard-campaigns__menu:hover {
  color: #4f46e5;
}

.dashboard-empty {
  text-align: center;
  color: #6b7280;
  padding: 40px 20px;
}

@media screen and (max-width: 900px) {
  .dashboard-range__content {
    grid-template-columns: 1fr;
  }

  .dashboard-line-chart {
    height: 240px;
  }
}

@media screen and (max-width: 600px) {
  .dashboard-page {
    padding: 16px;
  }

  .dashboard-page__title {
    font-size: 24px;
  }

  .dashboard-page__hero-actions {
    flex-direction: column;
    align-items: stretch;
  }

  .dashboard-page__hero-actions .button {
    width: 100%;
  }

  .dashboard-card {
    padding: 18px;
  }
}
</style>
