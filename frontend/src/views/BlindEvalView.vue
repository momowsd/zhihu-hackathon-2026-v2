<script setup lang="ts">
import type { Ref } from 'vue'
import { computed, nextTick, onMounted, ref, useTemplateRef, watch } from 'vue'
import { api, errorMessage, loadCategories, loadUserHistory, type ApiEnvelope, type Category, type UserHistoryItem } from '../api'
import { useRoute } from 'vue-router'
import KanshanMascot from '../components/KanshanMascot.vue'

type VoteOutcome = 'left' | 'right' | 'both_good' | 'both_bad'
type VoteModelEffect = {
  side: 'left' | 'right'
  modelId: string
  eloBefore: number
  eloAfter: number
  eloDelta: number
  rankBefore: number
  rankAfter: number
  rankDelta: number
}
type VoteEffect = {
  categoryId: string
  models: VoteModelEffect[]
}

type Session = { id: string; requestedCount: number; desiredCount?: number }
type SessionItemRow = {
  itemId: string
  position: number
  question: { id: string; prompt: string }
  left: { answerId: string; text: string; modelId?: string; modelName?: string }
  right: { answerId: string; text: string; modelId?: string; modelName?: string }
  voted: boolean
  /** 新接口：四档之一；旧数据可能仅有 winnerSide */
  outcome?: VoteOutcome
  winnerSide?: 'left' | 'right'
  confidenceScore?: number
  voteEffect?: VoteEffect
}
type SessionItemsPayload = {
  sessionId: string
  desiredCount?: number
  requestedCount: number
  items: SessionItemRow[]
}
type VoteResponse = {
  effect?: VoteEffect
}

const categories = ref<Category[]>([])
const categoryId = ref('')
const count = ref(5)
const session = ref<Session | null>(null)
const sessionItems = ref<SessionItemRow[]>([])
const currentIndex = ref(0)
const error = ref('')
const route = useRoute()
const setupCategorySegmentRef = useTemplateRef<HTMLElement>('setupCategorySegment')
const setupCategoryIndicator = ref({ width: 0, x: 0 })

/** 悬停在某档评分按钮上时，用于两侧卡片边框预览 */
const ratingHover = ref<VoteOutcome | null>(null)
const voteSubmitting = ref(false)

const currentRow = computed(() => sessionItems.value[currentIndex.value] ?? null)
const selectedCategory = computed(() => categories.value.find((cat) => cat.id === categoryId.value) ?? null)
const selectedSystemPromptHtml = computed(() => markdownToHtml(selectedCategory.value?.systemPromptMd ?? ''))
const leftTypedHtml = computed(() => markdownToHtml(leftTyped.value))
const rightTypedHtml = computed(() => markdownToHtml(rightTyped.value))
const setupCategoryActiveIndex = computed(() => {
  const index = categories.value.findIndex((cat) => cat.id === categoryId.value)
  return index >= 0 ? index : 0
})
const setupCategoryIndicatorStyle = computed(() => ({
  '--setup-seg-indicator-width': `${setupCategoryIndicator.value.width}px`,
  '--setup-seg-indicator-x': `${setupCategoryIndicator.value.x}px`,
}))

/** 盲评：先题干打字结束，再两侧回答并行打字（切换题目时取消上一轮） */
const questionTyped = ref('')
const leftTyped = ref('')
const rightTyped = ref('')
let typewriterRound = 0

const TYPEWRITER_FRAME_MS = 16

function sleep(ms: number) {
  return new Promise<void>((resolve) => {
    setTimeout(resolve, ms)
  })
}

function escapeHtml(value: string) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function inlineMarkdown(value: string) {
  return escapeHtml(value)
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/`(.+?)`/g, '<code>$1</code>')
}

function markdownToHtml(markdown: string) {
  const lines = markdown.trim().split(/\r?\n/)
  const html: string[] = []
  let listOpen = false
  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed) {
      if (listOpen) {
        html.push('</ul>')
        listOpen = false
      }
      continue
    }
    if (trimmed.startsWith('- ') || trimmed.startsWith('* ')) {
      if (!listOpen) {
        html.push('<ul>')
        listOpen = true
      }
      html.push(`<li>${inlineMarkdown(trimmed.slice(2))}</li>`)
      continue
    }
    const orderedListMatch = trimmed.match(/^\d+\.\s+(.+)$/)
    if (orderedListMatch) {
      if (!listOpen) {
        html.push('<ul>')
        listOpen = true
      }
      html.push(`<li>${inlineMarkdown(orderedListMatch[1])}</li>`)
      continue
    }
    if (listOpen) {
      html.push('</ul>')
      listOpen = false
    }
    if (trimmed.startsWith('### ')) html.push(`<h4>${inlineMarkdown(trimmed.slice(4))}</h4>`)
    else if (trimmed.startsWith('## ')) html.push(`<h3>${inlineMarkdown(trimmed.slice(3))}</h3>`)
    else if (trimmed.startsWith('# ')) html.push(`<h3>${inlineMarkdown(trimmed.slice(2))}</h3>`)
    else html.push(`<p>${inlineMarkdown(trimmed)}</p>`)
  }
  if (listOpen) html.push('</ul>')
  return html.join('')
}

async function updateSetupCategoryIndicator() {
  await nextTick()
  const el = setupCategorySegmentRef.value
  const active = el?.querySelector<HTMLElement>('.battle-setup-seg-btn--active')
  if (!el || !active) return
  setupCategoryIndicator.value = { width: active.offsetWidth, x: active.offsetLeft }
}

