<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, type ApiEnvelope } from '../api'

type TrendPoint = {
  date: string
  count: number
}

type Summary = {
  users: number
  votes: number
  questions: number
  models: number
  trends: {
    users: TrendPoint[]
    votes: TrendPoint[]
    questions: TrendPoint[]
    models: TrendPoint[]
  }
  categories: Array<{ id: string; name: string; description: string }>
  topModels: Array<{ display_name: string; elo_rating: number; vote_count: number }>
}

type MetricCard = {
  key: string
  title: string
  value: number
  points: TrendPoint[]
}

const summary = ref<Summary | null>(null)
const hoveredChart = ref<{ key: string; index: number } | null>(null)
const chartWidth = 280
const chartHeight = 92

const metricCards = computed<MetricCard[]>(() => {
  const data = summary.value
  if (!data) return []
  return [
    { key: 'users', title: '用户数', value: data.users, points: data.trends.users },
    { key: 'votes', title: '投票数', value: data.votes, points: data.trends.votes },
    { key: 'questions', title: '题目数', value: data.questions, points: data.trends.questions },
    { key: 'models', title: '模型数', value: data.models, points: data.trends.models },
  ]
})

function chartCoordinates(points: TrendPoint[]) {
  const values = points.map((point) => point.count)
  const max = Math.max(...values, 1)
  const min = Math.min(...values, 0)
  const spread = Math.max(max - min, 1)
  const xStep = points.length > 1 ? chartWidth / (points.length - 1) : chartWidth

  return points.map((point, index) => {
    const x = index * xStep
    const y = chartHeight - ((point.count - min) / spread) * (chartHeight - 18) - 9
    return { x, y }
  })
}

function linePath(points: TrendPoint[]) {
  const coords = chartCoordinates(points)
  if (!coords.length) return ''
  return coords.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x.toFixed(1)} ${point.y.toFixed(1)}`).join(' ')
}

function areaPath(points: TrendPoint[]) {
  const line = linePath(points)
  if (!line) return ''
  return `${line} L ${chartWidth} ${chartHeight} L 0 ${chartHeight} Z`
}

function setChartHover(event: MouseEvent, metric: MetricCard) {
  if (!metric.points.length) return
  const svg = event.currentTarget as SVGSVGElement
  const rect = svg.getBoundingClientRect()
  const scale = Math.min(rect.width / chartWidth, rect.height / chartHeight)
  const renderedWidth = chartWidth * scale
  const renderedLeft = rect.left + (rect.width - renderedWidth) / 2
  const x = Math.max(0, Math.min(chartWidth, ((event.clientX - renderedLeft) / Math.max(renderedWidth, 1)) * chartWidth))
  const maxIndex = metric.points.length - 1
  const index = Math.max(0, Math.min(maxIndex, Math.round((x / chartWidth) * maxIndex)))
  hoveredChart.value = { key: metric.key, index }
}

function clearChartHover(key: string) {
  if (hoveredChart.value?.key === key) hoveredChart.value = null
}

function hoveredIndex(metric: MetricCard) {
  return hoveredChart.value?.key === metric.key ? hoveredChart.value.index : -1
}

function hoveredCoordinate(metric: MetricCard) {
  const index = hoveredIndex(metric)
  if (index < 0) return null
  return chartCoordinates(metric.points)[index] ?? null
}

function hoveredPoint(metric: MetricCard) {
  const index = hoveredIndex(metric)
  if (index < 0) return null
  return metric.points[index] ?? null
}

function tooltipTransform(metric: MetricCard) {
  const point = hoveredCoordinate(metric)
  if (!point) return ''
  const x = Math.min(chartWidth - 88, Math.max(4, point.x + 10))
  const y = Math.min(chartHeight - 42, Math.max(4, point.y - 46))
  return `translate(${x.toFixed(1)} ${y.toFixed(1)})`
}

function hoverX(metric: MetricCard) {
  return hoveredCoordinate(metric)?.x ?? 0
}

function hoverY(metric: MetricCard) {
  return hoveredCoordinate(metric)?.y ?? 0
}

function hoverDate(metric: MetricCard) {
  return hoveredPoint(metric)?.date ?? ''
}

function hoverCount(metric: MetricCard) {
  return hoveredPoint(metric)?.count ?? 0
}

onMounted(async () => {
  const response = await api.get<ApiEnvelope<Summary>>('/dashboard/summary')
  summary.value = response.data.data
})
</script>

<template>
  <div class="page">
    <div class="header">
      <h1>Dashboard</h1>
      <p class="muted">查看当前盲评数据、分类和模型榜单概览。</p>
    </div>
    <div v-if="summary" class="dashboard-chart-grid">
      <article v-for="metric in metricCards" :key="metric.key" class="card chart-card">
        <div class="chart-head">
          <div>
            <p class="chart-title">{{ metric.title }}趋势</p>
            <p class="muted chart-subtitle">近 14 天累计变化</p>
          </div>
          <p class="chart-value">{{ metric.value }}</p>
        </div>
        <svg
          class="metric-chart"
          :viewBox="`0 0 ${chartWidth} ${chartHeight}`"
          role="img"
          :aria-label="`${metric.title}趋势图`"
          @mousemove="setChartHover($event, metric)"
          @mouseleave="clearChartHover(metric.key)"
        >
          <defs>
            <linearGradient :id="`metric-fill-${metric.key}`" x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stop-color="currentColor" stop-opacity="0.26" />
              <stop offset="100%" stop-color="currentColor" stop-opacity="0.02" />
            </linearGradient>
          </defs>
          <path class="metric-area" :d="areaPath(metric.points)" :fill="`url(#metric-fill-${metric.key})`" />
          <path class="metric-line" :d="linePath(metric.points)" />
          <circle
            v-for="point in chartCoordinates(metric.points)"
            :key="`${metric.key}-${point.x}`"
            class="metric-dot"
            :cx="point.x"
            :cy="point.y"
            r="2.4"
          />
          <rect class="metric-hit-area" x="0" y="0" :width="chartWidth" :height="chartHeight" />
          <g v-if="hoveredCoordinate(metric) && hoveredPoint(metric)" class="metric-hover-layer">
            <line
              class="metric-hover-line"
              :x1="hoverX(metric)"
              y1="0"
              :x2="hoverX(metric)"
              :y2="chartHeight"
            />
            <circle
              class="metric-hover-dot"
              :cx="hoverX(metric)"
              :cy="hoverY(metric)"
              r="4.6"
            />
            <g class="metric-tooltip" :transform="tooltipTransform(metric)">
              <rect width="84" height="38" rx="10" />
              <text x="10" y="15">{{ hoverDate(metric) }}</text>
              <text class="metric-tooltip-value" x="10" y="30">{{ hoverCount(metric) }}</text>
            </g>
          </g>
        </svg>
        <div class="chart-foot">
          <span>{{ metric.points[0]?.date }}</span>
          <span>{{ metric.points[metric.points.length - 1]?.date }}</span>
        </div>
      </article>
    </div>
    <div class="grid" style="margin-top: 16px">
      <div class="card">
        <h2>评估主题</h2>
        <p v-for="cat in summary?.categories" :key="cat.id">
          <strong>{{ cat.name }}</strong><br />
          <span class="muted">{{ cat.description }}</span>
        </p>
      </div>
      <div class="card">
        <h2>Top Models</h2>
        <p v-if="!summary?.topModels?.length" class="muted">暂无投票，完成一次盲评后会出现排名。</p>
        <p v-for="model in summary?.topModels" :key="model.display_name">
          <strong>{{ model.display_name }}</strong>
          <span class="muted"> Elo {{ Number(model.elo_rating).toFixed(0) }} / {{ model.vote_count }} 票</span>
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dashboard-chart-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.chart-card {
  min-height: 230px;
  overflow: hidden;
  background:
    linear-gradient(145deg, color-mix(in srgb, var(--surface) 92%, transparent), color-mix(in srgb, var(--surface-2) 72%, transparent)),
    radial-gradient(circle at 88% 12%, color-mix(in srgb, var(--brand-2) 18%, transparent), transparent 34%);
}

.chart-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}

