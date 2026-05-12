<script setup lang="ts">
import { computed, nextTick, onMounted, ref, useTemplateRef, watch } from 'vue'
import { api, loadCategories, loadPeerMatrix, type ApiEnvelope, type Category, type PeerMatrixResponse } from '../api'
import {
  arenaCreativeWritingRows,
  arenaOverallRows,
  type ArenaLeaderboardRow,
} from '../data/arenaLeaderboards'

type RankRow = {
  modelId: string
  displayName: string
  provider: string
  voteCount: number
  winCount: number
  lossCount: number
  eloRating: number
  lastEloDelta: number
  winRate: number
}

type RankingTab = 'kanshan' | 'ability'
type AbilityLeaderboard = 'overall' | 'creativeWriting'
type AbilityScope = 'platform' | 'all'

const categories = ref<Category[]>([])
const categoryId = ref('')
const rows = ref<RankRow[]>([])
const peerMatrix = ref<PeerMatrixResponse>({ models: [], cells: [], sampleCount: 0 })
const activeTab = ref<RankingTab>('kanshan')
const abilityLeaderboard = ref<AbilityLeaderboard>('overall')
const abilityScope = ref<AbilityScope>('platform')
const hoveredAbilityPointId = ref('')
const categorySegmentRef = useTemplateRef<HTMLElement>('categorySegment')
const categoryIndicator = ref({ width: 0, x: 0 })

const categoryActiveIndex = computed(() => {
  if (!categoryId.value) return 0
  const i = categories.value.findIndex((cat) => cat.id === categoryId.value)
  return i >= 0 ? i + 1 : 0
})

async function updateCategoryIndicator() {
  await nextTick()
  const el = categorySegmentRef.value
  const active = el?.querySelector<HTMLElement>('.segmented-btn--active')
  if (!el || !active) return
  categoryIndicator.value = { width: active.offsetWidth, x: active.offsetLeft }
}

const categoryIndicatorStyle = computed(() => ({
  '--seg-indicator-width': `${categoryIndicator.value.width}px`,
  '--seg-indicator-x': `${categoryIndicator.value.x}px`,
}))

const chart = {
  width: 760,
  height: 360,
  left: 62,
  right: 24,
  top: 28,
  bottom: 48,
}
const priceTicks = [0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 20, 50, 100]
const priceMin = priceTicks[0]
const priceMax = priceTicks[priceTicks.length - 1]
const plotWidth = chart.width - chart.left - chart.right
const plotHeight = chart.height - chart.top - chart.bottom

/** 静态徽标根路径（Vite `public/`，生产环境含 BASE_URL） */
const vendorAssetBase = `${import.meta.env.BASE_URL.replace(/\/?$/, '/')}`

function vendorLogoHref(row: Pick<RankRow, 'provider' | 'displayName'>): string {
  const p = row.provider.toLowerCase()
  const n = row.displayName.toLowerCase()
  const file = (name: string) => `${vendorAssetBase}assets/vendors/${name}`

  // 中国厂商 — 优先匹配展示名里的常见关键词（provider 未统一时也能识别）
  if (p.includes('alibaba') || n.includes('qwen') || n.includes('通义')) return file('alibaba.svg')
  if (p.includes('bytedance') || n.includes('doubao') || n.includes('豆包') || (n.includes('seed') && n.includes('字节')))
    return file('bytedance.svg')
  if (p.includes('baidu') || n.includes('ernie') || n.includes('文心')) return file('baidu.svg')
  if (p.includes('deepseek') || n.includes('deepseek')) return file('deepseek.svg')
  if (p.includes('tencent') || n.includes('hunyuan') || n.includes('混元')) return file('tencent.svg')
  if (p.includes('moonshot') || n.includes('kimi')) return file('moonshot.svg')
  if (p.includes('zhipu') || n.includes('glm') || n.includes('智谱')) return file('zhipu.svg')
  if (p.includes('minimax')) return file('minimax.svg')
  if (p.includes('stepfun') || n.includes('stepfun') || n.includes('阶跃')) return file('stepfun.svg')
  if (p.includes('baichuan') || n.includes('百川')) return file('baichuan.svg')
  if (p.includes('volcengine') || n.includes('火山')) return file('volcengine.svg')
  if (p.includes('iflytek') || n.includes('星火') || n.includes('讯飞')) return file('iflytek.svg')

  // 美国 / 国际常见云与实验室
  if (p.includes('openai') || /\bgpt\b/.test(n)) return file('openai.svg')
  if (p.includes('anthropic') || n.includes('claude')) return file('anthropic.svg')
  if (p.includes('google') || n.includes('gemini')) return file('google.svg')
  if (p.includes('meta') || /\bllama\b/.test(n)) return file('meta.svg')
  if (p.includes('mistral')) return file('mistral.svg')
  if (p.includes('x-ai') || p === 'xai' || p.includes('xai') || n.includes('grok')) return file('xai.svg')
  if (p.includes('amazon') || n.includes('bedrock') || n.includes('titan')) return file('amazon.svg')
  if (p.includes('microsoft') || p.includes('azure') || n.includes('copilot')) return file('microsoft.svg')
  if (p.includes('nvidia')) return file('nvidia.svg')
  if (p.includes('cohere')) return file('cohere.svg')
  if (p.includes('huggingface') || p.includes('hf')) return file('huggingface.svg')
  if (p.includes('perplexity')) return file('perplexity.svg')
  if (p.includes('groq')) return file('groq.svg')

  if (p.includes('submitted')) return file('other.svg')
  return file('other.svg')
}

function scatterClipId(modelId: string) {
  return `vp-${modelId.replace(/[^a-zA-Z0-9]/g, '')}`
}

const maxVoteCount = computed(() => Math.max(...rows.value.map((row) => row.voteCount), 1))
const maxElo = computed(() => Math.max(...rows.value.map((row) => row.eloRating), 1))
const minElo = computed(() => Math.min(...rows.value.map((row) => row.eloRating), 1000))
const totalVotes = computed(() => rows.value.reduce((sum, row) => sum + row.voteCount, 0))

const rankedRows = computed(() =>
  rows.value.map((row, index) => {
    const eloSpread = Math.max(maxElo.value - minElo.value, 1)
    const eloScore = ((row.eloRating - minElo.value) / eloSpread) * 100
    const usageScore = (row.voteCount / maxVoteCount.value) * 100
    const winScore = row.winRate * 100
    const momentum = Math.max(-100, Math.min(100, row.lastEloDelta * 8))
    const arenaScore = Math.round(eloScore * 0.55 + winScore * 0.3 + usageScore * 0.15)
    const reliability = Math.round(Math.min(100, row.voteCount * 12) * 0.55 + winScore * 0.45)

    return {
      ...row,
      rank: index + 1,
      arenaScore,
      reliability,
      usageShare: totalVotes.value > 0 ? row.voteCount / totalVotes.value : 0,
      usageScore,
      momentum,
      eloWidth: Math.max(6, (row.eloRating / maxElo.value) * 100),
      winRateWidth: Math.max(6, winScore),
    }
  }),
)

function estimatedBlendedCost(row: Pick<RankRow, 'displayName' | 'provider'>) {
  const official = officialArenaRowFor(row)
  if (official?.priceInput != null && official.priceOutput != null) {
    return (official.priceInput * 8 + official.priceOutput) / 9
  }

  const name = row.displayName.toLowerCase()
  const provider = row.provider.toLowerCase()

  if (name.includes('gpt-4o-mini')) return 0.2
  if (name.includes('haiku')) return 0.36
  if (name.includes('gemini') && name.includes('flash')) return 0.1
  if (provider.includes('openai')) return 1.5
  if (provider.includes('anthropic')) return 3
  if (provider.includes('google')) return 0.35
  if (provider.includes('submitted')) return 1
  return 1
}

