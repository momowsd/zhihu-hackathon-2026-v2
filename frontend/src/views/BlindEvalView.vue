<script setup lang="ts">
import type { Ref } from 'vue'
import { computed, onMounted, ref, watch } from 'vue'
import { api, errorMessage, loadCategories, type ApiEnvelope, type Category } from '../api'
import { useRoute } from 'vue-router'

type Session = { id: string; requestedCount: number; desiredCount?: number }
type SessionItemRow = {
  itemId: string
  position: number
  question: { id: string; prompt: string }
  left: { answerId: string; text: string }
  right: { answerId: string; text: string }
  voted: boolean
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
const confidence = ref(3)
const error = ref('')
const route = useRoute()

const pendingWinner = ref<'left' | 'right' | null>(null)
/** 未提交时，在题目间切换会暂存选项与信心分 */
const drafts = ref<Record<string, { pendingWinner: 'left' | 'right'; confidence: number }>>({})

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

function syncFromCurrentRow() {
  const row = currentRow.value
  if (!row) return
  if (row.voted) {
    pendingWinner.value = row.winnerSide ?? null
    confidence.value = row.confidenceScore ?? 3
  } else {
    const d = drafts.value[row.itemId]
    if (d) {
      pendingWinner.value = d.pendingWinner
      confidence.value = d.confidence
    } else {
      pendingWinner.value = null
      confidence.value = 3
    }
  }
}

function saveDraft() {
  const row = currentRow.value
  if (!row || row.voted) return
  if (pendingWinner.value != null) {
    drafts.value[row.itemId] = {
      pendingWinner: pendingWinner.value,
      confidence: confidence.value,
    }
  }
}

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
  syncFromCurrentRow()
}

async function start() {
  error.value = ''
  drafts.value = {}
  try {
    const response = await api.post<ApiEnvelope<Session>>('/eval/sessions', { categoryId: categoryId.value, count: count.value })
    await loadSessionItems(response.data.data.id)
  } catch (err) {
    error.value = errorMessage(err)
  }
}

function pick(side: 'left' | 'right') {
  const row = currentRow.value
  if (!row || row.voted) return
  pendingWinner.value = side
}

function goPrev() {
  if (!canGoPrev.value) return
  saveDraft()
  currentIndex.value -= 1
  syncFromCurrentRow()
}

function goNextNav() {
  if (!canGoNext.value) return
  saveDraft()
  currentIndex.value += 1
  syncFromCurrentRow()
}

async function confirmVote() {
  const row = currentRow.value
  if (!row?.itemId || row.voted || !pendingWinner.value) return
  error.value = ''
  try {
    await api.post('/eval/votes', {
      itemId: row.itemId,
      winnerSide: pendingWinner.value,
      confidenceScore: confidence.value,
    })
    row.voted = true
    row.winnerSide = pendingWinner.value
    row.confidenceScore = confidence.value
    delete drafts.value[row.itemId]

    const lastIdx = sessionItems.value.length - 1
    if (currentIndex.value < lastIdx) {
      currentIndex.value += 1
      syncFromCurrentRow()
    }
  } catch (err) {
    error.value = errorMessage(err)
  }
}

function resetRound() {
  session.value = null
  sessionItems.value = []
  currentIndex.value = 0
  pendingWinner.value = null
  drafts.value = {}
  error.value = ''
}

function progressLabel() {
  const total = sessionItems.value.length
  if (!total) return ''
  return `第 ${currentIndex.value + 1} / ${total} 题`
}
</script>

