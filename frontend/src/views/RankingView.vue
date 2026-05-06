<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api, loadCategories, type ApiEnvelope, type Category } from '../api'

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

const categories = ref<Category[]>([])
const categoryId = ref('')
const rows = ref<RankRow[]>([])

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

async function load() {
  rows.value = []
  const response = await api.get<ApiEnvelope<RankRow[]>>('/rankings', {
    params: { categoryId: categoryId.value || undefined },
  })
  rows.value = response.data.data ?? []
}

onMounted(async () => {
  categories.value = await loadCategories()
  await load()
})

watch(categoryId, load)
</script>

<template>
  <div class="page ranking-page">
    <div class="header ranking-hero">
      <div>
        <p class="eyebrow">LIVE MODEL ARENA</p>
        <h1>Ranking</h1>
        <p class="muted">参考多维 Leaderboard 与真实使用排行的呈现方式，用本地盲评数据展示 Elo、胜率、热度、稳定性和近期动量。</p>
      </div>
      <div class="hero-orb" aria-hidden="true"></div>
    </div>

    <div class="card ranking-filter">
      <label>分类筛选<select v-model="categoryId"><option value="">全部</option><option v-for="cat in categories" :key="cat.id" :value="cat.id">{{ cat.name }}</option></select></label>
      <span class="muted">综合分 = Elo 55% + 胜率 30% + 票数热度 15%</span>
    </div>

    <div v-if="rankedRows.length" class="grid ranking-kpis">
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
    <div v-else class="grid ranking-kpis ranking-kpis--empty">
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

    <div v-if="rankedRows.length" class="grid ranking-visuals">
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

    <div v-if="rankedRows.length" class="grid ranking-visuals ranking-visuals--wide">
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

    <section v-if="rankedRows.length" class="card price-performance-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">PRICE / PERFORMANCE</p>
          <h2>价格性能散点图</h2>
          <p class="muted">横轴为估算综合成本（8:1 input/output，$/1M tokens，对数刻度），纵轴为当前 Elo；标记为供应商徽标，大小略随本地投票热度变化。</p>
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
            <title>{{ row.displayName }} · Elo {{ row.eloRating.toFixed(0) }} · ${{ row.blendedCost.toFixed(2) }}/1M · {{ row.voteCount }} 票</title>
          </g>
        </svg>
      </div>
    </section>

    <div class="card ranking-table-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">FULL LEADERBOARD</p>
          <h2>模型排名明细</h2>
        </div>
        <span class="muted">Elo 主排序</span>
      </div>
      <table class="table ranking-table">
        <thead>
          <tr><th>#</th><th>模型</th><th>Provider</th><th>综合分</th><th>Elo</th><th>估算成本</th><th>胜率</th><th>热度</th><th>稳定性</th><th>最近变化</th></tr>
        </thead>
        <tbody>
          <template v-if="rankedRows.length">
            <tr v-for="row in pricePerformanceRows" :key="row.modelId + row.rank">
              <td><span class="rank-pill">#{{ row.rank }}</span></td>
              <td><strong>{{ row.displayName }}</strong></td>
              <td>{{ row.provider }}</td>
              <td><strong>{{ row.arenaScore }}</strong></td>
              <td>{{ row.eloRating.toFixed(0) }}</td>
              <td>${{ row.blendedCost.toFixed(2) }}/1M</td>
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

.ranking-filter {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
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

.avatar-img {
  pointer-events: none;
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

.ranking-table {
  min-width: 1040px;
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
  .section-head {
    align-items: flex-start;
    flex-direction: column;
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