function officialArenaRowFor(row: Pick<RankRow, 'displayName' | 'provider'>): ArenaLeaderboardRow | undefined {
  const displayName = row.displayName.toLowerCase()
  return arenaOverallRows.find((arenaRow) => {
    const platformName = arenaRow.platformDisplayName?.toLowerCase()
    const sourceName = arenaRow.sourceModel.toLowerCase()
    return platformName === displayName || arenaRow.displayName.toLowerCase() === displayName || sourceName === displayName
  })
}

function officialPriceLabel(row: Pick<RankRow, 'displayName' | 'provider'>) {
  const official = officialArenaRowFor(row)
  return official?.price && official.price !== 'N/A' ? official.price : null
}

function priceX(cost: number) {
  const min = Math.log10(priceMin)
  const max = Math.log10(priceMax)
  return chart.left + ((Math.log10(cost) - min) / (max - min)) * plotWidth
}

const eloAxis = computed(() => {
  const low = Math.floor((minElo.value - 40) / 20) * 20
  const high = Math.ceil((maxElo.value + 40) / 20) * 20
  const spread = Math.max(high - low, 80)
  return { low, high: low + spread }
})

function eloY(elo: number) {
  const { low, high } = eloAxis.value
  return chart.top + (1 - (elo - low) / (high - low)) * plotHeight
}

const priceAxisTicks = computed(() =>
  priceTicks.map((value) => ({
    value,
    x: priceX(value),
    label: `$${value}`,
  })),
)

const eloAxisTicks = computed(() => {
  const { low, high } = eloAxis.value
  const step = Math.max(20, Math.round((high - low) / 4 / 10) * 10)
  const ticks = []
  for (let value = low; value <= high; value += step) {
    ticks.push({ value, y: eloY(value) })
  }
  return ticks
})

const pricePerformanceRows = computed(() =>
  rankedRows.value.map((row) => {
    const cost = estimatedBlendedCost(row)
    /** 头像半径（SVG 单位）：略随热度变化，整体比之前圆点更小 */
    const avatarR = 9 + Math.min(2, Math.round(row.usageScore / 38))
    return {
      ...row,
      blendedCost: cost,
      priceLabel: officialPriceLabel(row) ?? `$${cost.toFixed(2)}/1M`,
      priceX: priceX(cost),
      eloY: eloY(row.eloRating),
      avatarR,
      vendorLogo: vendorLogoHref(row),
      clipId: scatterClipId(row.modelId),
      efficiencyScore: row.eloRating / cost,
    }
  }),
)

const topRows = computed(() => rankedRows.value.slice(0, 3))
const providerStats = computed(() => {
  const stats = new Map<string, { provider: string; votes: number; models: number }>()
  for (const row of rows.value) {
    const item = stats.get(row.provider) ?? { provider: row.provider, votes: 0, models: 0 }
    item.votes += row.voteCount
    item.models += 1
    stats.set(row.provider, item)
  }
  return Array.from(stats.values()).sort((a, b) => b.votes - a.votes)
})

const summaryStats = computed(() => {
  const leader = rankedRows.value[0]
  const avgWinRate = rows.value.length
    ? rows.value.reduce((sum, row) => sum + row.winRate, 0) / rows.value.length
    : 0
  return {
    leader,
    modelCount: rows.value.length,
    totalVotes: totalVotes.value,
    avgWinRate,
  }
})

const peerMatrixModels = computed(() => peerMatrix.value.models)
const peerMatrixCellMap = computed(() => {
  const map = new Map<string, PeerMatrixResponse['cells'][number]>()
  for (const cell of peerMatrix.value.cells) {
    map.set(`${cell.judgeModelId}:${cell.targetModelId}`, cell)
  }
  return map
})

function peerCell(judgeModelId: string, targetModelId: string) {
  return peerMatrixCellMap.value.get(`${judgeModelId}:${targetModelId}`) ?? null
}

function peerCellStyle(judgeModelId: string, targetModelId: string) {
  if (judgeModelId === targetModelId) {
    return { '--heat-positive': '0%', '--heat-negative': '0%' }
  }
  const cell = peerCell(judgeModelId, targetModelId)
  const score = cell?.samples ? Math.max(-1, Math.min(1, cell.score)) : 0
  const strength = cell?.samples ? 18 + Math.abs(score) * 42 : 0
  return {
    '--heat-positive': score > 0 ? `${strength.toFixed(1)}%` : '0%',
    '--heat-negative': score < 0 ? `${strength.toFixed(1)}%` : '0%',
  }
}

function peerCellLabel(judgeModelId: string, targetModelId: string) {
  if (judgeModelId === targetModelId) return '—'
  const cell = peerCell(judgeModelId, targetModelId)
  if (!cell?.samples) return '--'
  return cell.score.toFixed(2)
}

function peerCellTitle(judgeModelId: string, targetModelId: string) {
  const judge = peerMatrixModels.value.find((model) => model.modelId === judgeModelId)?.displayName ?? judgeModelId
  const target = peerMatrixModels.value.find((model) => model.modelId === targetModelId)?.displayName ?? targetModelId
  const cell = peerCell(judgeModelId, targetModelId)
  if (judgeModelId === targetModelId) return `${judge} 自评不计入`
  if (!cell?.samples) return `${judge} 对 ${target} 暂无互评样本`
  return `${judge} 对 ${target}: ${cell.score.toFixed(2)} · ${cell.samples} 样本 · 正向 ${cell.positive} / 负向 ${cell.negative} / 都好 ${cell.bothGood} / 都不好 ${cell.bothBad}`
}

const abilitySourceRows = computed(() =>
  abilityLeaderboard.value === 'overall' ? arenaOverallRows : arenaCreativeWritingRows,
)
const abilityRows = computed(() =>
  abilityScope.value === 'platform' ? abilitySourceRows.value.filter((row) => row.platformModelId) : abilitySourceRows.value,
)
const abilitySourceLabel = computed(() => (abilityLeaderboard.value === 'overall' ? '综合能力榜' : '内容创作榜'))
const abilitySummary = computed(() => {
  const leader = abilityRows.value[0]
  const totalVotes = abilityRows.value.reduce((sum, row) => sum + row.votes, 0)
  const avgScore = abilityRows.value.length
    ? abilityRows.value.reduce((sum, row) => sum + row.score, 0) / abilityRows.value.length
    : 0
  return { leader, totalVotes, avgScore, modelCount: abilityRows.value.length }
})

const abilityMaxScore = computed(() => Math.max(...abilityRows.value.map((row) => row.score), 1))
const abilityMinScore = computed(() => Math.min(...abilityRows.value.map((row) => row.score), 0))
const abilityTopRows = computed(() => abilityRowsWithMeta.value.slice(0, 3))
const abilityRowsWithMeta = computed(() =>
  abilityRows.value.map((row) => {
    const spread = Math.max(abilityMaxScore.value - abilityMinScore.value, 1)
    const blendedCost =
      row.priceInput != null && row.priceOutput != null ? (row.priceInput * 8 + row.priceOutput) / 9 : null
    return {
      ...row,
      rowId: row.platformModelId ?? row.sourceModel,
      isPlatformModel: Boolean(row.platformModelId),
      scoreWidth: Math.max(8, ((row.score - abilityMinScore.value) / spread) * 100),
      vendorLogo: vendorLogoHref(row),
      blendedCost,
    }
  }),
)

const abilityEfficiencyBaseRows = computed(() => {
  const pricedRows = abilityRowsWithMeta.value.filter(
    (row) => row.blendedCost != null && !row.vendorLogo.endsWith('assets/vendors/other.svg'),
  )
  if (abilityScope.value !== 'all') return pricedRows

  return Array.from(
    pricedRows
      .reduce((map, row) => {
        const key = row.provider.toLowerCase()
        const existing = map.get(key)
        if (!existing || row.score > existing.score) map.set(key, row)
        return map
      }, new Map<string, (typeof pricedRows)[number]>())
      .values(),
  ).sort((a, b) => b.score - a.score)
})

