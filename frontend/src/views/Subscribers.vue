<template>
  <section class="subscribers">
    <header class="columns page-header">
      <div class="column is-10">
        <h1 class="title is-4">
          {{ $t('globals.terms.subscribers') }}
          <span v-if="!isNaN(subscribers.total)">
            (<span data-cy="count">{{ subscribers.total }}</span>)
          </span>
          <span v-if="currentList">
            &raquo; {{ currentList.name }}
            <span v-if="queryParams.subStatus" class="has-text-grey has-text-weight-normal is-capitalized">({{ queryParams.subStatus }})</span>
          </span>
        </h1>
      </div>
      <div class="column has-text-right">
        <b-field v-if="$can('subscribers:manage')" expanded>
          <b-button expanded type="is-primary" icon-left="plus" @click="showNewForm" data-cy="btn-new" class="btn-new">
            {{ $t('globals.buttons.new') }}
          </b-button>
        </b-field>
        <b-field expanded>
          <b-button expanded type="is-info" icon-left="check" @click="validateAllServer" class="btn-validate">
            {{ $t('subscribers.validate') || 'Валидировать' }}
          </b-button>
        </b-field>
        <b-field>
          <a class="button is-light" :href="'/admin/validation-logs'" target="_blank" rel="noopener noreferrer">
            <b-icon icon="format-list-bulleted-square" size="is-small" />
            &nbsp;Логи валидации
          </a>
        </b-field>
      </div>
    </header>

    <section class="subscribers-controls">
      <div class="columns">
        <div class="column is-8">
          <form @submit.prevent="onSubmit">
            <div>
              <b-field addons>
                <b-input @input="onSimpleQueryInput" v-model="queryInput" expanded
                  :placeholder="$t('subscribers.queryPlaceholder')" icon="magnify" ref="query"
                  :disabled="isSearchAdvanced" data-cy="search" />
                <p class="controls">
                  <b-button native-type="submit" type="is-primary" icon-left="magnify" :disabled="isSearchAdvanced"
                    data-cy="btn-search" />
                </p>
              </b-field>

              <div v-if="isSearchAdvanced">
                <b-input v-model="queryParams.queryExp" @keydown.native.enter="onAdvancedQueryEnter" type="textarea"
                  ref="queryExp" placeholder="subscribers.name LIKE '%user%' or subscribers.status='blocklisted'"
                  data-cy="query" />
                <span class="is-size-6 has-text-grey">
                  {{ $t('subscribers.advancedQueryHelp') }}.{{ ' ' }}
                  <a href="https://listmonk.app/docs/querying-and-segmentation" target="_blank"
                    rel="noopener noreferrer">
                    {{ $t('globals.buttons.learnMore') }}.
                  </a>
                </span>
                <div class="buttons">
                  <b-button native-type="submit" type="is-primary" icon-left="magnify" data-cy="btn-query">
                    {{
                      $t('subscribers.query') }}
                  </b-button>
                  <b-button @click.prevent="toggleAdvancedSearch" icon-left="cancel" data-cy="btn-query-reset">
                    {{ $t('subscribers.reset') }}
                  </b-button>
                </div>
              </div><!-- advanced query -->
            </div>
          </form>
          <div v-if="!isSearchAdvanced" class="toggle-advanced">
            <a href="#" @click.prevent="toggleAdvancedSearch" data-cy="btn-advanced-search">
              <b-icon icon="cog-outline" size="is-small" />
              {{ $t('subscribers.advancedQuery') }}
            </a>
          </div>
        </div><!-- search -->
      </div>
    </section><!-- control -->

    <br />
    <b-table :data="subscribers.results ?? []" :loading="loading.subscribers" @check-all="onTableCheck"
      @check="onTableCheck" :checked-rows.sync="bulk.checked" paginated backend-pagination pagination-position="both"
      @page-change="onPageChange" :current-page="queryParams.page" :per-page="subscribers.perPage"
      :total="subscribers.total" hoverable checkable backend-sorting @sort="onSort">
      <template #top-left>
        <div class="actions">
          <a class="a" href="#" @click.prevent="exportSubscribers" data-cy="btn-export-subscribers">
            <b-icon icon="cloud-download-outline" size="is-small" />
            {{ $t('subscribers.export') }}
          </a>
          <template v-if="bulk.checked.length > 0">
            <a class="a" href="#" @click.prevent="validateSelected" data-cy="btn-validate-selected">
              <b-icon icon="check" size="is-small" /> {{ $t('subscribers.validateAndBlocklist') || 'Валидировать и в черный список' }}
            </a>
            <a class="a" href="#" @click.prevent="showBulkListForm" data-cy="btn-manage-lists">
              <b-icon icon="format-list-bulleted-square" size="is-small" /> Manage lists
            </a>
            <a class="a" href="#" @click.prevent="deleteSubscribers" data-cy="btn-delete-subscribers">
              <b-icon icon="trash-can-outline" size="is-small" /> Delete
            </a>
            <a class="a" href="#" @click.prevent="blocklistSubscribers" data-cy="btn-manage-blocklist">
              <b-icon icon="account-off-outline" size="is-small" /> Blocklist
            </a>
            <span class="a">
              {{ $t('subscribers.numSelected', { num: numSelectedSubscribers }) }}
              <span v-if="!bulk.all && subscribers.total > subscribers.perPage">
                &mdash;
                <a href="#" @click.prevent="selectAllSubscribers">
                  {{ $t('subscribers.selectAll', { num: subscribers.total }) }}
                </a>
              </span>
            </span>
          </template>
        </div>
      </template>

      <b-table-column v-slot="props" field="email" :label="$t('subscribers.email')" header-class="cy-email" sortable
        :td-attrs="$utils.tdID">
        <a :href="`/subscribers/${props.row.id}`" @click.prevent="showEditForm(props.row)"
          :class="{ 'blocklisted': props.row.status === 'blocklisted' }">
          {{ props.row.email }}
          <copy-text :text="`${props.row.email}`" hide-text />
        </a>
        <b-tag v-if="props.row.status !== 'enabled'" :class="props.row.status" data-cy="blocklisted">
          {{ $t(`subscribers.status.${props.row.status}`) }}
        </b-tag>
        <b-taglist>
          <template v-for="l in props.row.lists">
            <router-link :to="`/subscribers/lists/${l.id}`" :key="l.id" style="padding-right:0.5em;">
              <b-tag :class="l.subscriptionStatus" size="is-small" :key="l.id">
                {{ l.name }}
                <sup v-if="l.optin === 'double' || l.subscriptionStatus == 'unsubscribed'">
                  {{ $t(`subscribers.status.${l.subscriptionStatus}`) }}
                </sup>
              </b-tag>
            </router-link>
          </template>
        </b-taglist>
      </b-table-column>

      <b-table-column v-slot="props" field="name" :label="$t('globals.fields.name')" header-class="cy-name" sortable>
        <a :href="`/subscribers/${props.row.id}`" @click.prevent="showEditForm(props.row)"
          :class="{ 'blocklisted': props.row.status === 'blocklisted' }">
          {{ props.row.name }}
          <copy-text :text="`${props.row.name}`" hide-text />
        </a>
      </b-table-column>

      <b-table-column v-slot="props" field="lists" :label="$t('globals.terms.lists')" header-class="cy-lists" centered>
        {{ listCount(props.row.lists) }}
      </b-table-column>

      <b-table-column v-slot="props" field="created_at" :label="$t('globals.fields.createdAt')"
        header-class="cy-created_at" sortable>
        {{ $utils.niceDate(props.row.createdAt) }}
      </b-table-column>

      <b-table-column v-slot="props" field="updated_at" :label="$t('globals.fields.updatedAt')"
        header-class="cy-updated_at" sortable>
        {{ $utils.niceDate(props.row.updatedAt) }}
      </b-table-column>

      <b-table-column v-slot="props" cell-class="actions" align="right">
        <div>
          <a :href="`/api/subscribers/${props.row.id}/export`" data-cy="btn-download"
            :aria-label="$t('subscribers.downloadData')">
            <b-tooltip :label="$t('subscribers.downloadData')" type="is-dark">
              <b-icon icon="cloud-download-outline" size="is-small" />
            </b-tooltip>
          </a>
          <a href="#" @click.prevent="openHistory(props.row)" :aria-label="'История валидации'">
            <b-tooltip label="История валидации" type="is-dark">
              <b-icon icon="history" size="is-small" />
            </b-tooltip>
          </a>
          <a v-if="$can('subscribers:manage')" :href="`/subscribers/${props.row.id}`"
            @click.prevent="showEditForm(props.row)" data-cy="btn-edit" :aria-label="$t('globals.buttons.edit')">
            <b-tooltip :label="$t('globals.buttons.edit')" type="is-dark">
              <b-icon icon="pencil-outline" size="is-small" />
            </b-tooltip>
          </a>
          <a v-if="$can('subscribers:manage')" href="#" @click.prevent="deleteSubscriber(props.row)"
            data-cy="btn-delete" :aria-label="$t('globals.buttons.delete')">
            <b-tooltip :label="$t('globals.buttons.delete')" type="is-dark">
              <b-icon icon="trash-can-outline" size="is-small" />
            </b-tooltip>
          </a>
        </div>
      </b-table-column>

      <template #empty v-if="!loading.subscribers">
        <empty-placeholder />
      </template>
    </b-table>

    <!-- Manage list modal -->
    <b-modal scroll="keep" :aria-modal="true" :active.sync="isBulkListFormVisible" :width="500" class="has-overflow">
      <subscriber-bulk-list :num-subscribers="this.numSelectedSubscribers" @finished="bulkChangeLists" />
    </b-modal>

    <!-- Add / edit form modal -->
    <b-modal scroll="keep" :aria-modal="true" :active.sync="isFormVisible" :width="850" @close="onFormClose">
      <subscriber-form :data="curItem" :is-editing="isEditing" @finished="querySubscribers" />
    </b-modal>

    <!-- Validate email modal (inside root section) -->
    <b-modal :active.sync="showValidate" :width="700">
      <div class="box">
        <h4 class="title is-4">{{ $t('subscribers.validate') || 'Валидировать' }}</h4>
        <b-field :label="$t('subscribers.email')">
          <b-input v-model="validateEmailStr" placeholder="user@example.com" />
        </b-field>
        <div class="buttons">
          <b-button type="is-primary" :loading="validating" @click="runValidate">{{ $t('globals.buttons.run') || 'Проверить' }}</b-button>
          <b-button @click="showValidate = false">{{ $t('globals.buttons.close') || 'Закрыть' }}</b-button>
        </div>
        <div v-if="validateResult">
          <h5 class="title is-5">Результат</h5>
          <ul>
            <li><strong>Пользователь:</strong> {{ validateResult.user }}</li>
            <li><strong>Домен:</strong> {{ validateResult.domain }}</li>
            <li><strong>Синтаксис:</strong> {{ validateResult.syntaxOk ? 'Успешно' : 'Ошибка' }}</li>
            <li>
              <strong>MX-записи:</strong>
              {{ validateResult.mxOk ? 'Успешно' : 'Ошибка' }}
              <span v-if="validateResult.mxHosts && validateResult.mxHosts.length">
                ({{ validateResult.mxHosts.join(', ') }})
              </span>
            </li>
            <li><strong>Доставка:</strong> {{ validateResult.deliveryOk ? 'Успешно' : 'Ошибка' }}</li>
            <li><strong>Бесплатная:</strong> {{ validateResult.free ? 'Да' : 'Нет' }}</li>
            <li><strong>Временный:</strong> {{ validateResult.temporary ? 'Да' : 'Нет' }}</li>
          </ul>
          <h5 class="title is-5 mt-3">Диалог</h5>
          <pre style="max-height: 240px; overflow:auto;">{{ renderDialog(validateResult.dialog) }}</pre>
        </div>
      </div>
    </b-modal>

    <!-- Bulk validation progress -->
    <b-modal :active.sync="showBulkValidate" :width="800">
      <div class="box">
        <h4 class="title is-4">Валидация подписчиков</h4>
        <b-progress :value="bulkDone" :max="bulkTotal" type="is-info" show-value>
          {{ Math.round(100 * bulkDone / (bulkTotal || 1)) }}%
        </b-progress>
        <p class="mt-2">Всего: {{ bulkTotal }}, Готово: {{ bulkDone }}, В чёрный список: {{ bulkInvalid }}</p>
        <pre style="max-height: 300px; overflow:auto; background:#0b0b0b; color:#ddd; padding:8px;">
{{ bulkLogs.join('\n') }}
        </pre>
        <div class="buttons">
          <b-button v-if="!bulkRunning" type="is-primary" @click="showBulkValidate = false">Закрыть</b-button>
          <b-button v-if="bulkRunning" disabled>Идёт валидация…</b-button>
        </div>
      </div>
    </b-modal>

    <!-- Per-subscriber history -->
    <b-modal :active.sync="showHistory" :width="700">
      <div class="box">
        <h4 class="title is-4">История валидации — {{ historySub && historySub.email }}</h4>
        <pre style="max-height: 400px; overflow:auto;">{{ renderHistory(historyItems) }}</pre>
        <div class="buttons">
          <b-button @click="clearHistory(historySub.id)" type="is-danger">Очистить</b-button>
          <b-button @click="showHistory = false">Закрыть</b-button>
        </div>
      </div>
    </b-modal>
  </section>