<template>
  <div class="page blind-eval">
    <div class="header">
      <h1>1 vs 1 盲评</h1>
      <p class="muted">选择主题和题目数，匿名比较两个模型回答，选出胜者并给出差距/信心分。</p>
    </div>
    <div v-if="!session" class="card">
      <div class="form">
        <label>评估主题<select v-model="categoryId"><option v-for="cat in categories" :key="cat.id" :value="cat.id">{{ cat.name }}</option></select></label>
        <label>题目数<input v-model.number="count" type="number" min="1" max="20" /></label>
        <p v-if="error" class="error">{{ error }}</p>
        <button @click="start">开始盲评</button>
      </div>
    </div>
    <div v-else-if="allVoted" class="card">
      <h2>本轮已完成</h2>
      <p class="muted">可以去 Ranking 查看 Elo 变化，或重新开始一轮。</p>
      <button @click="resetRound">再来一轮</button>
    </div>
    <div v-else-if="currentRow" class="eval-flow">
      <div class="eval-progress">
        <span class="eval-progress-tag">{{ progressLabel() }}</span>
        <span v-if="currentRow.voted" class="eval-progress-done">本题已提交</span>
      </div>
      <p
        v-if="session && session.desiredCount && session.desiredCount > sessionItems.length"
        class="muted eval-shortfall"
      >
        该主题当前仅有 {{ sessionItems.length }} 道可盲评题，已按题库实际数量开局（您曾选择 {{ session.desiredCount }} 道）。
      </p>

      <div class="card eval-question">
        <div class="muted">题目</div>
        <h2 :aria-label="currentRow.question.prompt">
          <span aria-hidden="true">{{ questionTyped }}</span>
          <span v-if="cursorQuestion" class="eval-type-cursor" aria-hidden="true">▍</span>
        </h2>
      </div>

      <div class="grid eval-answers">
        <div
          class="card answer-card"
          :class="{
            'answer-card--selected': pendingWinner === 'left',
            'answer-card--locked': currentRow.voted && pendingWinner === 'left',
          }"
          @click="pick('left')"
        >
          <h3>回答 A</h3>
          <p :aria-label="currentRow.left.text">
            <span aria-hidden="true">{{ leftTyped }}</span>
            <span v-if="cursorLeft" class="eval-type-cursor" aria-hidden="true">▍</span>
          </p>
          <button type="button" class="pick-btn" :disabled="currentRow.voted" @click.stop="pick('left')">
            A 更好
          </button>
        </div>
        <div
          class="card answer-card"
          :class="{
            'answer-card--selected': pendingWinner === 'right',
            'answer-card--locked': currentRow.voted && pendingWinner === 'right',
          }"
          @click="pick('right')"
        >
          <h3>回答 B</h3>
          <p :aria-label="currentRow.right.text">
            <span aria-hidden="true">{{ rightTyped }}</span>
            <span v-if="cursorRight" class="eval-type-cursor" aria-hidden="true">▍</span>
          </p>
          <button type="button" class="pick-btn" :disabled="currentRow.voted" @click.stop="pick('right')">
            B 更好
          </button>
        </div>
      </div>

      <p v-if="error" class="error eval-error">{{ error }}</p>

      <div class="eval-footer">
        <div class="eval-confidence">
          <div class="eval-confidence-head">
            <span class="eval-confidence-label">差距 / 信心分</span>
            <span class="eval-confidence-value">{{ confidence }}</span>
          </div>
          <input
            v-model.number="confidence"
            class="eval-range"
            type="range"
            min="1"
            max="5"
            :disabled="currentRow.voted"
          />
        </div>

        <div class="eval-toolbar">
          <button
            type="button"
            class="eval-icon-btn"
            :disabled="!canGoPrev"
            title="上一题"
            @click="goPrev"
          >
            <svg class="eval-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M14 6l-6 6 6 6"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
          </button>

          <button
            type="button"
            class="eval-confirm"
            :disabled="!pendingWinner || currentRow.voted"
            @click="confirmVote"
          >
            <svg class="eval-icon eval-icon--sm" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M20 6L9 17l-5-5"
                stroke="currentColor"
                stroke-width="2.2"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
            <span>确定</span>
          </button>

          <button
            type="button"
            class="eval-icon-btn eval-icon-btn--accent"
            :disabled="!canGoNext"
            title="下一题"
            @click="goNextNav"
          >
            <svg class="eval-icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M10 6l6 6-6 6"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            </svg>
          </button>
        </div>
        <p class="eval-hint muted">
          箭头切换题目，未提交会暂存；最后一题确认后进入完成页。
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.blind-eval .eval-flow {
  display: flex;
  flex-direction: column;
  gap: 18px;
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

.eval-question h2 {
  margin: 8px 0 0;
  font-size: 1.35rem;
  line-height: 1.45;
}

.eval-answers {
  align-items: stretch;
}

.answer-card {
  cursor: pointer;
  transition:
    border-color 0.22s ease,
    box-shadow 0.22s ease,
    transform 0.18s ease;
}

.answer-card:hover:not(.answer-card--locked) {
  transform: translateY(-2px);
}

.answer-card--selected {
  border-color: color-mix(in srgb, var(--brand-2) 65%, var(--border)) !important;
  box-shadow:
    0 0 0 1px color-mix(in srgb, var(--brand-2) 45%, transparent),
    0 16px 48px color-mix(in srgb, var(--brand-2) 18%, transparent);
}

.answer-card--locked.answer-card--selected {
  border-color: color-mix(in srgb, var(--brand-2) 40%, var(--border)) !important;
  opacity: 0.92;
}

button.pick-btn {
  margin-top: auto;
  border-radius: 12px !important;
  font-weight: 700;
  font-size: 13px;
  background: color-mix(in srgb, var(--surface-2) 88%, transparent) !important;
  border: 1px solid var(--border) !important;
  color: var(--text-primary) !important;
  box-shadow: none !important;
}

button.pick-btn:hover:not(:disabled) {
  border-color: color-mix(in srgb, var(--brand-2) 45%, var(--border)) !important;
}

/* 当前选中的回答：与「确定」按钮 eval-confirm 同款配色 */
.answer-card--selected:not(.answer-card--locked) button.pick-btn {
  background: linear-gradient(135deg, color-mix(in srgb, var(--brand-2) 85%, #fff), color-mix(in srgb, var(--brand-3) 75%, #fff)) !important;
  color: #041018 !important;
  border: 1px solid color-mix(in srgb, var(--brand-2) 45%, transparent) !important;
  box-shadow:
    0 0 0 1px color-mix(in srgb, #fff 18%, transparent) inset,
    0 12px 32px color-mix(in srgb, var(--brand-2) 22%, transparent) !important;
}

.answer-card--selected:not(.answer-card--locked) button.pick-btn:hover:not(:disabled) {
  filter: brightness(1.05);
}

:root[data-theme='light'] .answer-card--selected:not(.answer-card--locked) button.pick-btn {
  color: #0f172a !important;
}

/* 已提交后的胜者一侧：同款渐变，略压亮度表示不可再改 */
.answer-card--locked.answer-card--selected button.pick-btn {
  background: linear-gradient(135deg, color-mix(in srgb, var(--brand-2) 85%, #fff), color-mix(in srgb, var(--brand-3) 75%, #fff)) !important;
  color: #041018 !important;
  border: 1px solid color-mix(in srgb, var(--brand-2) 45%, transparent) !important;
  box-shadow:
    0 0 0 1px color-mix(in srgb, #fff 18%, transparent) inset,
    0 8px 24px color-mix(in srgb, var(--brand-2) 16%, transparent) !important;
  opacity: 0.88;
}

:root[data-theme='light'] .answer-card--locked.answer-card--selected button.pick-btn {
  color: #0f172a !important;
}

.eval-error {
  margin: 0;
}

.eval-footer {
  margin-top: 4px;
  padding: 12px 14px 10px;
  border-radius: 16px;
  border: 1px solid var(--border);
  background: linear-gradient(
    165deg,
    color-mix(in srgb, var(--surface) 92%, transparent),
    color-mix(in srgb, var(--surface-2) 55%, transparent)
  );
  box-shadow: var(--shadow-xl);
}

.eval-confidence-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 6px;
}

.eval-confidence-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}

.eval-confidence-value {
  font-size: 22px;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.04em;
  background: linear-gradient(135deg, var(--brand-2), var(--brand-3));
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}

.eval-range {
  width: 100%;
  accent-color: var(--brand-2);
}

.eval-toolbar {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  margin-top: 12px;
}

button.eval-icon-btn {
  width: 42px;
  height: 42px;
  padding: 0;
  border-radius: 12px !important;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--surface-2) 88%, transparent) !important;
  border: 1px solid var(--border);
  color: var(--text-secondary);
  box-shadow: none !important;
  transform: none;
}

button.eval-icon-btn:not(:disabled):hover {
  color: var(--text-primary);
  border-color: color-mix(in srgb, var(--brand-2) 35%, var(--border));
  box-shadow: 0 0 24px color-mix(in srgb, var(--brand-2) 12%, transparent) !important;
}

button.eval-icon-btn:disabled {
  opacity: 0.38;
  cursor: not-allowed;
}

button.eval-icon-btn--accent:not(:disabled) {
  color: var(--brand-2);
  border-color: color-mix(in srgb, var(--brand-2) 42%, var(--border));
}

button.eval-confirm {
  min-width: 132px;
  height: 42px;
  padding: 0 18px;
  border-radius: 12px !important;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font-size: 14px;
  letter-spacing: 0.02em;
  background: linear-gradient(135deg, color-mix(in srgb, var(--brand-2) 85%, #fff), color-mix(in srgb, var(--brand-3) 75%, #fff));
  color: #041018;
  border: 1px solid color-mix(in srgb, var(--brand-2) 45%, transparent);
  box-shadow:
    0 0 0 1px color-mix(in srgb, #fff 18%, transparent) inset,
    0 12px 32px color-mix(in srgb, var(--brand-2) 22%, transparent);
}

button.eval-confirm:disabled {
  opacity: 0.45;
  filter: grayscale(0.3);
}

.eval-icon {
  width: 22px;
  height: 22px;
}

.eval-icon--sm {
  width: 20px;
  height: 20px;
}

.eval-hint {
  margin: 8px 0 0;
  font-size: 11px;
  line-height: 1.35;
  text-align: center;
}

:root[data-theme='light'] button.eval-confirm {
  color: #0f172a;
}
</style>