const abilityChartScoreAxis = computed(() => {
  const scores = abilityEfficiencyBaseRows.value.map((row) => row.score)
  if (!scores.length) return { low: 0, high: 100 }

  // 只用当前散点图里实际绘制的模型分数缩放纵轴，避免高分模型挤在顶部。
  const min = Math.min(...scores)
  const max = Math.max(...scores)
  const padding = Math.max(3, Math.round((max - min) * 0.08))
  const low = Math.floor((min - padding) / 5) * 5
  const high = Math.ceil((max + padding) / 5) * 5
  return { low, high: Math.max(high, low + 15) }
})

function abilityScoreY(score: number) {
  const { low, high } = abilityChartScoreAxis.value
  return chart.top + (1 - (score - low) / (high - low)) * plotHeight
}

const abilityEfficiencyRows = computed(() =>
  abilityEfficiencyBaseRows.value.map((row) => {
    const cost = row.blendedCost ?? 1
    return {
      ...row,
      priceX: priceX(cost),
      scoreY: abilityScoreY(row.score),
      pointR: row.isPlatformModel ? 9 : 7,
      efficiencyScore: row.score / cost,
    }
  }),
)

const hoveredAbilityPoint = computed(() =>
  abilityEfficiencyRows.value.find((row) => row.rowId === hoveredAbilityPointId.value),
)

const abilityTooltip = computed(() => {
  const row = hoveredAbilityPoint.value
  if (!row) return null
  const width = 210
  const height = 82
  const x = Math.min(chart.width - chart.right - width, Math.max(chart.left + 6, row.priceX + row.pointR + 14))
  const y = Math.min(chart.height - chart.bottom - height - 4, Math.max(chart.top + 4, row.scoreY - height / 2))
  return { row, x, y, width, height }
})

const abilityScoreTicks = computed(() => {
  const { low, high } = abilityChartScoreAxis.value
  const step = Math.max(5, Math.round((high - low) / 4 / 5) * 5)
  const ticks = []
  for (let value = low; value <= high; value += step) {
    ticks.push({ value, y: abilityScoreY(value) })
  }
  return ticks
})

function formatNumber(value: number) {
  return new Intl.NumberFormat('en-US').format(value)
}

async function load() {
  rows.value = []
  const response = await api.get<ApiEnvelope<RankRow[]>>('/rankings', {
    params: { categoryId: categoryId.value || undefined },
  })
  rows.value = response.data.data ?? []
}

async function loadPeerMatrixData() {
  peerMatrix.value = await loadPeerMatrix(categoryId.value)
}

onMounted(async () => {
  categories.value = await loadCategories()
  await Promise.all([load(), loadPeerMatrixData()])
  await updateCategoryIndicator()
})

watch(categoryId, () => {
  void load()
  void loadPeerMatrixData()
})
watch([categoryId, categories], updateCategoryIndicator)
</script>