function streamChunkSize(full: string, kind: 'question' | 'answer') {
  const length = Array.from(full).length
  if (length <= 0) return 1
  const targetDuration = kind === 'question'
    ? Math.min(Math.max(length * 18, 420), 1200)
    : Math.min(Math.max(length * 8, 900), 3600)
  const frames = Math.max(1, Math.round(targetDuration / TYPEWRITER_FRAME_MS))
  return Math.max(1, Math.ceil(length / frames))
}

async function streamInto(target: Ref<string>, full: string, round: number, kind: 'question' | 'answer') {
  const chars = Array.from(full)
  const chunkSize = streamChunkSize(full, kind)
  for (let index = 0; index < chars.length; index += chunkSize) {
    if (round !== typewriterRound) return
    target.value += chars.slice(index, index + chunkSize).join('')
    await sleep(TYPEWRITER_FRAME_MS)
  }
}

watch(
  () => currentRow.value,
  async (row) => {
    typewriterRound += 1
    const round = typewriterRound
    questionTyped.value = ''
    leftTyped.value = ''
    rightTyped.value = ''
    ratingHover.value = null
    if (!row) return

    const instant =
      typeof window !== 'undefined' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches

    if (instant || row.voted) {
      questionTyped.value = row.question.prompt
      leftTyped.value = row.left.text
      rightTyped.value = row.right.text
      return
    }

    await streamInto(questionTyped, row.question.prompt, round, 'question')
    if (round !== typewriterRound) return

    await Promise.all([
      streamInto(leftTyped, row.left.text, round, 'answer'),
      streamInto(rightTyped, row.right.text, round, 'answer'),
    ])
  },
  { immediate: true },
)

const cursorQuestion = computed(() => {
  const row = currentRow.value
  if (!row || row.voted) return false
  return questionTyped.value.length < row.question.prompt.length
})

const cursorLeft = computed(() => {
  const row = currentRow.value
  if (!row || row.voted) return false
  if (questionTyped.value.length < row.question.prompt.length) return false
  return leftTyped.value.length < row.left.text.length
})

const cursorRight = computed(() => {
  const row = currentRow.value
  if (!row || row.voted) return false
  if (questionTyped.value.length < row.question.prompt.length) return false
  return rightTyped.value.length < row.right.text.length
})

const allVoted = computed(
  () => sessionItems.value.length > 0 && sessionItems.value.every((r) => r.voted),
)
const completedRows = computed(() => sessionItems.value.filter((row) => row.voted))
const sessionEffectSummary = computed(() => {
  const byModel = new Map<
    string,
    { modelId: string; modelName: string; eloDelta: number; rankDelta: number; rankBefore: number; rankAfter: number; count: number }
  >()
  for (const item of completedRows.value) {
    for (const effect of item.voteEffect?.models ?? []) {
      const side = effect.side === 'left' ? item.left : item.right
      const modelName = side.modelName ?? effect.modelId
      const current =
        byModel.get(effect.modelId) ??
        {
          modelId: effect.modelId,
          modelName,
          eloDelta: 0,
          rankDelta: 0,
          rankBefore: effect.rankBefore,
          rankAfter: effect.rankAfter,
          count: 0,
        }
      current.eloDelta += effect.eloDelta
      current.rankDelta += effect.rankDelta
      current.rankAfter = effect.rankAfter
      current.count += 1
      byModel.set(effect.modelId, current)
    }
  }
  return Array.from(byModel.values()).sort((a, b) => Math.abs(b.eloDelta) - Math.abs(a.eloDelta))
})

const canGoPrev = computed(() => currentIndex.value > 0)
const canGoNext = computed(() => currentIndex.value < sessionItems.value.length - 1)

function resolvedOutcome(row: SessionItemRow): VoteOutcome | null {
  if (!row.voted) return null
  if (row.outcome) return row.outcome
  if (row.winnerSide === 'left') return 'left'
  if (row.winnerSide === 'right') return 'right'
  return null
}

function outcomeLabel(outcome: VoteOutcome | null) {
  if (outcome === 'left') return 'A 更好'
  if (outcome === 'right') return 'B 更好'
  if (outcome === 'both_good') return '都好'
  if (outcome === 'both_bad') return '都不好'
  return '未记录'
}

function winnerText(row: SessionItemRow) {
  const outcome = resolvedOutcome(row)
  if (outcome === 'left') return modelLabel(row, 'left')
  if (outcome === 'right') return modelLabel(row, 'right')
  if (outcome === 'both_good') return [modelLabel(row, 'left'), modelLabel(row, 'right')].join('、')
  if (outcome === 'both_bad') return '无胜出模型'
  return '未记录'
}

function modelLabel(row: SessionItemRow, side: 'left' | 'right') {
  const answer = side === 'left' ? row.left : row.right
  return answer.modelName ?? answer.modelId ?? (side === 'left' ? '模型 A' : '模型 B')
}

function mergeHistoryItem(row: SessionItemRow, history: UserHistoryItem): SessionItemRow {
  const outcome = history.outcome || row.outcome
  return {
    ...row,
    question: { id: history.questionId || row.question.id, prompt: history.questionPrompt || row.question.prompt },
    left: {
      answerId: history.leftAnswerId || row.left.answerId,
      text: history.leftAnswerText || row.left.text,
      modelId: history.leftModelId || row.left.modelId,
      modelName: history.leftModelName || row.left.modelName,
    },
    right: {
      answerId: history.rightAnswerId || row.right.answerId,
      text: history.rightAnswerText || row.right.text,
      modelId: history.rightModelId || row.right.modelId,
      modelName: history.rightModelName || row.right.modelName,
    },
    outcome: outcome || undefined,
    winnerSide: outcome === 'left' ? 'left' : outcome === 'right' ? 'right' : row.winnerSide,
    voted: true,
  }
}

