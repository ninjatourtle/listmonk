<template>
  <section class="campaign-deliveries">
    <header class="columns page-header">
      <div class="column is-8">
        <h1 class="title is-4">{{ $t('campaigns.deliveriesTitle') }}</h1>
      </div>
      <div class="column has-text-right">
        <router-link :to="{ name: 'campaigns' }" class="button">{{ $t('globals.buttons.back') }}</router-link>
      </div>
    </header>

    <b-table :data="rows" :loading="loading.campaigns" paginated backend-pagination
      :total="total" :per-page="perPage" :current-page="page"
      backend-sorting @page-change="onPageChange" @sort="onSort">
      <template #top-left>
        <div class="columns">
          <div class="column is-3">
            <b-field :label="$t('globals.fields.status')">
              <b-select v-model="filters.status" :placeholder="$t('globals.buttons.clear')" @input="refresh">
                <option value="">{{ $t('globals.buttons.clear') }}</option>
                <option value="pending">{{ $t('deliveries.status.pending') }}</option>
                <option value="sent">{{ $t('deliveries.status.sent') }}</option>
                <option value="opened">{{ $t('deliveries.status.opened') }}</option>
                <option value="clicked">{{ $t('deliveries.status.clicked') }}</option>
                <option value="bounced">{{ $t('deliveries.status.bounced') }}</option>
              </b-select>
            </b-field>
          </div>
        </div>
      </template>

      <b-table-column v-slot="p" field="email" :label="$t('subscribers.email')" sortable>
        <span>{{ p.row.email }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="name" :label="$t('globals.fields.name')" sortable>
        <span>{{ p.row.name }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="status" :label="$t('globals.fields.status')" sortable>
        <b-tag :type="statusType(p.row.status)">{{ tStatus(p.row.status) }}</b-tag>
      </b-table-column>
      <b-table-column v-slot="p" field="step" :label="$t('deliveries.step')" sortable>
        <span v-if="Number.isInteger(p.row.step)">{{ p.row.step + 1 }}</span>
        <span v-else>-</span>
      </b-table-column>
      <b-table-column v-slot="p" field="sent_at" :label="$t('campaigns.sentAt')" sortable>
        <span>{{ $utils.niceDate(p.row.sentAt, true) }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="action_at" :label="$t('campaigns.actionAt')" sortable>
        <span>{{ $utils.niceDate(p.row.actionAt, true) }}</span>
      </b-table-column>
    </b-table>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';

export default Vue.extend({
  data() {
    return {
      rows: [],
      total: 0,
      perPage: 20,
      page: 1,
      orderBy: 'email',
      order: 'asc',
      filters: { status: '' },
    };
  },
  computed: {
    ...mapState(['loading']),
    campId() { return parseInt(this.$route.params.id, 10); },
  },
  methods: {
    tStatus(s) {
      switch (s) {
        case 'pending': return this.$t('deliveries.status.pending');
        case 'sent': return this.$t('deliveries.status.sent');
        case 'opened': return this.$t('deliveries.status.opened');
        case 'clicked': return this.$t('deliveries.status.clicked');
        case 'bounced': return this.$t('deliveries.status.bounced');
        default: return s;
      }
    },
    statusType(s) {
      switch (s) {
        case 'pending': return 'is-light';
        case 'sent': return 'is-info';
        case 'opened': return 'is-primary';
        case 'clicked': return 'is-success';
        case 'bounced': return 'is-danger';
        default: return '';
      }
    },
    refresh() {
      const params = {
        page: this.page,
        order_by: this.orderBy,
        order: this.order,
        status: this.filters.status,
      };
      this.$api.getCampaignDeliveries(this.campId, params).then((d) => {
        this.rows = d.results || [];
        this.total = d.total || 0;
        this.perPage = d.per_page || this.perPage;
        this.page = d.page || this.page;
      });
    },
    onPageChange(p) { this.page = p; this.refresh(); },
    onSort(field, direction) { this.orderBy = field; this.order = direction; this.refresh(); },
  },
  mounted() {
    this.refresh();
  },
});
</script>