<template>
  <div class="page ranking-page">
    <div class="header ranking-hero">
      <div>
        <p class="eyebrow">LIVE MODEL ARENA</p>
        <h1>Ranking</h1>
        <p class="muted">刘看山榜展示本地 Battle Elo；模型能力榜固定参考 Arena.ai Text Overall 榜单，不受本地投票影响。</p>
      </div>
      <div class="hero-orb" aria-hidden="true"></div>
    </div>

    <div
      class="ranking-tabs"
      role="tablist"
      aria-label="榜单切换"
      :style="{ '--seg-count': 2, '--seg-active': activeTab === 'kanshan' ? 0 : 1 }"
    >
      <button
        type="button"
        class="ranking-tab"
        :class="{ 'ranking-tab--active': activeTab === 'kanshan' }"
        role="tab"
        :aria-selected="activeTab === 'kanshan'"
        @click="activeTab = 'kanshan'"
      >
        刘看山榜
      </button>
      <button
        type="button"
        class="ranking-tab"
        :class="{ 'ranking-tab--active': activeTab === 'ability' }"
        role="tab"
        :aria-selected="activeTab === 'ability'"
        @click="activeTab = 'ability'"
      >
        模型能力榜
      </button>
    </div>

    <div v-if="activeTab === 'kanshan'" class="card ranking-filter ranking-filter--kanshan">
      <div class="ability-controls">
        <div
          ref="categorySegment"
          class="segmented-control segmented-control--wrap"
          :class="{ 'segmented-control--measured': true }"
          aria-label="分类筛选"
          :style="{ ...categoryIndicatorStyle, '--seg-count': categories.length + 1, '--seg-active': categoryActiveIndex }"
        >
          <button
            type="button"
            class="segmented-btn"
            :class="{ 'segmented-btn--active': categoryId === '' }"
            @click="categoryId = ''"
          >
            全部
          </button>
          <button
            v-for="cat in categories"
            :key="cat.id"
            type="button"
            class="segmented-btn"
            :class="{ 'segmented-btn--active': categoryId === cat.id }"
            @click="categoryId = cat.id"
          >
            {{ cat.name }}
          </button>
        </div>
      </div>
      <span class="muted ranking-filter-note">刘看山榜：综合分 = Elo 55% + 胜率 30% + 票数热度 15%</span>
    </div>

    <div v-if="activeTab === 'kanshan' && rankedRows.length" class="grid ranking-kpis">
      <div class="card metric-card">
        <span class="muted">当前冠军</span>
        <strong>{{ summaryStats.leader?.displayName }}</strong>
        <small>{{ summaryStats.leader?.provider }} · Elo {{ summaryStats.leader?.eloRating.toFixed(0) }}</small>
      </div>
      <div class="card metric-card">
        <span class="muted">总票数</span>
        <strong>{{ summaryStats.totalVotes }}</strong>
        <small>{{ summaryStats.modelCount }} 个模型参与排名</small>
      </div>
      <div class="card metric-card">
        <span class="muted">平均胜率</span>
        <strong>{{ (summaryStats.avgWinRate * 100).toFixed(1) }}%</strong>
        <small>按模型聚合后的当前表现</small>
      </div>
      <div class="card metric-card">
        <span class="muted">最高综合分</span>
        <strong>{{ summaryStats.leader?.arenaScore }}</strong>
        <small>综合 Elo、胜率与热度</small>
      </div>
    </div>
    <div v-else-if="activeTab === 'kanshan'" class="grid ranking-kpis ranking-kpis--empty">
      <div class="card metric-card metric-card--empty-dash">
        <span class="muted">当前冠军</span>
        <strong class="dash-placeholder">--</strong>
        <small class="muted">暂无数据</small>
      </div>
      <div class="card metric-card metric-card--empty-dash">
        <span class="muted">总票数</span>
        <strong class="dash-placeholder">--</strong>
        <small class="muted">暂无数据</small>
      </div>
      <div class="card metric-card metric-card--empty-dash">
        <span class="muted">平均胜率</span>
        <strong class="dash-placeholder">--</strong>
        <small class="muted">暂无数据</small>
      </div>
      <div class="card metric-card metric-card--empty-dash">
        <span class="muted">最高综合分</span>
        <strong class="dash-placeholder">--</strong>
        <small class="muted">暂无数据</small>
      </div>
    </div>

    <template v-if="activeTab === 'ability'">
      <div class="card ranking-filter ranking-filter--ability">
        <div class="ability-controls">
          <div
            class="segmented-control"
            aria-label="能力榜类型"
            :style="{ '--seg-count': 2, '--seg-active': abilityLeaderboard === 'overall' ? 0 : 1 }"
          >
            <button
              type="button"
              class="segmented-btn"
              :class="{ 'segmented-btn--active': abilityLeaderboard === 'overall' }"
              @click="abilityLeaderboard = 'overall'"
            >
              综合能力榜
            </button>
            <button
              type="button"
              class="segmented-btn"
              :class="{ 'segmented-btn--active': abilityLeaderboard === 'creativeWriting' }"
              @click="abilityLeaderboard = 'creativeWriting'"
            >
              内容创作榜
            </button>
          </div>
          <div
            class="segmented-control"
            aria-label="能力榜范围"
            :style="{ '--seg-count': 2, '--seg-active': abilityScope === 'platform' ? 0 : 1 }"
          >
            <button
              type="button"
              class="segmented-btn"
              :class="{ 'segmented-btn--active': abilityScope === 'platform' }"
              @click="abilityScope = 'platform'"
            >
              当前 7 个模型
            </button>
            <button
              type="button"
              class="segmented-btn"
              :class="{ 'segmented-btn--active': abilityScope === 'all' }"
              @click="abilityScope = 'all'"
            >
              全部模型榜单
            </button>
          </div>
        </div>
        <span class="muted">{{ abilitySourceLabel }} · 显示 {{ abilityRows.length }} / {{ abilitySourceRows.length }} 个模型</span>
      </div>

      <div class="grid ranking-kpis">
        <div class="card metric-card">
          <span class="muted">Arena 最高分</span>
          <strong>{{ abilitySummary.leader?.displayName }}</strong>
          <small>{{ abilitySummary.leader?.provider }} · {{ abilitySummary.leader?.score }}{{ abilitySummary.leader?.scoreRange }}</small>
        </div>
        <div class="card metric-card">
          <span class="muted">参考模型</span>
          <strong>{{ abilitySummary.modelCount }}</strong>
          <small>{{ abilityScope === 'platform' ? '当前平台提供模型' : 'Arena 快照全量模型' }}</small>
        </div>
        <div class="card metric-card">
          <span class="muted">总样本票数</span>
          <strong>{{ formatNumber(abilitySummary.totalVotes) }}</strong>
          <small>Arena.ai Text · {{ abilitySourceLabel }}</small>
        </div>
        <div class="card metric-card">
          <span class="muted">平均能力分</span>
          <strong>{{ abilitySummary.avgScore.toFixed(0) }}</strong>
          <small>只读静态基准，不随 Battle 变化</small>
        </div>
      </div>

      <div class="grid ranking-visuals">
        <section class="card leaderboard-panel">
          <div class="section-head">
            <div>
              <p class="eyebrow">STATIC BENCHMARK</p>
              <h2>模型能力榜 Top 3</h2>
            </div>
            <span class="muted">Arena Rank</span>
          </div>
          <div class="podium-list">
            <article v-for="row in abilityTopRows" :key="row.rowId" class="podium-card">
              <div class="rank-badge">#{{ row.rank }}</div>
              <div>
                <h3>{{ row.displayName }}</h3>
                <p class="muted">{{ row.provider }} · {{ row.sourceModel }}</p>
              </div>
              <strong>{{ row.score }}</strong>
            </article>
          </div>
        </section>

        <section class="card chart-panel ability-bar-panel" :class="{ 'ability-scroll-panel': abilityScope === 'all' }">
          <div class="section-head">
            <div>
              <p class="eyebrow">ARENA SCORE</p>
              <h2>能力分布</h2>
            </div>
            <span class="muted">越长越强</span>
          </div>
          <div class="bar-chart ability-bar-list" :class="{ 'ability-bar-list--all': abilityScope === 'all' }">
            <div v-for="row in abilityRowsWithMeta" :key="row.rowId" class="bar-row">
              <span>{{ row.displayName }}</span>
              <div class="bar-track"><div class="bar-fill" :style="{ width: `${row.scoreWidth}%` }"></div></div>
              <strong>{{ row.score }}</strong>
            </div>
          </div>
        </section>
      </div>

      <section class="card price-performance-card">
        <div class="section-head">
          <div>
            <p class="eyebrow">ABILITY / PRICE</p>
            <h2>能力价格效率图</h2>
            <p class="muted">横轴为估算综合成本（8:1 input/output，$/1M tokens，对数刻度），纵轴为 Arena 能力分；右上更强，左上更高效。</p>
          </div>
          <span class="efficiency-tag">EFFICIENT ↑ 左上角更优</span>
        </div>

        <div class="scatter-shell">
          <svg class="scatter-chart" :viewBox="`0 0 ${chart.width} ${chart.height}`" role="img" aria-label="能力价格效率图">
            <line class="axis-line" :x1="chart.left" :x2="chart.left" :y1="chart.top" :y2="chart.height - chart.bottom" />
            <line class="axis-line" :x1="chart.left" :x2="chart.width - chart.right" :y1="chart.height - chart.bottom" :y2="chart.height - chart.bottom" />

            <g v-for="tick in abilityScoreTicks" :key="`ability-score-${tick.value}`">
              <line class="grid-line" :x1="chart.left" :x2="chart.width - chart.right" :y1="tick.y" :y2="tick.y" />
              <text class="axis-label" :x="chart.left - 12" :y="tick.y + 4" text-anchor="end">{{ tick.value }}</text>
            </g>

            <g v-for="tick in priceAxisTicks" :key="`ability-price-${tick.value}`">
              <line class="grid-line grid-line--vertical" :x1="tick.x" :x2="tick.x" :y1="chart.top" :y2="chart.height - chart.bottom" />
              <text class="axis-label" :x="tick.x" :y="chart.height - 18" text-anchor="middle">{{ tick.label }}</text>
            </g>

            <text class="axis-title" :x="chart.width / 2" :y="chart.height - 2" text-anchor="middle">Blended cost $/1M tokens</text>
            <text class="axis-title axis-title--vertical" :x="18" :y="chart.height / 2" text-anchor="middle" transform="rotate(-90 18 180)">Arena Score</text>
            <text class="efficient-label" :x="chart.left + 4" :y="chart.top - 8">EFFICIENT</text>

            <g
              v-for="row in abilityEfficiencyRows"
              :key="`ability-scatter-${row.rowId}`"
              class="scatter-point ability-scatter-point"
              :class="{ 'ability-scatter-point--hovered': hoveredAbilityPointId === row.rowId }"
              @mouseenter="hoveredAbilityPointId = row.rowId"
              @mouseleave="hoveredAbilityPointId = ''"
              @focusin="hoveredAbilityPointId = row.rowId"
              @focusout="hoveredAbilityPointId = ''"
              tabindex="0"
            >
              <circle class="avatar-halo" :cx="row.priceX" :cy="row.scoreY" :r="row.pointR + 4" />
              <circle class="avatar-ring" :class="{ 'avatar-ring--platform': row.isPlatformModel }" :cx="row.priceX" :cy="row.scoreY" :r="row.pointR + 1.5" />
              <image
                class="avatar-img"
                :href="row.vendorLogo"
                :x="row.priceX - row.pointR"
                :y="row.scoreY - row.pointR"
                :width="row.pointR * 2"
                :height="row.pointR * 2"
                preserveAspectRatio="xMidYMid slice"
              />
              <text v-if="row.isPlatformModel || row.rank <= 10 || abilityScope === 'all'" class="point-label" :x="row.priceX + row.pointR + 7" :y="row.scoreY + 4">{{ row.displayName }}</text>
              <title>{{ row.displayName }} · #{{ row.rank }} · Score {{ row.score }}{{ row.scoreRange }} · {{ row.price }}</title>
            </g>

            <g v-if="abilityTooltip" class="ability-tooltip" pointer-events="none">
              <rect class="ability-tooltip-box" :x="abilityTooltip.x" :y="abilityTooltip.y" :width="abilityTooltip.width" :height="abilityTooltip.height" rx="14" />
              <image
                class="ability-tooltip-logo"
                :href="abilityTooltip.row.vendorLogo"
                :x="abilityTooltip.x + 12"
                :y="abilityTooltip.y + 18"
                width="36"
                height="36"
                preserveAspectRatio="xMidYMid slice"
              />
              <text class="ability-tooltip-title" :x="abilityTooltip.x + 58" :y="abilityTooltip.y + 24">{{ abilityTooltip.row.displayName }}</text>
              <text class="ability-tooltip-line" :x="abilityTooltip.x + 58" :y="abilityTooltip.y + 44">
                #{{ abilityTooltip.row.rank }} · Score {{ abilityTooltip.row.score }}{{ abilityTooltip.row.scoreRange }}
              </text>
              <text class="ability-tooltip-line" :x="abilityTooltip.x + 58" :y="abilityTooltip.y + 63">
                {{ abilityTooltip.row.provider }} · {{ abilityTooltip.row.price }}
              </text>
            </g>
          </svg>
        </div>
      </section>

      <div class="card ranking-table-card">
        <div class="section-head">
          <div>
            <p class="eyebrow">ARENA.AI {{ abilityLeaderboard === 'overall' ? 'OVERALL' : 'CREATIVE WRITING' }}</p>
            <h2>模型能力明细</h2>
          </div>
        </div>
        <div class="ability-table-scroll" :class="{ 'ability-table-scroll--all': abilityScope === 'all' }">
        <table class="table ranking-table ability-table">
          <thead>
            <tr><th>Arena #</th><th>模型</th><th>Provider</th><th>Score</th><th>Votes</th><th>Price $/M</th><th>Context</th><th>来源名称</th></tr>
          </thead>
          <tbody>
            <tr v-for="row in abilityRowsWithMeta" :key="row.rowId">
              <td><span class="rank-pill">#{{ row.rank }}</span></td>
              <td>
                <div class="ability-model-cell">
                  <img :src="row.vendorLogo" alt="" aria-hidden="true" />
                  <strong>{{ row.displayName }}</strong>
                </div>
              </td>
              <td>{{ row.provider }}</td>
              <td><strong>{{ row.score }}{{ row.scoreRange }}</strong></td>
              <td>{{ formatNumber(row.votes) }}</td>
              <td>{{ row.price }}</td>
              <td>{{ row.context }}</td>
              <td><code>{{ row.sourceModel }}</code></td>
            </tr>
          </tbody>
        </table>
        </div>
      </div>
    </template>

    <div v-if="activeTab === 'kanshan' && rankedRows.length" class="grid ranking-visuals">
      <section class="card leaderboard-panel">
        <div class="section-head">
          <div>
            <p class="eyebrow">TOP MODELS</p>
            <h2>综合战力</h2>
          </div>
          <span class="muted">前 3 名</span>
        </div>
        <div class="podium-list">
          <article v-for="row in topRows" :key="row.modelId" class="podium-card">
            <div class="rank-badge">#{{ row.rank }}</div>
            <div>
              <h3>{{ row.displayName }}</h3>
              <p class="muted">{{ row.provider }}</p>
            </div>
            <strong>{{ row.arenaScore }}</strong>
          </article>
        </div>
      </section>

      <section class="card chart-panel">
        <div class="section-head">
          <div>
            <p class="eyebrow">ELO CURVE</p>
            <h2>Elo 分布</h2>
          </div>
          <span class="muted">越长越强</span>
        </div>
        <div class="bar-chart">
          <div v-for="row in rankedRows" :key="row.modelId" class="bar-row">
            <span>{{ row.displayName }}</span>
            <div class="bar-track"><div class="bar-fill" :style="{ width: `${row.eloWidth}%` }"></div></div>
            <strong>{{ row.eloRating.toFixed(0) }}</strong>
          </div>
        </div>
      </section>
    </div>

    <div v-if="activeTab === 'kanshan' && rankedRows.length" class="grid ranking-visuals ranking-visuals--wide">
      <section class="card chart-panel">
        <div class="section-head">
          <div>
            <p class="eyebrow">USAGE SIGNAL</p>
            <h2>Provider 热度</h2>
          </div>
          <span class="muted">按票数聚合</span>
        </div>
        <div class="provider-grid">
          <div v-for="provider in providerStats" :key="provider.provider" class="provider-chip">
            <span>{{ provider.provider }}</span>
            <div class="ring" :style="{ '--ring': `${totalVotes ? (provider.votes / totalVotes) * 360 : 0}deg` }">
              {{ totalVotes ? ((provider.votes / totalVotes) * 100).toFixed(0) : 0 }}%
            </div>
            <small class="muted">{{ provider.models }} 模型 · {{ provider.votes }} 票</small>
          </div>
        </div>
      </section>

      <section class="card chart-panel">
        <div class="section-head">
          <div>
            <p class="eyebrow">WIN RATE</p>
            <h2>胜率雷达</h2>
          </div>
          <span class="muted">盲评胜负比</span>
        </div>
        <div class="radar-stack">
          <div v-for="row in rankedRows" :key="row.modelId" class="radar-row">
            <span>{{ row.displayName }}</span>
            <div class="radar-line"><i :style="{ width: `${row.winRateWidth}%` }"></i></div>
            <strong>{{ (row.winRate * 100).toFixed(1) }}%</strong>
          </div>
        </div>
      </section>
    </div>

    <section v-if="activeTab === 'kanshan'" class="card peer-heatmap-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">MODEL PEER EVALS</p>
          <h2>模型互评热力图</h2>
          <p class="muted">行是裁判模型，列是被评价模型；分数越高代表越偏好该模型，越低代表越不偏好。</p>
        </div>
        <span class="muted">冷启样本 {{ peerMatrix.sampleCount }}</span>
      </div>
      <div v-if="peerMatrixModels.length" class="peer-heatmap-shell">
        <div
          class="peer-heatmap-grid"
          :style="{ gridTemplateColumns: `132px repeat(${peerMatrixModels.length}, minmax(112px, 1fr))` }"
          role="grid"
          aria-label="模型互评热力图"
        >
          <div class="peer-heatmap-corner" aria-hidden="true">Judge \ Target</div>
          <div v-for="model in peerMatrixModels" :key="`col-${model.modelId}`" class="peer-heatmap-axis peer-heatmap-axis--col">
            {{ model.displayName }}
          </div>
          <template v-for="judge in peerMatrixModels" :key="`row-${judge.modelId}`">
            <div class="peer-heatmap-axis peer-heatmap-axis--row">{{ judge.displayName }}</div>
            <div
              v-for="target in peerMatrixModels"
              :key="`${judge.modelId}-${target.modelId}`"
              class="peer-heatmap-cell"
              :class="{ 'peer-heatmap-cell--self': judge.modelId === target.modelId, 'peer-heatmap-cell--empty': !peerCell(judge.modelId, target.modelId)?.samples }"
              :style="peerCellStyle(judge.modelId, target.modelId)"
              :title="peerCellTitle(judge.modelId, target.modelId)"
              role="gridcell"
            >
              {{ peerCellLabel(judge.modelId, target.modelId) }}
            </div>
          </template>
        </div>
      </div>
      <p v-else class="muted empty-state">暂无模型互评样本。生成并放置 `eval-workspace/model-peer-evals/peer_votes.jsonl` 后，服务启动会自动导入。</p>
    </section>

    <section v-if="activeTab === 'kanshan' && rankedRows.length" class="card price-performance-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">PRICE / PERFORMANCE</p>
          <h2>价格性能散点图</h2>
          <p class="muted">横轴优先使用 Arena 官方价格折算综合成本（8:1 input/output，$/1M tokens，对数刻度），纵轴为当前 Elo；标记为供应商徽标。</p>
        </div>
        <span class="efficiency-tag">EFFICIENT ↑ 左上角更优</span>
      </div>

      <div class="scatter-shell">
        <svg class="scatter-chart" :viewBox="`0 0 ${chart.width} ${chart.height}`" role="img" aria-label="价格性能散点图">
          <defs>
            <clipPath v-for="row in pricePerformanceRows" :key="'clip-' + row.modelId" :id="row.clipId">
              <circle :cx="row.priceX" :cy="row.eloY" :r="row.avatarR" />
            </clipPath>
          </defs>

          <line class="axis-line" :x1="chart.left" :x2="chart.left" :y1="chart.top" :y2="chart.height - chart.bottom" />
          <line class="axis-line" :x1="chart.left" :x2="chart.width - chart.right" :y1="chart.height - chart.bottom" :y2="chart.height - chart.bottom" />

          <g v-for="tick in eloAxisTicks" :key="`elo-${tick.value}`">
            <line class="grid-line" :x1="chart.left" :x2="chart.width - chart.right" :y1="tick.y" :y2="tick.y" />
            <text class="axis-label" :x="chart.left - 12" :y="tick.y + 4" text-anchor="end">{{ tick.value }}</text>
          </g>

          <g v-for="tick in priceAxisTicks" :key="`price-${tick.value}`">
            <line class="grid-line grid-line--vertical" :x1="tick.x" :x2="tick.x" :y1="chart.top" :y2="chart.height - chart.bottom" />
            <text class="axis-label" :x="tick.x" :y="chart.height - 18" text-anchor="middle">{{ tick.label }}</text>
          </g>

          <text class="axis-title" :x="chart.width / 2" :y="chart.height - 2" text-anchor="middle">Blended cost $/1M tokens</text>
          <text class="axis-title axis-title--vertical" :x="18" :y="chart.height / 2" text-anchor="middle" transform="rotate(-90 18 180)">Elo Rating</text>
          <text class="efficient-label" :x="chart.left + 4" :y="chart.top - 8">EFFICIENT</text>

          <g v-for="row in pricePerformanceRows" :key="`scatter-${row.modelId}`" class="scatter-point">
            <circle class="avatar-halo" :cx="row.priceX" :cy="row.eloY" :r="row.avatarR + 5" />
            <circle class="avatar-ring" :cx="row.priceX" :cy="row.eloY" :r="row.avatarR + 1.25" />
            <image
              class="avatar-img"
              :href="row.vendorLogo"
              :x="row.priceX - row.avatarR"
              :y="row.eloY - row.avatarR"
              :width="row.avatarR * 2"
              :height="row.avatarR * 2"
              :clip-path="`url(#${row.clipId})`"
              preserveAspectRatio="xMidYMid slice"
            />
            <text class="point-label" :x="row.priceX + row.avatarR + 8" :y="row.eloY + 4">{{ row.displayName }}</text>
            <title>{{ row.displayName }} · Elo {{ row.eloRating.toFixed(0) }} · {{ row.priceLabel }} · {{ row.voteCount }} 票</title>
          </g>
        </svg>
      </div>
    </section>

    <div v-if="activeTab === 'kanshan'" class="card ranking-table-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">FULL LEADERBOARD</p>
          <h2>模型排名明细</h2>
        </div>
        <span class="muted">Elo 主排序</span>
      </div>
      <table class="table ranking-table">
        <thead>
          <tr><th>#</th><th>模型</th><th>Provider</th><th>综合分</th><th>Elo</th><th>价格 $/M</th><th>胜率</th><th>热度</th><th>稳定性</th><th>最近变化</th></tr>
        </thead>
        <tbody>
          <template v-if="rankedRows.length">
            <tr v-for="row in pricePerformanceRows" :key="row.modelId + row.rank">
              <td><span class="rank-pill">#{{ row.rank }}</span></td>
              <td>
                <div class="ability-model-cell">
                  <img :src="row.vendorLogo" alt="" aria-hidden="true" />
                  <strong>{{ row.displayName }}</strong>
                </div>
              </td>
              <td>{{ row.provider }}</td>
              <td><strong>{{ row.arenaScore }}</strong></td>
              <td>{{ row.eloRating.toFixed(0) }}</td>
              <td>{{ row.priceLabel }}</td>
              <td>{{ (row.winRate * 100).toFixed(1) }}%</td>
              <td>{{ row.voteCount }} 票 · {{ (row.usageShare * 100).toFixed(0) }}%</td>
              <td>{{ row.reliability }}</td>
              <td :class="{ positive: row.lastEloDelta > 0, negative: row.lastEloDelta < 0 }">
                {{ row.lastEloDelta > 0 ? '+' : '' }}{{ row.lastEloDelta.toFixed(1) }}
              </td>
            </tr>
          </template>
          <tr v-else class="ranking-empty-row">
            <td v-for="col in 10" :key="'empty-' + col"><span class="dash-cell">--</span></td>
          </tr>
        </tbody>
      </table>
      <p v-if="!rankedRows.length" class="muted empty-state">
        当前筛选条件下暂无排名数据；可切换分类或完成对应主题的盲评后再查看。
      </p>
    </div>
  </div>