async function enrichCompletedSessionFromHistory(sessionId: string) {
  const response = await loadUserHistory(1, 200)
  const byItem = new Map(response.items.filter((item) => item.sessionId === sessionId).map((item) => [item.itemId, item]))
  if (!byItem.size) return
  sessionItems.value = sessionItems.value.map((row) => {
    const history = byItem.get(row.itemId)
    return history ? mergeHistoryItem(row, history) : row
  })
}

function signedNumber(value: number, digits = 1) {
  const fixed = Math.abs(value).toFixed(digits)
  if (value > 0) return `+${fixed}`
  if (value < 0) return `-${fixed}`
  return `0.${'0'.repeat(digits)}`
}

function rankDeltaLabel(value: number) {
  if (value > 0) return `↑${value}`
  if (value < 0) return `↓${Math.abs(value)}`
  return '持平'
}

/** 左侧卡片边框：good=绿, bad=红 */
function leftCardAccent(): 'none' | 'good' | 'bad' {
  const row = currentRow.value
  if (!row) return 'none'
  const h = ratingHover.value
  if (!row.voted) {
    if (h === 'left' || h === 'both_good') return 'good'
    if (h === 'both_bad') return 'bad'
    return 'none'
  }
  const o = resolvedOutcome(row)
  if (o === 'left' || o === 'both_good') return 'good'
  if (o === 'both_bad') return 'bad'
  return 'none'
}

function rightCardAccent(): 'none' | 'good' | 'bad' {
  const row = currentRow.value
  if (!row) return 'none'
  const h = ratingHover.value
  if (!row.voted) {
    if (h === 'right' || h === 'both_good') return 'good'
    if (h === 'both_bad') return 'bad'
    return 'none'
  }
  const o = resolvedOutcome(row)
  if (o === 'right' || o === 'both_good') return 'good'
  if (o === 'both_bad') return 'bad'
  return 'none'
}

onMounted(async () => {
  categories.value = await loadCategories()
  categoryId.value = categories.value[0]?.id ?? ''
  await updateSetupCategoryIndicator()
  const arenaId = route.query.arenaSessionId
  if (typeof arenaId === 'string' && arenaId) {
    try {
      await loadSessionItems(arenaId)
    } catch (err) {
      error.value = errorMessage(err)
    }
  }
})

watch([categoryId, categories], updateSetupCategoryIndicator)

async function loadSessionItems(sessionId: string, options: { keepIndex?: boolean } = {}) {
  const previousIndex = currentIndex.value
  const voteEffectByItem = new Map(sessionItems.value.map((item) => [item.itemId, item.voteEffect]))
  const response = await api.get<ApiEnvelope<SessionItemsPayload>>(`/eval/sessions/${sessionId}/items`)
  const data = response.data.data
  session.value = {
    id: data.sessionId,
    requestedCount: data.requestedCount,
    desiredCount: data.desiredCount,
  }
  sessionItems.value = data.items.map((item) => ({
    ...item,
    voteEffect: item.voteEffect ?? voteEffectByItem.get(item.itemId),
  }))
  currentIndex.value = options.keepIndex ? Math.min(previousIndex, Math.max(0, data.items.length - 1)) : 0
  if (sessionItems.value.length > 0 && sessionItems.value.every((item) => item.voted)) {
    await enrichCompletedSessionFromHistory(data.sessionId)
  }
  error.value = ''
}

async function start() {
  error.value = ''
  try {
    const response = await api.post<ApiEnvelope<Session>>('/eval/sessions', { categoryId: categoryId.value, count: count.value })
    await loadSessionItems(response.data.data.id)
  } catch (err) {
    error.value = errorMessage(err)
  }
}

function goPrev() {
  if (!canGoPrev.value) return
  currentIndex.value -= 1
}

function goNextNav() {
  if (!canGoNext.value) return
  currentIndex.value += 1
}

async function submitVote(outcome: VoteOutcome) {
  const row = currentRow.value
  if (!row?.itemId || row.voted || voteSubmitting.value) return
  error.value = ''
  voteSubmitting.value = true
  try {
    const response = await api.post<ApiEnvelope<VoteResponse>>('/eval/votes', {
      itemId: row.itemId,
      outcome,
    })
    row.voted = true
    row.outcome = outcome
    if (outcome === 'left') row.winnerSide = 'left'
    else if (outcome === 'right') row.winnerSide = 'right'
    else row.winnerSide = undefined
    row.voteEffect = response.data.data.effect

    const lastIdx = sessionItems.value.length - 1
    if (currentIndex.value < lastIdx) {
      currentIndex.value += 1
    } else if (session.value?.id) {
      await loadSessionItems(session.value.id, { keepIndex: true })
      await enrichCompletedSessionFromHistory(session.value.id)
    }
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    voteSubmitting.value = false
  }
}

function resetRound() {
  session.value = null
  sessionItems.value = []
  currentIndex.value = 0
  error.value = ''
  ratingHover.value = null
}

function progressLabel() {
  const total = sessionItems.value.length
  if (!total) return ''
  return `第 ${currentIndex.value + 1} / ${total} 题`
}

