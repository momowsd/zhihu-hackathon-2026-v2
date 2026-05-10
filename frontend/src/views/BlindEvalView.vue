<script setup lang="ts">
import type { Ref } from 'vue'
import { computed, onMounted, ref, watch } from 'vue'
import { api, errorMessage, loadCategories, type ApiEnvelope, type Category } from '../api'
import { useRoute } from 'vue-router'
import KanshanMascot from '../components/KanshanMascot.vue'

type VoteOutcome = 'left' | 'right' | 'both_good' | 'both_bad'

type Session = { id: string; requestedCount: number; desiredCount?: number }
type SessionItemRow = {
  itemId: string
  position: number
  question: { id: string; prompt: string }
  left: { answerId: string; text: string }
  right: { answerId: string; text: string }
  voted: boolean
  /** 新接口：四档之一；旧数据可能仅有 winnerSide */
  outcome?: VoteOutcome
  winnerSide?: 'left' | 'right'
  confidenceScore?: number
}
type SessionItemsPayload = {
  sessionId: string
  desiredCount?: number
  requestedCount: number
  items: SessionItemRow[]
}

const categories = ref<Category[]>([])
const categoryId = ref('')
const count = ref(5)
const session = ref<Session | null>(null)
const sessionItems = ref<SessionItemRow[]>([])
const currentIndex = ref(0)
const error = ref('')
const route = useRoute()

/** 悬停在某档评分按钮上时，用于两侧卡片边框预览 */
const ratingHover = ref<VoteOutcome | null>(null)
const voteSubmitting = ref(false)

const currentRow = computed(() => sessionItems.value[currentIndex.value] ?? null)

/** 盲评：先题干打字结束，再两侧回答并行打字（切换题目时取消上一轮） */
const questionTyped = ref('')
const leftTyped = ref('')
const rightTyped = ref('')
let typewriterRound = 0

const CHAR_MS = 15

function sleep(ms: number) {
  return new Promise<void>((resolve) => {
    setTimeout(resolve, ms)
  })
}

async function streamInto(target: Ref<string>, full: string, round: number) {
  for (const ch of full) {
    if (round !== typewriterRound) return
    target.value += ch
    await sleep(CHAR_MS)
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

    await streamInto(questionTyped, row.question.prompt, round)
    if (round !== typewriterRound) return

    await Promise.all([
      streamInto(leftTyped, row.left.text, round),
      streamInto(rightTyped, row.right.text, round),
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

const canGoPrev = computed(() => currentIndex.value > 0)
const canGoNext = computed(() => currentIndex.value < sessionItems.value.length - 1)

function resolvedOutcome(row: SessionItemRow): VoteOutcome | null {
  if (!row.voted) return null
  if (row.outcome) return row.outcome
  if (row.winnerSide === 'left') return 'left'
  if (row.winnerSide === 'right') return 'right'
  return null
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
  const arenaId = route.query.arenaSessionId
  if (typeof arenaId === 'string' && arenaId) {
    try {
      await loadSessionItems(arenaId)
    } catch (err) {
      error.value = errorMessage(err)
    }
  }
})

async function loadSessionItems(sessionId: string) {
  const response = await api.get<ApiEnvelope<SessionItemsPayload>>(`/eval/sessions/${sessionId}/items`)
  const data = response.data.data
  session.value = {
    id: data.sessionId,
    requestedCount: data.requestedCount,
    desiredCount: data.desiredCount,
  }
  sessionItems.value = data.items
  currentIndex.value = 0
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
    await api.post('/eval/votes', {
      itemId: row.itemId,
      outcome,
    })
    row.voted = true
    row.outcome = outcome
    if (outcome === 'left') row.winnerSide = 'left'
    else if (outcome === 'right') row.winnerSide = 'right'
    else row.winnerSide = undefined

    const lastIdx = sessionItems.value.length - 1
    if (currentIndex.value < lastIdx) {
      currentIndex.value += 1
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
        选择主题与题量，匿名对比两侧回答，在底部四档中选一提交。知乎刘看山会在流程里陪你读题、选档——点点他，还有一句随机主持提示。
      </p>
    </div>
    <div v-if="!session" class="card battle-setup-card">
      <div class="battle-setup-layout">
        <KanshanMascot scene="home" size="md" no-sprite-trim />
        <div class="form battle-setup-form">
          <label>评估主题<select v-model="categoryId"><option v-for="cat in categories" :key="cat.id" :value="cat.id">{{ cat.name }}</option></select></label>
          <label>题目数<input v-model.number="count" type="number" min="1" max="20" /></label>
          <p v-if="error" class="error">{{ error }}</p>
          <button @click="start">开始盲评</button>
        </div>
      </div>
    </div>
    <div v-else-if="allVoted" class="card battle-done-card">
      <div class="battle-done-layout">
        <KanshanMascot scene="battle" :sprite-frame="3" :auto-cycle="false" size="sm" />
        <div>
          <h2>本轮已完成</h2>
          <p class="muted">可以去 Ranking 查看 Elo 变化，或重新开始一轮。</p>
          <button @click="resetRound">再来一轮</button>
        </div>
      </div>
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
            <span class="battle-assistant-label">助手 A</span>
          </div>
          <div class="battle-answer-body">
            <p :aria-label="currentRow.left.text">
              <span aria-hidden="true">{{ leftTyped }}</span>
              <span v-if="cursorLeft" class="eval-type-cursor" aria-hidden="true">▍</span>
            </p>
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
            <span class="battle-assistant-label">助手 B</span>
          </div>
          <div class="battle-answer-body">
            <p :aria-label="currentRow.right.text">
              <span aria-hidden="true">{{ rightTyped }}</span>
              <span v-if="cursorRight" class="eval-type-cursor" aria-hidden="true">▍</span>
            </p>
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

@media (min-width: 640px) {
  .battle-setup-layout {
    flex-direction: row;
    align-items: flex-end;
    justify-content: space-between;
    gap: 24px;
  }

  .battle-setup-form {
    flex: 1;
    min-width: 0;
  }
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
}

@media (min-width: 560px) {
  .battle-done-layout {
    flex-direction: row;
    align-items: center;
    text-align: left;
    gap: 22px;
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
}

.battle-answer-body p {
  margin: 0;
  line-height: 1.65;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 14px;
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