</template>

<style scoped>
.ranking-page {
  position: relative;
}

.ranking-hero {
  position: relative;
  overflow: hidden;
  padding: 4px 0 8px;
}

.eyebrow {
  margin: 0 0 6px;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.18em;
  color: var(--brand-2);
}

.hero-orb {
  position: absolute;
  right: 8%;
  top: 8px;
  width: 160px;
  height: 160px;
  border-radius: 50%;
  background:
    radial-gradient(circle at 35% 35%, color-mix(in srgb, var(--brand-2) 90%, #fff), transparent 34%),
    radial-gradient(circle, color-mix(in srgb, var(--brand-3) 70%, transparent), transparent 64%);
  filter: blur(6px);
  opacity: 0.36;
  pointer-events: none;
}

.ranking-tabs {
  display: inline-grid;
  grid-template-columns: repeat(var(--seg-count), minmax(0, 1fr));
  position: relative;
  gap: var(--seg-gap);
  padding: 6px;
  margin: 0 0 16px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: color-mix(in srgb, var(--surface-2) 64%, transparent);
  --seg-gap: 8px;
  --seg-pad: 6px;
  --seg-count: 2;
  --seg-active: 0;
}

.ranking-tabs::before {
  content: '';
  position: absolute;
  top: var(--seg-pad);
  bottom: var(--seg-pad);
  left: var(--seg-pad);
  width: calc((100% - var(--seg-pad) * 2 - var(--seg-gap) * (var(--seg-count) - 1)) / var(--seg-count));
  border-radius: 999px;
  background: linear-gradient(
    135deg,
    color-mix(in srgb, var(--brand-2) 24%, transparent),
    color-mix(in srgb, var(--brand-3) 16%, transparent)
  );
  border: 1px solid color-mix(in srgb, var(--brand-2) 42%, var(--border));
  transform: translateX(calc(var(--seg-active) * (100% + var(--seg-gap))));
  transition: transform 0.24s cubic-bezier(0.22, 1, 0.36, 1);
  pointer-events: none;
}

.ranking-tab {
  position: relative;
  z-index: 1;
  min-width: 96px;
  border: 0;
  padding: 8px 16px;
  color: var(--text-secondary);
  background: transparent !important;
  box-shadow: none !important;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 800;
}

.ranking-tab:hover {
  color: var(--text-primary);
  transform: none;
  box-shadow: none;
}

.ranking-tab--active {
  color: var(--text-primary);
}

.ranking-filter {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}

.ranking-filter--ability {
  align-items: center;
}

.ranking-filter--kanshan {
  align-items: flex-start;
  flex-direction: column;
  gap: 8px;
}

.ability-controls {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.segmented-control {
  display: inline-grid;
  grid-template-columns: repeat(var(--seg-count), minmax(0, 1fr));
  position: relative;
  gap: var(--seg-gap);
  padding: 4px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: color-mix(in srgb, var(--surface-solid) 54%, transparent);
  --seg-gap: 4px;
  --seg-pad: 4px;
  --seg-count: 2;
  --seg-active: 0;
}

.segmented-control::before {
  content: '';
  position: absolute;
  top: var(--seg-pad);
  bottom: var(--seg-pad);
  left: var(--seg-pad);
  width: calc((100% - var(--seg-pad) * 2 - var(--seg-gap) * (var(--seg-count) - 1)) / var(--seg-count));
  border-radius: 999px;
  background: color-mix(in srgb, var(--brand-2) 16%, transparent);
  border: 1px solid color-mix(in srgb, var(--brand-2) 38%, var(--border));
  transform: translateX(calc(var(--seg-active) * (100% + var(--seg-gap))));
  transition: transform 0.24s cubic-bezier(0.22, 1, 0.36, 1);
  pointer-events: none;
}

.segmented-control--wrap {
  display: inline-flex;
  flex-wrap: nowrap;
  max-width: 100%;
  overflow-x: auto;
  border-radius: 999px;
  scrollbar-width: thin;
}

.segmented-control--measured {
  --seg-indicator-width: 0px;
  --seg-indicator-x: 0px;
}

.segmented-control--measured::before {
  width: var(--seg-indicator-width);
  transform: translateX(calc(var(--seg-indicator-x) - var(--seg-pad)));
}

.segmented-btn {
  position: relative;
  z-index: 1;
  min-width: 0;
  padding: 7px 12px;
  border: 0;
  color: var(--text-secondary);
  background: transparent !important;
  box-shadow: none !important;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 800;
  white-space: nowrap;
}

.segmented-btn:hover {
  color: var(--text-primary);
  transform: none;
  box-shadow: none;
}

.segmented-btn--active {
  color: var(--text-primary);
  transition:
    color 0.18s ease;
}

.ranking-filter-note {
  display: block;
  font-size: 12px;
  line-height: 1.4;
}

.ranking-kpis {
  margin-bottom: 16px;
}

.metric-card {
  min-height: 118px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  background:
    linear-gradient(145deg, color-mix(in srgb, var(--surface) 88%, transparent), color-mix(in srgb, var(--surface-2) 70%, transparent)),
    radial-gradient(circle at 90% 20%, color-mix(in srgb, var(--brand-2) 18%, transparent), transparent 35%);
}

.metric-card strong {
  font-size: 2rem;
  line-height: 1.1;
  letter-spacing: -0.04em;
}

.metric-card small {
  color: var(--text-secondary);
}

.ranking-visuals {
  align-items: stretch;
  margin-bottom: 16px;
}

.ranking-visuals--wide {
  grid-template-columns: 1fr 1fr;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}

.section-head h2 {
  margin: 0;
}

.podium-list {
  display: grid;
  gap: 12px;
}

.podium-card {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 14px;
  padding: 14px;
  border-radius: 18px;
  border: 1px solid color-mix(in srgb, var(--brand-2) 28%, var(--border));
  background: linear-gradient(135deg, color-mix(in srgb, var(--surface-2) 78%, transparent), color-mix(in srgb, var(--brand-2) 10%, transparent));
}

.podium-card h3,
.podium-card p {
  margin: 0;
}

.rank-badge,
.rank-pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--brand-2) 45%, var(--border));
  background: color-mix(in srgb, var(--brand-2) 12%, transparent);
  color: var(--brand-2);
  font-weight: 800;
}