function ratingBtnClass(outcome: VoteOutcome) {
  const row = currentRow.value
  if (!row?.voted) return {}
  const cur = resolvedOutcome(row)
  return {
    'battle-rating-btn--active-a': outcome === 'left' && cur === 'left',
    'battle-rating-btn--active-b': outcome === 'right' && cur === 'right',
    'battle-rating-btn--active-both-good': outcome === 'both_good' && cur === 'both_good',
    'battle-rating-btn--active-both-bad': outcome === 'both_bad' && cur === 'both_bad',
  }
}

/** 题干旁 mascot：四视图精灵固定「侧视」一格（原 battle UI 默认态为 frame 1） */
const BATTLE_Q_SPRITE_FRAME = 1

const battleHostLine = computed(() => {
  if (!session.value) return '挑个主题，点「开始」，我就陪你进 Battle。'
  const row = currentRow.value
  if (!row) return ''
  if (voteSubmitting.value) return '正在把你的选择记到服务器上…'
  if (row.voted) return '收到，这一题交卷啦。'
  if (ratingHover.value === 'both_bad') return '两边都不太满意？诚实选「都不好」也很有价值。'
  if (ratingHover.value === 'both_good') return '难分高下？「都好」会记成平局 Elo。'
  if (ratingHover.value === 'left') return '更倾向 A 这一侧？'
  if (ratingHover.value === 'right') return '更倾向 B 这一侧？'
  if (cursorQuestion.value) return '这一题我来出题——先把题干读完，我不催你。'
  if (cursorLeft.value || cursorRight.value) return '两边的回答在慢慢展开，别急。'
  return '两边都扫完了？在下面四档里点最贴近你感受的一档。'
})
</script>