.chart-title {
  margin: 0;
  color: var(--text-primary);
  font-weight: 850;
}

.chart-subtitle {
  margin: 5px 0 0;
  font-size: 13px;
}

.chart-value {
  margin: 0;
  color: var(--text-primary);
  font-size: 28px;
  font-weight: 900;
  line-height: 1;
  font-variant-numeric: tabular-nums;
}

.metric-chart {
  width: 100%;
  height: 112px;
  color: var(--brand-2);
  overflow: visible;
}

.metric-area {
  opacity: 0.9;
}

.metric-line {
  fill: none;
  stroke: currentColor;
  stroke-width: 3;
  stroke-linecap: round;
  stroke-linejoin: round;
  filter: drop-shadow(0 8px 14px color-mix(in srgb, var(--brand-2) 24%, transparent));
}

.metric-dot {
  fill: var(--surface-solid);
  stroke: currentColor;
  stroke-width: 2;
}

.metric-hit-area {
  fill: transparent;
  pointer-events: all;
}

.metric-hover-layer {
  pointer-events: none;
}

.metric-hover-line {
  stroke: color-mix(in srgb, var(--text-secondary) 72%, transparent);
  stroke-width: 1.3;
  stroke-dasharray: 4 4;
}

.metric-hover-dot {
  fill: var(--surface-solid);
  stroke: currentColor;
  stroke-width: 2.4;
  filter: drop-shadow(0 4px 9px color-mix(in srgb, var(--brand-2) 34%, transparent));
}

.metric-tooltip rect {
  fill: color-mix(in srgb, var(--surface-solid) 96%, transparent);
  stroke: color-mix(in srgb, var(--border) 90%, transparent);
  filter: drop-shadow(0 10px 18px color-mix(in srgb, #000 18%, transparent));
}

.metric-tooltip text {
  fill: var(--text-secondary);
  font-size: 10px;
  font-weight: 750;
}

.metric-tooltip-value {
  fill: var(--text-primary) !important;
  font-size: 13px !important;
  font-weight: 900 !important;
  font-variant-numeric: tabular-nums;
}

.chart-foot {
  display: flex;
  justify-content: space-between;
  color: var(--text-secondary);
  font-size: 12px;
  margin-top: 10px;
}

@media (max-width: 760px) {
  .dashboard-chart-grid {
    grid-template-columns: 1fr;
  }
}
</style>