.rank-badge {
  width: 42px;
  height: 42px;
}

.rank-pill {
  min-width: 42px;
  padding: 5px 9px;
}

.bar-chart,
.radar-stack {
  display: grid;
  gap: 12px;
}

.ability-scroll-panel {
  max-height: min(680px, 72vh);
  overflow: hidden;
}

.ability-bar-list--all {
  max-height: calc(min(680px, 72vh) - 96px);
  overflow-y: auto;
  padding-right: 6px;
}

.bar-row,
.radar-row {
  display: grid;
  grid-template-columns: minmax(120px, 1.2fr) 2fr auto;
  align-items: center;
  gap: 12px;
  font-size: 13px;
}

.bar-track,
.radar-line {
  height: 10px;
  overflow: hidden;
  border-radius: 999px;
  background: color-mix(in srgb, var(--surface-2) 82%, transparent);
  border: 1px solid var(--border);
}

.bar-fill,
.radar-line i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, var(--brand-2), var(--brand-3));
  box-shadow: 0 0 18px color-mix(in srgb, var(--brand-2) 36%, transparent);
}

.provider-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 12px;
}

.provider-chip {
  display: grid;
  justify-items: center;
  gap: 8px;
  padding: 14px 10px;
  border-radius: 18px;
  border: 1px solid var(--border);
  background: color-mix(in srgb, var(--surface-2) 70%, transparent);
}