<template>
  <div class="page blind-eval">
    <div class="header">
      <h1>1 vs 1 盲评</h1>
      <p class="muted">
        选择主题与题量，匿名对比两侧回答，在底部四档中选一提交。<br />
        知乎刘看山会在流程里陪你读题、选档; 点点他看他会对你说些什么吧。
      </p>
    </div>
    <div v-if="!session" class="card battle-setup-card">
      <div class="battle-setup-layout">
        <div class="battle-setup-mascot">
          <KanshanMascot scene="home" size="md" no-sprite-trim />
        </div>
        <div class="form battle-setup-form">
          <div class="battle-setup-controls">
            <div class="battle-setup-topic">
              <span class="battle-setup-field-label">评估主题</span>
              <div
                ref="setupCategorySegment"
                class="battle-setup-segmented battle-setup-segmented--measured"
                aria-label="评估主题"
                :style="{ ...setupCategoryIndicatorStyle, '--setup-seg-count': categories.length, '--setup-seg-active': setupCategoryActiveIndex }"
              >
                <button
                  v-for="cat in categories"
                  :key="cat.id"
                  type="button"
                  class="battle-setup-seg-btn"
                  :class="{ 'battle-setup-seg-btn--active': categoryId === cat.id }"
                  @click="categoryId = cat.id"
                >
                  {{ cat.name }}
                </button>
              </div>
            </div>
            <label class="battle-setup-count">题目数<input v-model.number="count" type="number" min="1" max="20" /></label>
            <button class="battle-setup-start" @click="start">开始盲评</button>
          </div>
          <section v-if="selectedCategory" class="system-prompt-card">
            <div class="system-prompt-head">
              <div>
                <p class="eyebrow">SYSTEM PROMPT</p>
                <h3>{{ selectedCategory.name }}</h3>
              </div>
              <span v-if="selectedCategory.domainSlug" class="domain-pill">{{ selectedCategory.domainSlug }}</span>
            </div>
            <div v-if="selectedSystemPromptHtml" class="system-prompt-md" v-html="selectedSystemPromptHtml"></div>
            <p v-else class="muted">
              当前主题还没有匹配到 `eval-workspace/domains/*/prompts/system.md`，请确认分类 code/name 与评估领域目录已绑定。
            </p>
          </section>
          <p v-if="error" class="error">{{ error }}</p>
        </div>
      </div>
    </div>
    <div v-else-if="allVoted" class="card battle-done-card">
      <div class="battle-done-layout">
        <KanshanMascot scene="battle" :sprite-frame="3" :auto-cycle="false" size="sm" />
        <div>
          <h2>本轮已完成</h2>
          <p class="muted">下面是本轮你的实际选择、胜出模型，以及这轮选择对 Elo 与排名的贡献。</p>
          <button @click="resetRound">再来一轮</button>
        </div>
      </div>

      <section class="battle-result-section">
        <div class="battle-result-head">
          <div>
            <p class="eyebrow">ROUND DETAIL</p>
            <h3>本轮评估结果</h3>
          </div>
        </div>
        <div class="battle-result-list">
          <article v-for="row in completedRows" :key="row.itemId" class="battle-result-item">
            <div class="battle-result-main">
              <span class="rank-pill">#{{ row.position + 1 }}</span>
              <div class="battle-result-question-wrap" tabindex="0">
                <p class="battle-result-question">{{ row.question.prompt }}</p>
                <div class="battle-answer-preview" role="tooltip">
                  <div class="battle-answer-preview-col">
                    <strong>A · {{ modelLabel(row, 'left') }}</strong>
                    <p>{{ row.left.text }}</p>
                  </div>
                  <div class="battle-answer-preview-col">
                    <strong>B · {{ modelLabel(row, 'right') }}</strong>
                    <p>{{ row.right.text }}</p>
                  </div>
                </div>
                <p class="muted battle-result-matchup">
                  A：{{ modelLabel(row, 'left') }} / B：{{ modelLabel(row, 'right') }}
                </p>
              </div>
            </div>
            <div class="battle-result-choice">
              <span class="battle-result-outcome">{{ outcomeLabel(resolvedOutcome(row)) }}</span>
              <strong>{{ winnerText(row) }}</strong>
            </div>
          </article>
        </div>
      </section>

      <section class="battle-result-section battle-impact-section">
        <div class="battle-result-head">
          <div>
            <p class="eyebrow">ELO / RANK IMPACT</p>
            <h3>本轮选择贡献</h3>
          </div>
          <p class="muted">Elo 为本轮累计变化；排名为本轮每次投票记录的分类榜排名变化累计。</p>
        </div>
        <p v-if="!sessionEffectSummary.length" class="muted">本轮贡献明细暂不可用，新的投票会自动记录 Elo 与排名快照。</p>
        <div v-else class="battle-impact-grid">
          <article v-for="row in sessionEffectSummary" :key="row.modelId" class="battle-impact-card">
            <div>
              <h4>{{ row.modelName }}</h4>
              <p class="muted">参与 {{ row.count }} 次对比</p>
            </div>
            <div class="battle-impact-metrics">
              <span :class="{ 'impact-positive': row.eloDelta > 0, 'impact-negative': row.eloDelta < 0 }">
                Elo {{ signedNumber(row.eloDelta) }}
              </span>
              <span :class="{ 'impact-positive': row.rankDelta > 0, 'impact-negative': row.rankDelta < 0 }">
                Rank {{ rankDeltaLabel(row.rankDelta) }}
              </span>
            </div>
          </article>
        </div>
      </section>
    </div>
    <div v-else-if="currentRow" class="eval-flow">
      <div class="battle-top">
        <div class="eval-progress">
          <span class="eval-progress-tag">{{ progressLabel() }}</span>
          <span v-if="currentRow.voted" class="eval-progress-done">本题已提交</span>
        </div>
        <div class="battle-nav">
          <button type="button" class="secondary battle-nav-btn" :disabled="!canGoPrev" title="上一题" @click="goPrev">←</button>
          <button type="button" class="secondary battle-nav-btn" :disabled="!canGoNext" title="下一题" @click="goNextNav">→</button>
        </div>
      </div>

      <p
        v-if="session && session.desiredCount && session.desiredCount > sessionItems.length"
        class="muted eval-shortfall"
      >
        该主题当前仅有 {{ sessionItems.length }} 道可盲评题，已按题库实际数量开局（您曾选择 {{ session.desiredCount }} 道）。
      </p>

      <div class="battle-q-host card eval-question">
        <div class="battle-q-body">
          <KanshanMascot
            class="battle-q-mascot"
            scene="battle"
            :sprite-frame="BATTLE_Q_SPRITE_FRAME"
            :auto-cycle="false"
            size="xs"
            :show-caption="false"
            transparent-viewport
          />
          <div class="battle-q-text-stack">
            <div class="battle-q-meta-block">
              <span class="battle-q-kicker">刘看山出题</span>
              <p class="battle-q-aside">{{ battleHostLine }}</p>
            </div>
            <h2 class="battle-q-title" :aria-label="currentRow.question.prompt">
              <span aria-hidden="true">{{ questionTyped }}</span>
              <span v-if="cursorQuestion" class="eval-type-cursor" aria-hidden="true">▍</span>
            </h2>
          </div>
        </div>
      </div>

      <div class="battle-duel">
        <div
          class="card answer-card battle-answer"
          :class="{
            'answer-card--accent-good': leftCardAccent() === 'good',
            'answer-card--accent-bad': leftCardAccent() === 'bad',
          }"
        >
          <div class="battle-answer-head">
            <span class="battle-assistant-label">模型 A</span>
          </div>
          <div class="battle-answer-body">
            <div class="battle-answer-markdown" :aria-label="currentRow.left.text">
              <div aria-hidden="true" v-html="leftTypedHtml"></div>
              <span v-if="cursorLeft" class="eval-type-cursor" aria-hidden="true">▍</span>
            </div>
          </div>
        </div>
        <div
          class="card answer-card battle-answer"
          :class="{
            'answer-card--accent-good': rightCardAccent() === 'good',
            'answer-card--accent-bad': rightCardAccent() === 'bad',
          }"
        >
          <div class="battle-answer-head">
            <span class="battle-assistant-label">模型 B</span>
          </div>
          <div class="battle-answer-body">
            <div class="battle-answer-markdown" :aria-label="currentRow.right.text">
              <div aria-hidden="true" v-html="rightTypedHtml"></div>
              <span v-if="cursorRight" class="eval-type-cursor" aria-hidden="true">▍</span>
            </div>
          </div>
        </div>
      </div>

      <p v-if="error" class="error eval-error">{{ error }}</p>

      <div class="battle-rating card">
        <div class="battle-rating-row">
          <button
            type="button"
            class="secondary battle-rating-btn"
            :class="ratingBtnClass('left')"
            :disabled="currentRow.voted || voteSubmitting"
            @mouseenter="ratingHover = 'left'"
            @mouseleave="ratingHover = null"
            @click="submitVote('left')"
          >
            <span class="battle-rating-icon" aria-hidden="true">←</span>
            A 更好
          </button>
          <button
            type="button"
            class="secondary battle-rating-btn"
            :class="ratingBtnClass('both_good')"
            :disabled="currentRow.voted || voteSubmitting"
            @mouseenter="ratingHover = 'both_good'"
            @mouseleave="ratingHover = null"
            @click="submitVote('both_good')"
          >
            <span class="battle-rating-icon" aria-hidden="true">⇄</span>
            都好
          </button>
          <button
            type="button"
            class="secondary battle-rating-btn"
            :class="ratingBtnClass('both_bad')"
            :disabled="currentRow.voted || voteSubmitting"
            @mouseenter="ratingHover = 'both_bad'"
            @mouseleave="ratingHover = null"
            @click="submitVote('both_bad')"
          >
            <span class="battle-rating-icon battle-rating-icon--o" aria-hidden="true">∅</span>
            都不好
          </button>
          <button
            type="button"
            class="secondary battle-rating-btn"
            :class="ratingBtnClass('right')"
            :disabled="currentRow.voted || voteSubmitting"
            @mouseenter="ratingHover = 'right'"
            @mouseleave="ratingHover = null"
            @click="submitVote('right')"
          >
            B 更好
            <span class="battle-rating-icon" aria-hidden="true">→</span>
          </button>
        </div>
        <p class="battle-rating-hint muted">
          点选即提交；未答题可用箭头切换题目。单侧更好使用较强 K；「都好」「都不好」为平局 Elo，且「都不好」调节更轻。
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.blind-eval .eval-flow {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.battle-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.battle-setup-card {
  overflow: visible;
}

.battle-setup-layout {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 18px;
}

.battle-setup-mascot {
  display: flex;
  justify-content: center;
  align-self: center;
}

.battle-setup-form {
  gap: 10px;
}

.battle-setup-controls {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 10px;
  align-items: end;
}

.battle-setup-controls label {
  display: grid;
  gap: 6px;
  font-size: 13px;
  color: var(--text-secondary);
}

.battle-setup-topic {
  display: grid;
  gap: 6px;
  min-width: 0;
}

.battle-setup-field-label {
  color: var(--text-secondary);
  font-size: 13px;
}

.battle-setup-segmented {
  --setup-seg-gap: 4px;
  --setup-seg-pad: 4px;
  --setup-seg-indicator-width: 0px;
  --setup-seg-indicator-x: 0px;
  display: inline-flex;
  position: relative;
  max-width: 100%;
  min-height: 40px;
  gap: var(--setup-seg-gap);
  padding: var(--setup-seg-pad);
  overflow-x: auto;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: color-mix(in srgb, var(--surface-solid) 54%, transparent);
  scrollbar-width: thin;
}

.battle-setup-segmented::before {
  content: '';
  position: absolute;
  top: var(--setup-seg-pad);
  bottom: var(--setup-seg-pad);
  left: var(--setup-seg-pad);
  width: var(--setup-seg-indicator-width);
  border-radius: 999px;
  background: color-mix(in srgb, var(--brand-2) 16%, transparent);
  border: 1px solid color-mix(in srgb, var(--brand-2) 38%, var(--border));
  transform: translateX(calc(var(--setup-seg-indicator-x) - var(--setup-seg-pad)));
  transition:
    width 0.22s ease,
    transform 0.24s cubic-bezier(0.22, 1, 0.36, 1);
  pointer-events: none;
}

.battle-setup-seg-btn {
  position: relative;
  z-index: 1;
  flex: 0 0 auto;
  min-width: 0;
  padding: 7px 12px;
  border: 0;
  border-radius: 999px;
  color: var(--text-secondary);
  background: transparent !important;
  box-shadow: none !important;
  white-space: nowrap;
  font-size: 12px;
  font-weight: 800;
}

.battle-setup-seg-btn:hover {
  color: var(--text-primary);
  transform: none;
  box-shadow: none;
}

.battle-setup-seg-btn--active {
  color: var(--text-primary);
  transition:
    color 0.18s ease;
}

.battle-setup-controls select,
.battle-setup-controls input,
.battle-setup-start {
  min-height: 40px;
  padding-top: 9px;
  padding-bottom: 9px;
}

@media (min-width: 640px) {
  .battle-setup-layout {
    flex-direction: row;
    align-items: flex-start;
    justify-content: space-between;
    gap: 18px;
  }

  .battle-setup-mascot {
    flex: 0 0 196px;
    position: sticky;
    top: 88px;
  }

  .battle-setup-form {
    flex: 1;
    min-width: 0;
  }

  .battle-setup-controls {
    grid-template-columns: minmax(360px, 1.45fr) 86px minmax(128px, 0.7fr);
    align-items: end;
  }
}

@media (min-width: 640px) and (max-width: 920px) {
  .battle-setup-controls {
    grid-template-columns: 112px minmax(140px, 1fr);
  }

  .battle-setup-topic {
    grid-column: 1 / -1;
  }
}

.system-prompt-card {
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 16px;
  background: color-mix(in srgb, var(--surface-2) 68%, transparent);
}

.system-prompt-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}

