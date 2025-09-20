<template>
  <section class="recent-sends">
    <header class="columns page-header">
      <div class="column is-8">
        <h1 class="title is-4">
          <b-icon icon="send" class="mr-2" />
          {{ $t('sends.title') }}
        </h1>
      </div>
    </header>

    <b-field grouped>
      <b-input v-model.number="filters.campaignId" type="number" :placeholder="$t('sends.campaignId')" />
      <b-input v-model.number="filters.limit" type="number" :placeholder="$t('sends.limit')" />
      <b-button @click="refresh" type="is-primary">{{ $t('globals.buttons.refresh') }}</b-button>
    </b-field>

    <b-table :data="rows" :loading="loading.campaigns">
      <b-table-column v-slot="p" field="createdAt" :label="$t('globals.fields.createdAt')">
        <span>{{ $utils.niceDate(p.row.createdAt, true) }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="campaignName" :label="$t('globals.terms.campaign')">
        <router-link :to="{ name: 'campaign', params: { id: p.row.campaignId } }">{{ p.row.campaignName }}</router-link>
      </b-table-column>
      <b-table-column v-slot="p" field="subscriberEmail" :label="$t('subscribers.email')">
        <span>{{ p.row.subscriberEmail }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="fromEmail" :label="$t('campaigns.fromAddress')">
        <span>{{ p.row.fromEmail }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="subject" :label="$t('campaigns.subject')">
        <span>{{ p.row.subject }}</span>
      </b-table-column>
      <b-table-column v-slot="p" field="step" :label="$t('deliveries.step')">
        <span v-if="Number.isInteger(p.row.step)">{{ (p.row.step || 0) + 1 }}</span>
        <span v-else>-</span>
      </b-table-column>
      <b-table-column v-slot="p" field="success" :label="$t('sends.status')">
        <b-tag :type="p.row.success ? 'is-success' : 'is-danger'">{{ p.row.success ? $t('sends.success') : $t('sends.failed') }}</b-tag>
      </b-table-column>
      <b-table-column v-slot="p" field="error" :label="$t('sends.error')">
        <span class="has-text-danger" v-if="p.row.error">{{ p.row.error }}</span>
        <span v-else>-</span>
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
      filters: { campaignId: '', limit: 100 },
    };
  },
  computed: { ...mapState(['loading']) },
  methods: {
    refresh() {
      const params = { campaign_id: this.filters.campaignId || 0, limit: this.filters.limit || 100 };
      this.$api.getRecentSends(params).then((d) => { this.rows = d; });
    },
  },
  mounted() { this.refresh(); },
});
</script>