.ring {
  --ring: 0deg;
  width: 78px;
  height: 78px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  font-weight: 800;
  background:
    radial-gradient(circle, var(--surface) 0 55%, transparent 56%),
    conic-gradient(var(--brand-2) var(--ring), color-mix(in srgb, var(--surface-2) 70%, transparent) 0);
  border: 1px solid var(--border);
}

.peer-heatmap-card {
  margin-bottom: 16px;
}

.peer-heatmap-card .section-head {
  align-items: flex-start;
}

.peer-heatmap-card .section-head p.muted {
  max-width: 720px;
  margin: 6px 0 0;
}

.peer-heatmap-shell {
  overflow: auto;
  border-radius: 18px;
  border: 1px solid var(--border);
  background:
    linear-gradient(color-mix(in srgb, var(--surface) 82%, transparent), color-mix(in srgb, var(--surface) 82%, transparent)),
    repeating-linear-gradient(90deg, color-mix(in srgb, var(--border) 42%, transparent) 0 1px, transparent 1px 68px),
    repeating-linear-gradient(0deg, color-mix(in srgb, var(--border) 36%, transparent) 0 1px, transparent 1px 48px);
}

.peer-heatmap-grid {
  display: grid;
  min-width: 920px;
}

.peer-heatmap-corner,
.peer-heatmap-axis,
.peer-heatmap-cell {
  min-height: 46px;
  display: grid;
  place-items: center;
  padding: 8px;
  border-right: 1px solid color-mix(in srgb, var(--border) 78%, transparent);
  border-bottom: 1px solid color-mix(in srgb, var(--border) 78%, transparent);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.peer-heatmap-corner,
.peer-heatmap-axis {
  position: sticky;
  z-index: 2;
  background: color-mix(in srgb, var(--surface-solid) 92%, transparent);
  color: var(--text-secondary);
  font-weight: 800;
}

.peer-heatmap-corner {
  left: 0;
  top: 0;
  z-index: 4;
}

.peer-heatmap-axis--col {
  top: 0;
  text-align: center;
  line-height: 1.25;
  word-break: normal;
}

.peer-heatmap-axis--row {
  left: 0;
  justify-items: start;
  text-align: left;
  line-height: 1.25;
}

.peer-heatmap-cell {
  --heat-positive: 0%;
  --heat-negative: 0%;
  color: var(--text-primary);
  font-weight: 800;
  background:
    linear-gradient(
      135deg,
      color-mix(in srgb, var(--brand-2) var(--heat-positive), transparent),
      color-mix(in srgb, var(--danger) var(--heat-negative), transparent)
    ),
    color-mix(in srgb, var(--surface-2) 64%, transparent);
}

.peer-heatmap-cell--empty {
  color: var(--text-secondary);
  font-weight: 650;
}

.peer-heatmap-cell--self {
  color: color-mix(in srgb, var(--text-secondary) 72%, transparent);
  background:
    repeating-linear-gradient(135deg, color-mix(in srgb, var(--border) 40%, transparent) 0 6px, transparent 6px 12px),
    color-mix(in srgb, var(--surface-2) 58%, transparent);
}

.price-performance-card {
  margin-bottom: 16px;
  background:
    linear-gradient(145deg, color-mix(in srgb, var(--surface) 90%, transparent), color-mix(in srgb, var(--surface-2) 72%, transparent)),
    radial-gradient(circle at 16% 18%, color-mix(in srgb, var(--brand-2) 15%, transparent), transparent 32%),
    radial-gradient(circle at 82% 24%, color-mix(in srgb, var(--brand-3) 13%, transparent), transparent 28%);
}

.price-performance-card .section-head {
  align-items: flex-start;
}

.price-performance-card .section-head p.muted {
  max-width: 760px;
  margin: 6px 0 0;
}

.efficiency-tag {
  display: inline-flex;
  white-space: nowrap;
  padding: 7px 11px;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--brand-2) 40%, var(--border));
  color: var(--brand-2);
  background: color-mix(in srgb, var(--brand-2) 10%, transparent);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.08em;
}

