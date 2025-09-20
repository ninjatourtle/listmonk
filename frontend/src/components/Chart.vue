<template>
  <section class="chart">
    <canvas class="chart-canvas" />
  </section>
</template>

<script>
import ChartJS from 'chart.js/auto';

const DEFAULT_DONUT = {
  type: 'doughnut',
  data: {},
  options: {
    responsive: true,
    cutout: '70%',
    maintainAspectRatio: false,
    plugins: {
      legend: {
        display: false,
      },
      tooltip: {
        backgroundColor: '#fff',
        borderColor: '#ddd',
        borderWidth: 1,
        titleColor: '#666',
        bodyColor: '#666',
        bodyFont: {
          size: 15,
        },
        bodySpacing: 10,
        padding: 10,
        callbacks: {
          label: (item) => {
            const data = item.chart.data.datasets[item.datasetIndex];
            const total = data.data.reduce((acc, val) => acc + val, 0);
            const val = data.data[item.dataIndex];
            const percentage = ((val / total) * 100).toFixed(2);
            return `${val} (${percentage}%)`;
          },
        },
      },
    },
  },
};

const DEFAULT_LINE = {
  type: 'line',
  data: {},
  options: {
    responsive: true,
    lineTension: 0.5,
    maintainAspectRatio: false,
    interaction: {
      intersect: false,
      axis: 'index',
    },
    plugins: {
      legend: {
        display: false,
      },
      tooltip: {
        backgroundColor: '#fff',
        borderColor: '#ddd',
        borderWidth: 1,
        bodyColor: '#666',
        displayColors: true,
        bodyFont: {
          size: 15,
        },
        bodySpacing: 10,
        padding: 10,
      },
    },
    scales: {
      x: {
        grid: {
          display: false,
        },
      },
      y: {
        grid: {
          display: false,
        },
        ticks: {
          precision: 0,
        },
      },
    },
  },
};

const DEFAULT_BAR = {
  type: 'bar',
  data: {},
  options: {
    responsive: true,
    indexAxis: 'y',
    barThickness: 40,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        display: false,
      },
      tooltip: {
        backgroundColor: '#fff',
        borderColor: '#ddd',
        borderWidth: 1,
        titleColor: '#666',
        bodyColor: '#666',
        bodyFont: {
          size: 15,
        },
        bodySpacing: 10,
        padding: 10,
      },
    },
    scales: {
      x: {
        grid: {
          display: false,
        },
      },
      y: {
        grid: {
          display: false,
        },
      },
    },
  },
};

export default {
  name: 'Chart',

  props: {
    data: { type: Object, default: () => { } },
    type: { type: String, default: 'line' },
    onClick: { type: Function, default: () => { } },
    options: { type: Object, default: () => ({}) },
  },

  data() {
    return {
      chart: null,
    };
  },

  mounted() {
    this.initChart();
  },

  beforeDestroy() {
    if (this.chart) {
      this.chart.destroy();
      this.chart = null;
    }
  },

  watch: {
    data: {
      deep: true,
      handler() {
        this.updateData();
      },
    },
    type() {
      this.initChart();
    },
    options: {
      deep: true,
      handler() {
        this.initChart();
      },
    },
  },

  methods: {
    baseConfig() {
      switch (this.$props.type) {
        case 'donut':
          return DEFAULT_DONUT;
        case 'bar':
          return DEFAULT_BAR;
        default:
          return DEFAULT_LINE;
      }
    },

    initChart() {
      const ctx = this.$el.querySelector('.chart-canvas');
      if (this.chart) {
        this.chart.destroy();
        this.chart = null;
      }
      const def = this.baseConfig();
      const mergedOptions = { ...(def.options || {}), ...(this.$props.options || {}) };
      const conf = { ...def, data: this.$props.data, options: mergedOptions };
      if (this.$props.onClick) {
        conf.options.onClick = this.$props.onClick;
      }
      this.chart = new ChartJS(ctx, conf);
    },

    updateData() {
      if (!this.chart) {
        this.initChart();
        return;
      }
      this.chart.data = this.$props.data || {};
      this.chart.update('none');
    },
  },
};
</script>