.system-prompt-head h3,
.system-prompt-head p {
  margin: 0;
}

.domain-pill {
  display: inline-flex;
  align-items: center;
  border: 1px solid color-mix(in srgb, var(--brand-2) 36%, var(--border));
  border-radius: 999px;
  padding: 4px 9px;
  color: var(--brand-2);
  background: color-mix(in srgb, var(--brand-2) 10%, transparent);
  font-size: 12px;
  font-weight: 800;
  white-space: nowrap;
}

.system-prompt-md {
  color: var(--text-secondary);
  max-height: 150px;
  overflow: auto;
  padding-right: 4px;
  line-height: 1.55;
  font-size: 13px;
}

.system-prompt-md :deep(p) {
  margin: 0 0 8px;
}

.system-prompt-md :deep(ul) {
  margin: 8px 0 0;
  padding-left: 18px;
}

.system-prompt-md :deep(li) {
  margin: 4px 0;
}

.system-prompt-md :deep(code) {
  padding: 1px 5px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--surface) 84%, transparent);
}

.battle-done-card {
  overflow: visible;
}

.battle-done-layout {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  text-align: center;
  margin-bottom: 20px;
}

@media (min-width: 560px) {
  .battle-done-layout {
    flex-direction: row;
    align-items: center;
    text-align: left;
    gap: 22px;
  }
}