.scatter-shell {
  overflow-x: auto;
  border-radius: 18px;
  border: 1px solid var(--border);
  background:
    linear-gradient(color-mix(in srgb, var(--surface) 74%, transparent), color-mix(in srgb, var(--surface) 74%, transparent)),
    repeating-linear-gradient(90deg, color-mix(in srgb, var(--border) 42%, transparent) 0 1px, transparent 1px 76px),
    repeating-linear-gradient(0deg, color-mix(in srgb, var(--border) 36%, transparent) 0 1px, transparent 1px 56px);
}

.scatter-chart {
  display: block;
  min-width: 720px;
  width: 100%;
  height: auto;
}

.axis-line {
  stroke: color-mix(in srgb, var(--text-secondary) 46%, transparent);
  stroke-width: 1.2;
}

.grid-line {
  stroke: color-mix(in srgb, var(--border) 78%, transparent);
  stroke-width: 1;
}

.grid-line--vertical {
  stroke-dasharray: 3 7;
}

.axis-label,
.axis-title,
.efficient-label {
  fill: var(--text-secondary);
  font-size: 11px;
}

.axis-title,
.efficient-label {
  font-weight: 800;
  letter-spacing: 0.08em;
}

.efficient-label {
  fill: var(--brand-2);
}

.scatter-point {
  cursor: default;
}

.ability-scatter-point {
  cursor: pointer;
  outline: none;
}

.ability-scatter-point .avatar-halo,
.ability-scatter-point .avatar-ring,
.ability-scatter-point .avatar-img,
.ability-scatter-point .point-label {
  transition:
    opacity 0.16s ease,
    transform 0.16s ease,
    fill 0.16s ease,
    stroke 0.16s ease;
  transform-box: fill-box;
  transform-origin: center;
}

.ability-scatter-point--hovered .avatar-halo {
  fill: color-mix(in srgb, var(--brand-2) 28%, transparent);
  transform: scale(1.55);
}

.ability-scatter-point--hovered .avatar-ring,
.ability-scatter-point:focus-visible .avatar-ring {
  stroke: color-mix(in srgb, var(--brand) 72%, var(--brand-2));
  stroke-width: 2.8;
  transform: scale(1.55);
}

.ability-scatter-point--hovered .avatar-img,
.ability-scatter-point:focus-visible .avatar-img {
  transform: scale(1.55);
}

.ability-scatter-point--hovered .point-label {
  font-size: 12px;
}

.scatter-point:hover .avatar-ring {
  stroke: color-mix(in srgb, var(--brand-2) 65%, var(--border));
  stroke-width: 2;
}

.scatter-point:hover .avatar-halo {
  fill: color-mix(in srgb, var(--brand-2) 14%, transparent);
}

.avatar-halo {
  fill: color-mix(in srgb, var(--brand-2) 8%, transparent);
  filter: blur(0.6px);
  pointer-events: none;
}

.avatar-ring {
  fill: none;
  stroke: color-mix(in srgb, var(--border) 88%, transparent);
  stroke-width: 1.25;
  pointer-events: none;
}

.avatar-ring--platform {
  stroke: color-mix(in srgb, var(--brand) 64%, var(--brand-2));
  stroke-width: 2;
}

.avatar-img {
  pointer-events: none;
}

.ability-tooltip-box {
  fill: color-mix(in srgb, var(--surface-solid) 92%, transparent);
  stroke: color-mix(in srgb, var(--brand-2) 42%, var(--border));
  stroke-width: 1;
  filter: drop-shadow(0 14px 30px color-mix(in srgb, #000 22%, transparent));
}

.ability-tooltip-logo {
  opacity: 0.98;
}

.ability-tooltip-title {
  fill: var(--text-primary);
  font-size: 12px;
  font-weight: 800;
}

.ability-tooltip-line {
  fill: var(--text-secondary);
  font-size: 11px;
  font-weight: 600;
}

.ability-point {
  fill: color-mix(in srgb, var(--brand-3) 68%, var(--brand-2));
  stroke: color-mix(in srgb, var(--surface-solid) 86%, transparent);
  stroke-width: 1.2;
}

.ability-point--platform {
  fill: var(--brand-2);
  stroke: color-mix(in srgb, var(--brand) 60%, var(--surface-solid));
  stroke-width: 2;
}

.point-label {
  font-size: 11px;
  font-weight: 700;
  fill: var(--text-primary);
  paint-order: stroke;
  stroke: var(--surface);
  stroke-width: 4px;
  stroke-linejoin: round;
}

.ranking-table-card {
  overflow-x: auto;
}

.ability-table-scroll {
  overflow-x: auto;
}

.ability-table-scroll--all {
  max-height: min(720px, 74vh);
  overflow: auto;
  border-radius: 16px;
  border: 1px solid var(--border);
}

.ability-table-scroll--all .ability-table thead {
  position: sticky;
  top: 0;
  z-index: 5;
}

.ability-table-scroll--all .ability-table thead th {
  position: sticky;
  top: 0;
  z-index: 6;
  background: color-mix(in srgb, var(--surface-solid) 94%, transparent);
  backdrop-filter: blur(10px);
  box-shadow: 0 1px 0 var(--border), 0 10px 18px color-mix(in srgb, #000 8%, transparent);
}

.ranking-table {
  min-width: 1040px;
}

.ability-table {
  min-width: 980px;
  border-collapse: separate;
  border-spacing: 0;
}

.ability-model-cell {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.ability-model-cell img {
  width: 22px;
  height: 22px;
  border-radius: 999px;
  border: 1px solid var(--border);
  background: color-mix(in srgb, var(--surface-solid) 82%, transparent);
}

.ability-table code {
  font-size: 12px;
  color: var(--text-secondary);
}

.positive {
  color: var(--brand-2);
  font-weight: 700;
}

.negative {
  color: #ff6b6b;
  font-weight: 700;
}

.empty-state {
  margin: 0;
  padding: 18px 0 4px;
}

.dash-placeholder,
.dash-cell {
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.04em;
  color: var(--text-secondary);
}

.metric-card--empty-dash strong.dash-placeholder {
  font-size: 1.35rem;
  font-weight: 750;
}

.ranking-empty-row td {
  text-align: center;
  padding: 20px 10px;
  color: var(--text-secondary);
}

@media (max-width: 760px) {
  .ranking-filter,
  .ranking-tabs,
  .section-head {
    align-items: flex-start;
    flex-direction: column;
  }

  .ranking-tabs {
    display: flex;
    width: 100%;
    border-radius: 18px;
  }

  .ability-controls,
  .segmented-control {
    width: 100%;
  }

  .ranking-tab {
    width: 100%;
  }

  .segmented-btn {
    flex: 1;
  }

  .ranking-visuals--wide {
    grid-template-columns: 1fr;
  }

  .bar-row,
  .radar-row {
    grid-template-columns: 1fr;
    gap: 6px;
  }

  .efficiency-tag {
    white-space: normal;
  }
}
</style>
