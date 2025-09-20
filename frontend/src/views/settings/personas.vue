<template>
  <section>
    <h3 class="title is-5">{{ $t('settings.personas.title') }}</h3>
    <p class="mb-4">{{ $t('settings.personas.help') }}</p>

    <div class="mb-3">
      <b-button type="is-primary" icon-left="plus" @click="addPersona">{{ $t('settings.personas.add') }}</b-button>
    </div>

    <div v-if="!data.personas || data.personas.length === 0" class="has-text-grey">
      {{ $t('settings.personas.empty') }}
    </div>

    <div v-for="(p, idx) in data.personas" :key="idx" class="box">
      <div class="is-pulled-right">
        <b-button type="is-danger" size="is-small" icon-left="delete" @click="removePersona(idx)">
          {{ $t('globals.buttons.remove') }}
        </b-button>
      </div>

      <div class="columns is-multiline">
        <div class="column is-4">
          <b-field :label="$t('settings.personas.domain')" label-position="on-border">
            <b-input v-model="p.domain" placeholder="example.com" />
          </b-field>
        </div>

        <div class="column is-4">
          <b-field :label="$t('settings.personas.label')" label-position="on-border">
            <b-input v-model="p.label" />
          </b-field>
        </div>

        <div class="column is-12">
          <b-field :label="$t('settings.personas.smtp')" :message="$t('settings.personas.smtpHelp')">
            <b-select v-model="p.smtp" multiple expanded>
              <option v-for="(s, i) in smtpOptions" :key="i" :value="s.value">{{ s.label }}</option>
            </b-select>
          </b-field>
        </div>

        <div class="column is-12">
          <h4 class="title is-6">{{ $t('settings.personas.accounts') }}</h4>
          <div class="mb-2">
            <b-button size="is-small" type="is-primary" icon-left="plus" @click="addAccount(p)">
              {{ $t('settings.personas.addAccount') }}
            </b-button>
          </div>

          <div v-if="!p.accounts || p.accounts.length === 0" class="has-text-grey is-size-7">
            {{ $t('settings.personas.noAccounts') }}
          </div>

          <div v-for="(acc, aidx) in p.accounts" :key="aidx" class="card mb-3">
            <div class="card-content">
              <div class="columns is-multiline">
                <div class="column is-3">
                  <b-field :label="$t('settings.personas.account')" label-position="on-border"
                    :message="$t('settings.personas.accountsHelp')">
                    <b-input v-model="acc.name" :placeholder="$t('settings.personas.accountsPlaceholder')" />
                  </b-field>
                </div>
                <div class="column is-2">
                  <b-field :label="$t('settings.personas.imapEnabled')">
                    <b-switch v-model="acc.imap_enabled" />
                  </b-field>
                </div>
                <div class="column is-2">
                  <b-field label="POP3 enabled">
                    <b-switch v-model="acc.pop_enabled" />
                  </b-field>
                </div>
                <div class="column has-text-right">
                  <b-button size="is-small" type="is-danger" icon-left="delete" @click="removeAccount(p, aidx)">
                    {{ $t('globals.buttons.remove') }}
                  </b-button>
                </div>
              </div>

              <div v-if="acc.imap_enabled">
                <div class="columns is-multiline">
                  <div class="column is-4">
                    <b-field :label="$t('settings.personas.imapHost')" label-position="on-border">
                      <b-input v-model="acc.imap_host" placeholder="imap.example.com" />
                    </b-field>
                  </div>
                  <div class="column is-2">
                    <b-field :label="$t('settings.personas.imapPort')" label-position="on-border">
                      <b-numberinput v-model="acc.imap_port" :min="1" :max="65535" controls-position="compact" />
                    </b-field>
                  </div>
                  <div class="column is-3">
                    <b-field :label="$t('settings.personas.imapUsername')" label-position="on-border">
                      <b-input v-model="acc.imap_username" />
                    </b-field>
                  </div>
                  <div class="column is-3">
                    <b-field :label="$t('settings.personas.imapPassword')" label-position="on-border">
                      <b-input v-model="acc.imap_password" type="password" />
                    </b-field>
                  </div>
                </div>
                <div class="columns is-multiline">
                  <div class="column is-2">
                    <b-field :label="$t('settings.personas.imapTLS')">
                      <b-switch v-model="acc.imap_tls" />
                    </b-field>
                  </div>
                  <div class="column is-4">
                    <b-field :label="$t('settings.personas.imapFolder')" label-position="on-border">
                      <b-input v-model="acc.imap_folder" placeholder="INBOX" />
                    </b-field>
                  </div>
                  <div class="column is-4">
                    <b-field :label="$t('settings.personas.imapCheckInterval')" label-position="on-border"
                      :message="$t('settings.personas.imapCheckIntervalHelp')">
                      <b-input v-model="acc.imap_check_interval" placeholder="5m" />
                    </b-field>
                  </div>
                </div>
              </div>

              <div v-if="acc.pop_enabled">
                <div class="columns is-multiline">
                  <div class="column is-4">
                    <b-field label="POP3 host" label-position="on-border">
                      <b-input v-model="acc.pop_host" placeholder="pop.example.com" />
                    </b-field>
                  </div>
                  <div class="column is-2">
                    <b-field label="POP3 port" label-position="on-border">
                      <b-numberinput v-model="acc.pop_port" :min="1" :max="65535" controls-position="compact" />
                    </b-field>
                  </div>
                  <div class="column is-3">
                    <b-field label="POP3 username" label-position="on-border">
                      <b-input v-model="acc.pop_username" />
                    </b-field>
                  </div>
                  <div class="column is-3">
                    <b-field label="POP3 password" label-position="on-border">
                      <b-input v-model="acc.pop_password" type="password" />
                    </b-field>
                  </div>
                </div>
                <div class="columns is-multiline">
                  <div class="column is-2">
                    <b-field label="POP3 TLS">
                      <b-switch v-model="acc.pop_tls_enabled" />
                    </b-field>
                  </div>
                  <div class="column is-3">
                    <b-field label="TLS skip verify">
                      <b-switch v-model="acc.pop_tls_skip_verify" />
                    </b-field>
                  </div>
                  <div class="column is-4">
                    <b-field label="POP3 check interval" label-position="on-border">
                      <b-input v-model="acc.pop_check_interval" placeholder="2m" />
                    </b-field>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="column is-12">
          <b-field :label="$t('settings.personas.suppressions')" :message="$t('settings.personas.suppressionsHelp')">
            <b-input
              type="textarea"
              v-model="p.suppressionsText"
              rows="3"
              @input="applySuppressions(p)"
              placeholder="user1@example.com\nuser2@example.com"
            />
          </b-field>
        </div>
      </div>
    </div>
  </section>
