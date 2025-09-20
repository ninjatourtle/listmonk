<template>
  <section class="smtp-sends">
    <header class="columns page-header">
      <div class="column is-8">
        <h1 class="title is-4">
          <b-icon icon="send" class="mr-2" />
          {{ $t('smtp.sends.title') || 'Последние SMTP-отправки' }}
        </h1>
      </div>
    </header>

    <b-field grouped>
      <b-input v-model="filters.server" :placeholder="$t('globals.fields.server') || 'Сервер'" />
      <b-checkbox v-model="filters.today">{{ $t('globals.fields.today') || 'Сегодня' }}</b-checkbox>
      <b-input v-model="filters.from" placeholder="YYYY-MM-DD" />
      <b-input v-model="filters.to" placeholder="YYYY-MM-DD" />
      <b-select v-model="filters.status" placeholder="Статус">
        <option value="">{{ $t('globals.terms.all') || 'Все' }}</option>
        <option value="sent">sent</option>
        <option value="queued">queued</option>
      </b-select>
      <b-input v-model.number="filters.limit" type="number" :placeholder="$t('sends.limit')" />
      <b-button @click="refresh" type="is-primary">{{ $t('globals.buttons.refresh') }}</b-button>
    </b-field>

    <b-table :data="rows" :loading="loading.campaigns">
      <b-table-column v-slot="p" field="createdAt" :label="$t('globals.fields.createdAt')">
        <span>{{ $utils.niceDate(p.row.createdAt, true) }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="server" :label="$t('globals.fields.server') || 'Сервер'">
        <span>{{ p.row.server }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="email" :label="$t('subscribers.email')">
        <span>{{ p.row.email }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="subject" :label="$t('campaigns.subject')">
        <span>{{ p.row.subject }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="status" :label="$t('globals.fields.status') || 'Статус'">
        <b-tag :type="statusType(p.row.status)">{{ p.row.status || 'sent' }}</b-tag>
      </b-table-column>
    </b-table>

    <div class="buttons mt-3">
      <b-button @click="prevPage" :disabled="(filters.page || 1) <= 1">◀</b-button>
      <span class="mx-2">{{ $t('globals.fields.page') || 'Стр.' }} {{ filters.page || 1 }}</span>
      <b-button @click="nextPage">▶</b-button>
    </div>
  </section>
 </template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';

export default Vue.extend({
  data() {
    return {
      rows: [],
      filters: {
        server: '',
        today: false,
        from: '',
        to: '',
        page: 1,
        limit: 100,
        status: '',
      },
    };
  },
  computed: { ...mapState(['loading']) },
  methods: {
    refresh() {
      const params = { limit: this.filters.limit || 100, page: this.filters.page || 1 };
      if (this.filters.server) params.server = this.filters.server;
      if (this.filters.today) params.today = 1;
      if (this.filters.from) params.from = this.filters.from;
      if (this.filters.to) params.to = this.filters.to;
      if (this.filters.status) params.status = this.filters.status;
      this.$api.getSmtpSends(params).then((d) => { this.rows = d; });
    },
    statusType(s) {
      const v = (s || '').toLowerCase();
      if (v === 'queued') return 'is-warning';
      return 'is-success';
    },
    nextPage() { this.filters.page = (this.filters.page || 1) + 1; this.refresh(); },
    prevPage() { this.filters.page = Math.max(1, (this.filters.page || 1) - 1); this.refresh(); },
  },
  mounted() {
    const q = this.$route && this.$route.query ? this.$route.query : {};
    if (q.server) this.filters.server = q.server;
    if (q.today === '1' || q.today === 1 || q.today === true) this.filters.today = true;
    if (q.from) this.filters.from = q.from;
    if (q.to) this.filters.to = q.to;
    if (q.page) this.filters.page = parseInt(q.page, 10) || 1;
    this.refresh();
  },
});
</script>
