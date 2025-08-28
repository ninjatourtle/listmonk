<template>
  <section>
    <h1 class="title">{{ $t('smtp.stats.title') }}</h1>
    <b-table :data="stats" :striped="true">
      <b-table-column field="name" :label="$t('globals.fields.name')" />
      <b-table-column field="sent_total" :label="$t('smtp.stats.total')" />
      <b-table-column field="sent_today" :label="$t('smtp.stats.today')" />
      <b-table-column field="actions" :label="$t('globals.terms.actions')">
        <template #default="props">
          <b-button size="is-small" @click="openSettings(props.row)">{{ $t('globals.buttons.settings') }}</b-button>
        </template>
      </b-table-column>
    </b-table>
    <b-modal :active.sync="showModal">
      <div class="box">
        <b-field :label="$t('smtp.stats.dailyLimit')">
          <b-input v-model="current.daily_limit" type="number" min="0" />
        </b-field>
        <b-field :label="$t('smtp.stats.sendPause')">
          <b-input v-model="current.send_pause" placeholder="0s" />
        </b-field>
        <b-button type="is-primary" @click="save">{{ $t('globals.buttons.save') }}</b-button>
      </div>
    </b-modal>
  </section>
</template>

<script>
export default {
  data() {
    return {
      stats: [],
      showModal: false,
      current: {},
    };
  },
  methods: {
    fetch() {
      this.$api.getSMTPStats().then((resp) => {
        this.stats = resp;
      });
    },
    openSettings(row) {
      this.current = { ...row };
      this.showModal = true;
    },
    save() {
      this.$api.updateSMTPServer(this.current.name, {
        daily_limit: this.current.daily_limit,
        send_pause: this.current.send_pause,
      }).then(() => {
        this.showModal = false;
        this.fetch();
      });
    },
  },
  created() {
    this.fetch();
  },
};
</script>