.battle-result-section {
  margin-top: 16px;
  border: 1px solid var(--border);
  border-radius: 20px;
  background: color-mix(in srgb, var(--surface-2) 58%, transparent);
  overflow: visible;
}

.battle-result-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 15px 16px;
  border-bottom: 1px solid var(--border);
}

.battle-result-head h3,
.battle-result-head p {
  margin: 0;
}

.battle-result-head .muted {
  max-width: 420px;
  font-size: 12px;
  line-height: 1.5;
  text-align: right;
}

.battle-result-list {
  display: grid;
}

.battle-result-item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(170px, 0.32fr);
  gap: 14px;
  padding: 14px 16px;
  border-bottom: 1px solid color-mix(in srgb, var(--border) 72%, transparent);
}

.battle-result-item:last-child {
  border-bottom: 0;
}

.battle-result-main {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  min-width: 0;
  position: relative;
}

.rank-pill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 42px;
  padding: 5px 9px;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--brand-2) 45%, var(--border));
  background: color-mix(in srgb, var(--brand-2) 12%, transparent);
  color: var(--brand-2);
  font-weight: 800;
  flex-shrink: 0;
}

.battle-result-question {
  margin: 0 0 8px;
  font-weight: 750;
  line-height: 1.5;
  cursor: help;
  text-decoration: underline;
  text-decoration-style: dotted;
  text-decoration-thickness: 1px;
  text-underline-offset: 4px;
  text-decoration-color: color-mix(in srgb, var(--brand-2) 55%, transparent);
}

.battle-result-question-wrap {
  position: relative;
  min-width: 0;
}

.battle-answer-preview {
  position: absolute;
  left: 0;
  top: calc(100% + 10px);
  z-index: 30;
  width: min(680px, calc(100vw - 72px));
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  padding: 14px;
  border: 1px solid color-mix(in srgb, var(--brand-2) 36%, var(--border));
  border-radius: 18px;
  background: color-mix(in srgb, var(--surface) 94%, transparent);
  box-shadow: var(--shadow-xl);
  opacity: 0;
  transform: translateY(6px);
  pointer-events: none;
  transition:
    opacity 0.16s ease,
    transform 0.16s ease;
}

.battle-result-question-wrap:hover .battle-answer-preview,
.battle-result-question-wrap:focus-within .battle-answer-preview {
  opacity: 1;
  transform: translateY(0);
}

.battle-answer-preview-col {
  max-height: 260px;
  overflow: auto;
  padding: 10px;
  border-radius: 14px;
  background: color-mix(in srgb, var(--surface-2) 76%, transparent);
}

.battle-answer-preview-col strong {
  display: block;
  margin-bottom: 8px;
  color: var(--brand-2);
}

.battle-answer-preview-col p {
  margin: 0;
  white-space: pre-wrap;
  line-height: 1.55;
  font-size: 13px;
}

.battle-result-matchup {
  margin: 0;
  font-size: 13px;
}

.battle-result-choice {
  display: grid;
  align-content: start;
  justify-items: end;
  gap: 5px;
  text-align: right;
}

.battle-result-outcome {
  display: inline-flex;
  padding: 4px 9px;
  border-radius: 999px;
  color: var(--brand-2);
  background: color-mix(in srgb, var(--brand-2) 12%, transparent);
  font-size: 12px;
  font-weight: 800;
}

.battle-impact-section {
  overflow: visible;
}

.battle-impact-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  padding: 14px 16px 16px;
}

.battle-impact-card {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 14px;
  border: 1px solid var(--border);
  border-radius: 18px;
  background:
    radial-gradient(circle at 10% 14%, color-mix(in srgb, var(--brand-2) 12%, transparent), transparent 36%),
    color-mix(in srgb, var(--surface) 76%, transparent);
}

.battle-impact-card h4 {
  margin: 0 0 6px;
}

.battle-impact-card p {
  margin: 0;
  font-size: 12px;
}

.battle-impact-metrics {
  display: grid;
  gap: 6px;
  justify-items: end;
  white-space: nowrap;
  font-weight: 800;
}

.impact-positive {
  color: #16a34a;
}

.impact-negative {
  color: var(--danger);
}

@media (max-width: 720px) {
  .battle-result-head,
  .battle-result-item,
  .battle-impact-card {
    grid-template-columns: 1fr;
  }

  .battle-result-head {
    flex-direction: column;
  }

  .battle-result-head .muted,
  .battle-result-choice {
    justify-items: start;
    text-align: left;
  }

  .battle-impact-grid {
    grid-template-columns: 1fr;
  }

  .battle-answer-preview {
    grid-template-columns: 1fr;
  }
}

.battle-q-host {
  max-width: 720px;
  margin: 0 auto;
  width: 100%;
  padding: 7px 8px 8px;
  overflow: visible;
}

.battle-q-body {
  display: flex;
  flex-direction: row;
  align-items: flex-start;
  gap: 6px;
}