</template>

<script>
export default {
  name: 'PersonasSettings',
  props: {
    form: { type: Object, required: true },
  },
  data() {
    return {
      data: this.form,
      smtpOptions: [],
    };
  },
  mounted() {
    // Build SMTP options from form.smtp
    const opts = [];
    const servers = (this.data.smtp || []);
    for (let i = 0; i < servers.length; i += 1) {
      const nm = servers[i].name || '';
      if (nm) {
        const label = nm.startsWith('email-') ? nm.substring(6) : nm;
        opts.push({ value: nm, label });
      }
    }
    this.smtpOptions = opts;
  },
  methods: {
    addPersona() {
      if (!this.data.personas) this.$set(this.data, 'personas', []);
      this.data.personas.push({
        domain: '',
        label: '',
        smtp: [],
        accounts: [],
        suppressions: [],
        suppressionsText: '',
      });
    },
    removePersona(i) {
      this.data.personas.splice(i, 1);
    },
    addAccount(p) {
      if (!p.accounts) this.$set(p, 'accounts', []);
      p.accounts.push({
        name: '',
        imap_enabled: false,
        imap_host: '',
        imap_port: 993,
        imap_username: '',
        imap_password: '',
        imap_tls: true,
        imap_folder: 'INBOX',
        imap_check_interval: '5m',
      });
    },
    removeAccount(p, idx) {
      p.accounts.splice(idx, 1);
    },
    normalizeLocalPart(tag) {
      const v = (tag || '').toString().trim().toLowerCase();
      if (!v) return null;
      // Only allow a-z0-9._-
      if (!/^[a-z0-9._-]+$/.test(v)) return null;
      return v;
    },
    applySuppressions(p) {
      const lines = (p.suppressionsText || '')
        .split('\n')
        .map((x) => x.trim().toLowerCase())
        .filter((x) => x);
      // Dedup
      const set = new Set();
      const out = [];
      lines.forEach((e) => { if (!set.has(e)) { set.add(e); out.push(e); } });
      this.$set(p, 'suppressions', out);
    },
  },
};
</script>