</template>

<script>
import Vue from 'vue';
import { mapState } from 'vuex';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import { uris } from '../constants';
import SubscriberBulkList from './SubscriberBulkList.vue';
import SubscriberForm from './SubscriberForm.vue';
import CopyText from '../components/CopyText.vue';

export default Vue.extend({
  components: {
    SubscriberForm,
    SubscriberBulkList,
    CopyText,
    EmptyPlaceholder,
  },

  data() {
    return {
      // Current subscriber item being edited.
      curItem: null,
      isSearchAdvanced: false,
      isEditing: false,
      isFormVisible: false,
      isBulkListFormVisible: false,

      // Table bulk row selection states.
      bulk: {
        checked: [],
        all: false,
      },

      queryInput: '',

      // Query params to filter the getSubscribers() API call.
      queryParams: {
        // Search query expression.
        queryExp: '',
        search: '',

        // ID of the list the current subscriber view is filtered by.
        listID: null,
        page: 1,
        orderBy: 'id',
        order: 'desc',
        subStatus: null,
      },
      showValidate: false,
      validateEmailStr: '',
      validating: false,
      validateResult: null,

      // Bulk validation progress
      showBulkValidate: false,
      bulkRunning: false,
      bulkTotal: 0,
      bulkDone: 0,
      bulkInvalid: 0,
      bulkLogs: [],

      // History modal
      showHistory: false,
      historyItems: [],
      historySub: null,
    };
  },

  methods: {
    // Count the lists from which a subscriber has not unsubscribed.
    listCount(lists) {
      return lists.reduce((defVal, item) => (defVal + (item.subscriptionStatus !== 'unsubscribed' ? 1 : 0)), 0);
    },
    openValidate() {
      this.validateEmailStr = '';
      this.validateResult = null;
      this.showValidate = true;
    },
    async validateAllServer() {
      // Запуск серверной валидации для всех подписчиков (всей базы)
      this.$utils.confirm('Проверить ВСЕХ подписчиков и смотреть прогресс в логах?', async () => {
        window.open('/admin/validation-logs', '_blank');
        try {
          await this.$api.validateSubscribersByQuery({ all: true });
          this.$utils.toast('Запущено. Прогресс в логах.', 'is-success');
        } catch (e) {
          this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
        }
      });
    },
    startBulkValidation(rows) {
      // Start bulk UI
      this.showBulkValidate = true;
      this.bulkRunning = true;
      this.bulkTotal = rows.length;
      this.bulkDone = 0;
      this.bulkInvalid = 0;
      this.bulkLogs = [];
      const work = rows.slice();
      const next = () => {
        if (work.length === 0) {
          // finished
          this.bulkRunning = false;
          this.querySubscribers();
          return;
        }
        const row = work.shift();
        const { email } = row;
        this.$api.validateSubscriberEmail(email).then((resp) => {
          const ok = resp.syntaxOk && resp.mxOk && resp.deliveryOk;
          const line = `${ok ? 'OK' : 'FAIL'} | ${email} | syntax:${resp.syntaxOk ? 'ok' : 'err'} mx:${resp.mxOk ? 'ok' : 'err'} delivery:${resp.deliveryOk ? 'ok' : 'err'}`;
          this.bulkLogs.push(line);
          if (this.bulkLogs.length > 500) {
            this.bulkLogs = this.bulkLogs.slice(-500);
          }
          this.saveHistory(row.id, email, resp, ok);
          if (!ok) {
            this.$api.blocklistSubscribers({ ids: [row.id] }).then(() => {
              this.bulkInvalid += 1;
            }).catch(() => { this.bulkInvalid += 1; });
          }
        }).catch((e) => {
          this.bulkLogs.push(`ERR | ${email} | ${e}`);
          if (this.bulkLogs.length > 500) {
            this.bulkLogs = this.bulkLogs.slice(-500);
          }
          this.$api.blocklistSubscribers({ ids: [row.id] }).then(() => {
            this.bulkInvalid += 1;
          }).catch(() => { this.bulkInvalid += 1; });
        }).finally(() => {
          this.bulkDone += 1;
          setTimeout(next, 100);
        });
      };
      next();
    },
    renderDialog(d) {
      if (!d || !Array.isArray(d)) return '';
      return d.map((l) => (l.dir ? `${l.dir}> ${l.msg}` : `${l.Dir || ''}> ${l.Msg || ''}`)).join('\n');
    },
    runValidate() {
      const email = (this.validateEmailStr || '').trim();
      if (!email) return;
      this.validating = true;
      this.$api.validateSubscriberEmail(email).then((resp) => {
        this.validateResult = resp;
      }).catch(() => {}).finally(() => { this.validating = false; });
    },

    toggleAdvancedSearch() {
      this.isSearchAdvanced = !this.isSearchAdvanced;
      this.queryParams.search = '';

      // Toggling to simple search.
      if (!this.isSearchAdvanced) {
        this.queryInput = '';
        this.queryParams.queryExp = '';
        this.queryParams.page = 1;
        this.querySubscribers();
        this.$refs.query.focus();
        return;
      }

      // Toggling to advanced search.
      const q = this.queryInput.replace(/'/, "''").trim();
      if (q) {
        if (this.$utils.validateEmail(q)) {
          this.queryParams.queryExp = `email = '${q.toLowerCase()}'`;
        } else {
          this.queryParams.queryExp = `(name ~* '${q}' OR email ~* '${q.toLowerCase()}')`;
        }
      }

      // Toggling to advanced search.
      this.$nextTick(() => {
        this.$refs.queryExp.focus();
      });
    },

    // Mark all subscribers in the query as selected.
    selectAllSubscribers() {
      this.bulk.all = true;
    },

    onTableCheck() {
      // Disable bulk.all selection if there are no rows checked in the table.
      if (this.bulk.checked.length !== this.subscribers.total) {
        this.bulk.all = false;
      }
    },

    // Show the edit list form.
    showEditForm(sub) {
      this.curItem = sub;
      this.isFormVisible = true;
      this.isEditing = true;
    },

    // Show the new list form.
    showNewForm() {
      this.curItem = {};
      this.isFormVisible = true;
      this.isEditing = false;
    },

    showBulkListForm() {
      this.isBulkListFormVisible = true;
    },

    onFormClose() {
      if (this.$route.params.id) {
        this.$router.push({ name: 'subscribers' });
      }
    },

    onPageChange(p) {
      this.querySubscribers({ page: p });
    },

    onSort(field, direction) {
      this.querySubscribers({ orderBy: field, order: direction });
    },

    // Prepares an SQL expression for simple name search inputs and saves it
    // in this.queryExp.
    onSimpleQueryInput(v) {
      const q = v.replace(/'/, "''").trim();
      this.queryParams.queryExp = '';
      this.queryParams.page = 1;
      this.queryParams.search = q.toLowerCase();
    },

    // Ctrl + Enter on the advanced query searches.
    onAdvancedQueryEnter(e) {
      if (e.ctrlKey) {
        this.onSubmit();
      }
    },

    onSubmit() {
      this.querySubscribers({ page: 1 });
    },

    // Search / query subscribers.
    querySubscribers(params) {
      this.queryParams = { ...this.queryParams, ...params };

      const qp = {
        list_id: this.queryParams.listID,
        search: this.queryParams.search,
        query: this.queryParams.queryExp,
        page: this.queryParams.page,
        subscription_status: this.queryParams.subStatus,
        order_by: this.queryParams.orderBy,
        order: this.queryParams.order,
      };

      if (this.queryParams.queryExp) {
        delete qp.search;
      } else {
        delete qp.queryExp;
      }

      this.$nextTick(() => {
        this.$api.getSubscribers(qp).then(() => {
          this.bulk.checked = [];
        });
      });
    },

    deleteSubscriber(sub) {
      this.$utils.confirm(
        null,
        () => {
          this.$api.deleteSubscriber(sub.id).then(() => {
            this.querySubscribers();

            this.$utils.toast(this.$t('globals.messages.deleted', { name: sub.name }));
          });
        },
      );
    },

    blocklistSubscribers() {
      let fn = null;
      if (!this.bulk.all && this.bulk.checked.length > 0) {
        // If 'all' is not selected, blocklist subscribers by IDs.
        fn = () => {
          const ids = this.bulk.checked.map((s) => s.id);
          this.$api.blocklistSubscribers({ ids })
            .then(() => this.querySubscribers());
        };
      } else {
        // 'All' is selected, blocklist by query.
        fn = () => {
          this.$api.blocklistSubscribersByQuery({
            search: this.queryParams.search,
            query: this.queryParams.queryExp,
            list_ids: this.queryParams.listID ? [this.queryParams.listID] : null,
            subscription_status: this.queryParams.subStatus,
          }).then(() => this.querySubscribers());
        };
      }

      this.$utils.confirm(this.$t('subscribers.confirmBlocklist', { num: this.numSelectedSubscribers }), fn);
    },

    exportSubscribers() {
      const num = !this.bulk.all && this.bulk.checked.length > 0
        ? this.bulk.checked.length : this.subscribers.total;

      this.$utils.confirm(this.$t('subscribers.confirmExport', { num }), () => {
        const q = new URLSearchParams();

        if (this.queryParams.search) {
          q.append('search', this.queryParams.search);
        } else if (this.queryParams.queryExp) {
          q.append('query', this.queryParams.queryExp);
        }

        if (this.queryParams.listID) {
          q.append('list_id', this.queryParams.listID);
        }

        if (this.queryParams.subStatus) {
          q.append('subscription_status', this.queryParams.subStatus);
        }

        // Export selected subscribers.
        if (!this.bulk.all && this.bulk.checked.length > 0) {
          this.bulk.checked.map((s) => q.append('id', s.id));
        }

        document.location.href = `${uris.exportSubscribers}?${q.toString()}`;
      });
    },

    deleteSubscribers() {
      let fn = null;
      if (!this.bulk.all && this.bulk.checked.length > 0) {
        // If 'all' is not selected, delete subscribers by IDs.
        fn = () => {
          const ids = this.bulk.checked.map((s) => s.id);
          this.$api.deleteSubscribers({ id: ids })
            .then(() => {
              this.querySubscribers();

              this.$utils.toast(this.$t('subscribers.subscribersDeleted', { num: this.numSelectedSubscribers }));
            });
        };
      } else {
        // 'All' is selected, delete by query.
        fn = () => {
          this.$api.deleteSubscribersByQuery({
            // If the query expression is empty, explicitly pass `all=true`
            // so that the backend deletes all records in the DB with an empty query string.
            all: this.queryParams.queryExp.trim() === '' && this.queryParams.search.trim() === '',
            search: this.queryParams.search,
            query: this.queryParams.queryExp,
            list_ids: this.queryParams.listID ? [this.queryParams.listID] : null,
            subscription_status: this.queryParams.subStatus,
          }).then(() => {
            this.querySubscribers();

            this.$utils.toast(this.$t(
              'subscribers.subscribersDeleted',
              { num: this.numSelectedSubscribers },
            ));
          });
        };
      }

      this.$utils.confirm(this.$t('subscribers.confirmDelete', { num: this.numSelectedSubscribers }), fn);
    },

    // Проверить выбранные (сервером) и смотреть прогресс в логах
    validateSelected() {
      const rows = (this.bulk && this.bulk.checked) ? this.bulk.checked.slice() : [];
      if (!rows.length) { this.$utils.toast('Не выбрано ни одного подписчика', 'is-danger'); return; }
      const ids = rows.map((r) => r.id);
      this.$utils.confirm('Проверить выбранные адреса и смотреть прогресс в логах?', async () => {
        window.open('/admin/validation-logs', '_blank');
        try {
          await this.$api.validateSubscribersBulk(ids);
          this.$utils.toast('Запущено. Прогресс в логах.', 'is-success');
        } catch (e) {
          this.$utils.toast(this.$t('globals.messages.internalError'), 'is-danger');
        }
      });
    },
    // History helpers
    historyKey(id) { return `validation_history_${id}`; },
    saveHistory(id, email, result, ok) {
      const key = this.historyKey(id);
      let arr = [];
      try { arr = JSON.parse(localStorage.getItem(key) || '[]'); } catch (e) { arr = []; }
      arr.push({
        ts: Date.now(),
        email,
        ok,
        result,
      });
      try { localStorage.setItem(key, JSON.stringify(arr.slice(-50))); } catch (e) { /* ignore */ }
    },
    openHistory(row) {
      this.historySub = row;
      const key = this.historyKey(row.id);
      try { this.historyItems = JSON.parse(localStorage.getItem(key) || '[]'); } catch (e) { this.historyItems = []; }
      this.showHistory = true;
    },
    clearHistory(id) {
      try { localStorage.removeItem(this.historyKey(id)); } catch (e) { /* ignore */ }
      this.historyItems = [];
    },
    renderHistory(items) {
      if (!items || !items.length) return 'Нет записей';
      return items.map((it) => {
        const d = new Date(it.ts).toLocaleString();
        const st = it.ok ? 'OK' : 'FAIL';
        const r = it.result || {};
        return `${d} | ${st} | ${it.email} | syntax:${r.syntaxOk ? 'ok' : 'err'} mx:${r.mxOk ? 'ok' : 'err'} delivery:${r.deliveryOk ? 'ok' : 'err'}`;
      }).join('\n');
    },

    bulkChangeLists(action, preconfirm, lists) {
      const data = {
        action,
        query: this.fullQueryExp,
        search: this.queryParams.search,
        list_ids: this.queryParams.listID ? [this.queryParams.listID] : null,
        target_list_ids: lists.map((l) => l.id),
      };

      if (preconfirm) {
        data.status = 'confirmed';
      }

      let fn = null;
      if (!this.bulk.all && this.bulk.checked.length > 0) {
        // If 'all' is not selected, perform by IDs.
        fn = this.$api.addSubscribersToLists;
        data.ids = this.bulk.checked.map((s) => s.id);
      } else {
        // 'All' is selected, perform by query.
        data.query = this.queryParams.queryExp;
        data.subscription_status = this.queryParams.subStatus;
        fn = this.$api.addSubscribersToListsByQuery;
      }

      fn(data).then(() => {
        this.querySubscribers();
        this.$utils.toast(this.$t('subscribers.listChangeApplied'));
      });
    },
  },

  computed: {
    ...mapState(['subscribers', 'lists', 'loading']),

    numSelectedSubscribers() {
      if (this.bulk.all) {
        return this.subscribers.total;
      }
      return this.bulk.checked.length;
    },

    // Returns the list that the subscribers are being filtered by in.
    currentList() {
      if (!this.queryParams.listID || !this.lists.results) {
        return null;
      }

      return this.lists.results.find((l) => l.id === this.queryParams.listID);
    },
  },

  mounted() {
    if (this.$route.params.listID) {
      this.queryParams.listID = parseInt(this.$route.params.listID, 10);
    }
    if (this.$route.query.subscription_status) {
      this.queryParams.subStatus = this.$route.query.subscription_status;
    }

    if (this.$route.params.id) {
      this.$api.getSubscriber(parseInt(this.$route.params.id, 10)).then((data) => {
        this.showEditForm(data);
      });
    } else {
      // Get subscribers on load.
      this.querySubscribers();
    }
  },
});
</script>