.battle-q-text-stack {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.battle-q-meta-block {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.battle-q-mascot {
  flex-shrink: 0;
  align-self: flex-start;
  margin-left: -6px;
}

/* 侧视朝左素材 → 镜像为朝右（面向题干）；仅作用于题干旁 xs mascot */
:deep(.battle-q-mascot .kanshan-viewport) {
  transform: scaleX(-1);
  transform-origin: center center;
}

.battle-q-title {
  margin: 0;
  font-size: 1.18rem;
  line-height: 1.32;
}

@media (max-width: 520px) {
  .battle-q-body {
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
  }

  .battle-q-mascot {
    align-self: center;
  }
}

.battle-q-kicker {
  display: block;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--brand-2);
  margin: 0;
}

.battle-q-aside {
  margin: 0;
  font-size: 13px;
  line-height: 1.45;
  color: var(--text-secondary);
}

.battle-duel {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  align-items: stretch;
}

@media (max-width: 820px) {
  .battle-duel {
    grid-template-columns: 1fr;
  }
}

.eval-progress {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.eval-progress-tag {
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--text-secondary);
  padding: 6px 12px;
  border-radius: 999px;
  border: 1px solid var(--border);
  background: color-mix(in srgb, var(--surface-2) 70%, transparent);
}

.eval-progress-done {
  font-size: 13px;
  font-weight: 600;
  color: var(--brand-2);
}

.battle-nav {
  display: flex;
  gap: 8px;
}

.battle-nav-btn {
  min-width: 40px;
  padding: 8px 12px;
  border-radius: 12px !important;
  font-weight: 700;
}

.battle-answer {
  display: flex;
  flex-direction: column;
  min-height: 220px;
  cursor: default;
  transition:
    border-color 0.2s ease,
    box-shadow 0.2s ease;
}

.battle-answer-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.battle-assistant-label {
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-secondary);
}

.battle-answer-body {
  flex: 1;
  min-height: 0;
  max-height: min(52vh, 460px);
  overflow: auto;
  padding-right: 4px;
}

.battle-answer-markdown {
  min-height: 100%;
  line-height: 1.65;
  word-break: break-word;
  font-size: 14px;
}

.battle-answer-markdown :deep(p),
.battle-answer-markdown :deep(ul),
.battle-answer-markdown :deep(h3),
.battle-answer-markdown :deep(h4) {
  margin-top: 0;
}

.battle-answer-markdown :deep(p) {
  margin: 0;
  margin-bottom: 0.72em;
}

.battle-answer-markdown :deep(ul) {
  margin: 0 0 0.72em;
  padding-left: 1.2em;
}

.battle-answer-markdown :deep(li) {
  margin: 0.2em 0;
}

.battle-answer-markdown :deep(h3),
.battle-answer-markdown :deep(h4) {
  margin-bottom: 0.55em;
  color: var(--text);
  line-height: 1.35;
}

.battle-answer-markdown :deep(strong) {
  color: var(--text);
  font-weight: 800;
}

.battle-answer-markdown :deep(code) {
  padding: 0.08em 0.32em;
  border-radius: 6px;
  background: color-mix(in srgb, var(--surface-2) 86%, transparent);
  color: var(--brand-2);
}

.answer-card--accent-good {
  border-color: color-mix(in srgb, #22c55e 55%, var(--border)) !important;
  box-shadow:
    0 0 0 1px color-mix(in srgb, #22c55e 35%, transparent),
    0 12px 36px color-mix(in srgb, #22c55e 12%, transparent);
}

.answer-card--accent-bad {
  border-color: color-mix(in srgb, var(--danger) 55%, var(--border)) !important;
  box-shadow:
    0 0 0 1px color-mix(in srgb, var(--danger) 35%, transparent),
    0 12px 36px color-mix(in srgb, var(--danger) 12%, transparent);
}

.eval-error {
  margin: 0;
}

.battle-rating {
  padding: 14px 16px 12px;
  border-radius: 16px;
}

.battle-rating-row {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}

@media (max-width: 720px) {
  .battle-rating-row {
    grid-template-columns: 1fr 1fr;
  }
}

.battle-rating-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 12px 10px;
  border-radius: 14px !important;
  font-weight: 700;
  font-size: 13px;
  box-shadow: none !important;
  transform: none !important;
  min-height: 46px;
}

.battle-rating-btn:not(:disabled):hover {
  border-color: color-mix(in srgb, var(--brand-2) 38%, var(--border));
}

.battle-rating-icon {
  font-size: 15px;
  opacity: 0.9;
}

.battle-rating-icon--o {
  font-size: 17px;
  line-height: 1;
}

.battle-rating-btn--active-a {
  border-color: color-mix(in srgb, #22c55e 50%, var(--border)) !important;
  background: color-mix(in srgb, #22c55e 14%, var(--surface-2)) !important;
  color: var(--text-primary) !important;
}

.battle-rating-btn--active-b {
  border-color: color-mix(in srgb, #22c55e 50%, var(--border)) !important;
  background: color-mix(in srgb, #22c55e 14%, var(--surface-2)) !important;
  color: var(--text-primary) !important;
}

.battle-rating-btn--active-both-good {
  border-color: color-mix(in srgb, #22c55e 45%, var(--border)) !important;
  background: color-mix(in srgb, #22c55e 12%, var(--surface-2)) !important;
}

.battle-rating-btn--active-both-bad {
  border-color: color-mix(in srgb, var(--danger) 45%, var(--border)) !important;
  background: color-mix(in srgb, var(--danger) 12%, var(--surface-2)) !important;
}

.battle-rating-btn:disabled:not(.battle-rating-btn--active-a):not(.battle-rating-btn--active-b):not(
    .battle-rating-btn--active-both-good
  ):not(.battle-rating-btn--active-both-bad) {
  opacity: 0.5;
}

.battle-rating-hint {
  margin: 10px 0 0;
  font-size: 11px;
  line-height: 1.4;
  text-align: center;
}
</style>
