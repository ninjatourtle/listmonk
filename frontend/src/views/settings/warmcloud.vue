<template>
  <div>
    <div class="columns">
      <div class="column">
        <h3 class="title is-4">{{ $t('settings.warmcloud.title') }}</h3>
        <p class="subtitle is-6">{{ $t('settings.warmcloud.description') }}</p>
      </div>
    </div>

    <div class="box">
      <div class="columns">
        <div class="column is-6">
          <b-field :label="$t('settings.warmcloud.globalEnabled')" label-position="on-border">
            <b-switch v-model="data['warmcloud.global_enabled']" name="warmcloud.global_enabled" :native-value="true" />
          </b-field>
        </div>
      </div>

      <div v-if="data['warmcloud.global_enabled']">
        <div class="columns">
          <div class="column is-6">
            <b-field :label="$t('settings.warmcloud.globalToken')" label-position="on-border"
              :message="$t('settings.warmcloud.globalTokenHelp')">
              <b-input v-model="data['warmcloud.global_token']" name="warmcloud.global_token" type="text"
                placeholder="your_api_token" :maxlength="200" />
            </b-field>
          </div>
          <div class="column is-6">
            <b-field :label="$t('settings.warmcloud.globalUserId')" label-position="on-border">
              <b-numberinput v-model="data['warmcloud.global_user_id']" name="warmcloud.global_user_id" type="is-light"
                controls-position="compact" placeholder="123" min="1" />
            </b-field>
          </div>
        </div>

        <div class="columns">
          <div class="column is-6">
            <b-field :label="$t('settings.warmcloud.globalRefId')" label-position="on-border">
              <b-input v-model="data['warmcloud.global_ref_id']" name="warmcloud.global_ref_id"
                placeholder="ref123" :maxlength="100" />
            </b-field>
          </div>
          <div class="column is-6">
            <b-field :label="$t('settings.warmcloud.globalBig') || 'Provider (big)'" label-position="on-border"
              :message="$t('settings.warmcloud.globalBigHelp') || 'm — mail.ru, g — gmail.com'">
              <b-select v-model="data['warmcloud.global_big']" name="warmcloud.global_big" placeholder="m">
                <option value="m">mail.ru (m)</option>
                <option value="g">gmail.com (g)</option>
              </b-select>
            </b-field>
          </div>
        </div>
        <div class="columns">
          <div class="column is-6">
            <b-field :label="$t('settings.warmcloud.globalDefaultLimit')" label-position="on-border"
              :message="$t('settings.warmcloud.globalDefaultLimitHelp')">
              <b-numberinput v-model="data['warmcloud.global_default_limit']" name="warmcloud.global_default_limit" type="is-light"
                controls-position="compact" placeholder="10" min="1" />
            </b-field>
          </div>
        </div>

        <div class="columns">
          <div class="column is-6">
            <b-field :label="$t('settings.warmcloud.globalDefaultStep')" label-position="on-border"
              :message="$t('settings.warmcloud.globalDefaultStepHelp')">
              <b-numberinput v-model="data['warmcloud.global_default_step']" name="warmcloud.global_default_step" type="is-light"
                controls-position="compact" placeholder="5" min="1" max="50" />
            </b-field>
          </div>
          <div class="column is-6">
            <b-field :label="$t('settings.warmcloud.globalDefaultCampaignId')" label-position="on-border"
              :message="$t('settings.warmcloud.globalDefaultCampaignIdHelp')">
              <b-numberinput v-model="data['warmcloud.global_default_campaign_id']" name="warmcloud.global_default_campaign_id" type="is-light"
                controls-position="compact" placeholder="1" min="1" />
            </b-field>
          </div>
        </div>

        <div class="columns">
          <div class="column is-6">
            <b-field :label="$t('settings.warmcloud.readPercent') || 'Read percent (%)'" label-position="on-border"
              :message="$t('settings.warmcloud.readPercentHelp') || 'What percent of sent emails to mark as read/confirm'">
              <b-numberinput v-model="data['warmcloud.global_read_percent']" name="warmcloud.global_read_percent" type="is-light"
                controls-position="compact" placeholder="50" min="0" max="100" />
            </b-field>
          </div>
        </div>

        <div class="columns">
          <div class="column is-6">
            <b-field :label="$t('settings.warmcloud.maxPerTick') || 'Max per tick'" label-position="on-border"
              :message="$t('settings.warmcloud.maxPerTickHelp') || 'Maximum emails to process per minute (micro-batch)'">
              <b-numberinput v-model="data['warmcloud.global_max_per_tick']" name="warmcloud.global_max_per_tick" type="is-light"
                controls-position="compact" placeholder="5" min="0" max="100" />
            </b-field>
          </div>
          <div class="column is-6">
            <b-field :label="$t('settings.warmcloud.sleepBetween') || 'Sleep between (e.g. 3s)'" label-position="on-border"
              :message="$t('settings.warmcloud.sleepBetweenHelp') || 'Pause between micro-requests'">
              <b-input v-model="data['warmcloud.global_sleep_between']" name="warmcloud.global_sleep_between"
                placeholder="3s" />
            </b-field>
          </div>
        </div>

        <!-- Removed global working hours to avoid confusion; per-server hours are available on each SMTP -->
      </div>
    </div>
  </div>
</template>

<script>
import Vue from 'vue';

export default Vue.extend({
  props: {
    form: {
      type: Object, default: () => { },
    },
  },

  data() {
    return {
      data: this.form,
    };
  },

  methods: {
    async saveWarmCloudSettings() {
      try {
        await this.$api.updateWarmCloudSettings({
          'warmcloud.global_enabled': this.data['warmcloud.global_enabled'],
          'warmcloud.global_token': this.data['warmcloud.global_token'],
          'warmcloud.global_user_id': this.data['warmcloud.global_user_id'],
          'warmcloud.global_ref_id': this.data['warmcloud.global_ref_id'],
          'warmcloud.global_big': this.data['warmcloud.global_big'],
          'warmcloud.global_default_limit': this.data['warmcloud.global_default_limit'],
          'warmcloud.global_default_step': this.data['warmcloud.global_default_step'],
          'warmcloud.global_default_campaign_id': this.data['warmcloud.global_default_campaign_id'],
          'warmcloud.global_read_percent': this.data['warmcloud.global_read_percent'],
          'warmcloud.global_max_per_tick': this.data['warmcloud.global_max_per_tick'],
          'warmcloud.global_sleep_between': this.data['warmcloud.global_sleep_between'],
          'warmcloud.global_working_hours': this.data['warmcloud.global_working_hours'],
        });
        this.$utils.toast(this.$t('globals.messages.updated'));
      } catch (error) {
        this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
      }
    },
  },
});
</script>
