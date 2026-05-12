<script setup lang="ts">
import { computed, nextTick, onMounted, ref, useTemplateRef, watch } from 'vue'
import { errorMessage, loadUserHistory, type UserHistoryItem, type UserModelFitRow } from '../api'
import { clearTokens, useAuth } from '../auth'
import { router } from '../router'

const auth = useAuth()
const roleLabel = computed(() => (auth.isAdmin.value ? '管理员' : '普通用户'))
const history = ref<UserHistoryItem[]>([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = 10
const fitScopes = ref<string[]>(['全部'])
const topModels = ref<UserModelFitRow[]>([])
const activeFitScope = ref('全部')
const loading = ref(false)
const error = ref('')
const fitScopeTabsRef = useTemplateRef<HTMLElement>('fitScopeTabs')
const fitScopeIndicator = ref({ width: 0, x: 0 })
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
const scopeTopModels = computed(() => topModels.value.filter((row) => row.scope === activeFitScope.value))
const domainCount = computed(() => Math.max(0, fitScopes.value.filter((scope) => scope !== '全部').length))
const fitScopeActiveIndex = computed(() => Math.max(0, fitScopes.value.findIndex((scope) => scope === activeFitScope.value)))
const fitScopeIndicatorStyle = computed(() => ({
  '--fit-scope-indicator-width': `${fitScopeIndicator.value.width}px`,
  '--fit-scope-indicator-x': `${fitScopeIndicator.value.x}px`,
  '--fit-scope-count': fitScopes.value.length,
  '--fit-scope-active': fitScopeActiveIndex.value,
}))

const groupedHistory = computed(() => {
  const groups = new Map<string, { sessionId: string; categoryName: string; sessionCreatedAt: string; items: UserHistoryItem[] }>()
  for (const item of history.value) {
    const group = groups.get(item.sessionId) ?? {
      sessionId: item.sessionId,
      categoryName: item.categoryName,
      sessionCreatedAt: item.sessionCreatedAt,
      items: [],
    }
    group.items.push(item)
    groups.set(item.sessionId, group)
  }
  return Array.from(groups.values())
})

function logout() {
  clearTokens()
  router.push('/')
}

function formatTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', { dateStyle: 'short', timeStyle: 'short' })
}

function winnerText(item: UserHistoryItem) {
  if (item.outcome === 'both_bad') return '无胜出模型'
  if (!item.winnerModels.length) return '未记录'
  return item.winnerModels.join('、')
}

function rateLabel(value: number) {
  return `${Math.round(value * 100)}%`
}

function fitBasis(row: UserModelFitRow) {
  return `${row.positive} 次正向选择 / ${row.appearances} 次出现，适配率 ${rateLabel(row.rate)}`
}

async function updateFitScopeIndicator() {
  await nextTick()
  const el = fitScopeTabsRef.value
  const active = el?.querySelector<HTMLElement>('.fit-scope-btn--active')
  if (!el || !active) return
  fitScopeIndicator.value = { width: active.offsetWidth, x: active.offsetLeft }
}

async function load(page = currentPage.value) {
  loading.value = true
  error.value = ''
  try {
    const response = await loadUserHistory(page, pageSize)
    history.value = response.items
    total.value = response.total
    currentPage.value = response.page
    fitScopes.value = response.fitScopes.length ? response.fitScopes : ['全部']
    topModels.value = response.topModels
    if (!fitScopes.value.includes(activeFitScope.value)) {
      activeFitScope.value = '全部'
    }
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

function goPage(page: number) {
  if (page < 1 || page > totalPages.value || loading.value) return
  load(page)
}

watch([activeFitScope, fitScopes], updateFitScopeIndicator, { flush: 'post' })

onMounted(() => {
  load()
  updateFitScopeIndicator()
})
</script>

<template>
  <div class="page">
    <div class="header">
      <h1>个人中心</h1>
      <p class="muted">查看当前登录身份。后续接入 OAuth 后，这里可以承载统一账号信息。</p>
    </div>
    <div class="card profile-card profile-card--compact">
      <div class="profile-main">
        <div class="avatar">{{ auth.username.value.slice(0, 1).toUpperCase() || 'U' }}</div>
        <div>
          <div class="muted">用户名</div>
          <h2>{{ auth.username.value }}</h2>
          <p class="muted">角色：{{ roleLabel }}</p>
        </div>
      </div>
      <div class="profile-stats">
        <div>
          <strong>{{ total }}</strong>
          <span class="muted">历史评估</span>
        </div>
        <div>
          <strong>{{ domainCount }}</strong>
          <span class="muted">覆盖领域</span>
        </div>
      </div>
      <button class="ghost profile-logout" @click="logout">退出登录</button>
    </div>

    <section class="card profile-fit-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">MODEL MATCH</p>
          <h2>最适配你的模型 Top 3</h2>
          <p class="muted">基于你的历史 Battle 选择统计，优先按正向选择次数排序。</p>
        </div>
      </div>
      <div
        ref="fitScopeTabs"
        class="fit-scope-tabs fit-scope-tabs--measured"
        aria-label="模型适配统计范围"
        :style="fitScopeIndicatorStyle"
      >
        <button
          v-for="scope in fitScopes"
          :key="scope"
          type="button"
          class="fit-scope-btn"
          :class="{ 'fit-scope-btn--active': activeFitScope === scope }"
          @click="activeFitScope = scope"
        >
          {{ scope }}
        </button>
      </div>
      <p v-if="!scopeTopModels.length" class="muted">完成更多评估后，这里会展示更稳定的模型适配排序。</p>
      <div v-else class="fit-model-grid">
        <article v-for="row in scopeTopModels" :key="row.scope + row.modelId" class="fit-model-card">
          <span class="rank-pill">#{{ row.rank }}</span>
          <div>
            <h3>{{ row.modelName }}</h3>
            <p class="muted">{{ fitBasis(row) }}</p>
          </div>
        </article>
      </div>
    </section>

    <section class="card profile-history-card">
      <div class="section-head">
        <div>
          <p class="eyebrow">MY HISTORY</p>
          <h2>我的历史评估</h2>
          <p class="muted">鼠标悬停在问题上，可以预览当时 A/B 两个模型的完整回答。</p>
        </div>
        <button type="button" class="secondary history-refresh" :disabled="loading" @click="load()">
          {{ loading ? '加载中…' : '刷新' }}
        </button>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
      <p v-else-if="loading && !history.length" class="muted">正在读取历史评估…</p>
      <p v-else-if="!history.length" class="muted">暂无历史评估。完成一次 Battle 后，这里会展示你的选择记录。</p>

      <div v-else class="history-list">
        <article v-for="group in groupedHistory" :key="group.sessionId" class="history-session">
          <div class="history-session-head">
            <div>
              <h3>{{ group.categoryName }}</h3>
              <p class="muted">{{ formatTime(group.sessionCreatedAt) }} · {{ group.items.length }} 条记录</p>
            </div>
          </div>
          <div class="history-items">
            <div v-for="item in group.items" :key="item.itemId" class="history-item">
              <div class="history-item-main">
                <span class="rank-pill">#{{ item.position + 1 }}</span>
                <div class="history-question-wrap" tabindex="0">
                  <p class="history-question">{{ item.questionPrompt }}</p>
                  <div class="answer-preview" role="tooltip">
                    <div class="answer-preview-col">
                      <strong>A · {{ item.leftModelName }}</strong>
                      <p>{{ item.leftAnswerText }}</p>
                    </div>
                    <div class="answer-preview-col">
                      <strong>B · {{ item.rightModelName }}</strong>
                      <p>{{ item.rightAnswerText }}</p>
                    </div>
                  </div>
                  <p class="muted history-matchup">A：{{ item.leftModelName }} / B：{{ item.rightModelName }}</p>
                </div>
              </div>
              <div class="history-result">
                <span class="history-outcome">{{ item.outcomeLabel }}</span>
                <strong>{{ winnerText(item) }}</strong>
                <small class="muted">{{ formatTime(item.votedAt) }}</small>
              </div>
            </div>
          </div>
        </article>
      </div>

      <div v-if="total > pageSize" class="history-pagination">
        <button type="button" class="secondary" :disabled="currentPage <= 1 || loading" @click="goPage(currentPage - 1)">上一页</button>
        <span class="muted">第 {{ currentPage }} / {{ totalPages }} 页 · 共 {{ total }} 条</span>
        <button type="button" class="secondary" :disabled="currentPage >= totalPages || loading" @click="goPage(currentPage + 1)">下一页</button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.profile-history-card {
  margin-top: 18px;
}

.profile-card--compact {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 20px;
  padding: 18px 20px;
}

.profile-main {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.profile-main h2 {
  margin: 2px 0 4px;
}

.profile-main p {
  margin: 0;
}

.profile-stats {
  display: flex;
  gap: 10px;
}

.profile-stats > div {
  min-width: 96px;
  padding: 12px 14px;
  border: 1px solid color-mix(in srgb, var(--brand-2) 24%, var(--border));
  border-radius: 18px;
  background:
    radial-gradient(circle at 18% 18%, color-mix(in srgb, var(--brand-2) 18%, transparent), transparent 36%),
    linear-gradient(135deg, color-mix(in srgb, var(--surface-2) 92%, transparent), color-mix(in srgb, var(--brand) 9%, var(--surface)));
  box-shadow: inset 0 1px 0 color-mix(in srgb, white 12%, transparent);
  text-align: center;
}

.profile-stats strong,
.profile-stats span {
  display: block;
}

.profile-stats strong {
  font-size: 24px;
  line-height: 1;
  color: var(--brand-2);
  letter-spacing: -0.02em;
}

.profile-stats span {
  margin-top: 7px;
  font-size: 12px;
  font-weight: 700;
}

.profile-logout {
  border-color: color-mix(in srgb, var(--danger) 58%, transparent);
  background: color-mix(in srgb, var(--danger) 14%, transparent);
  color: var(--danger);
}

.profile-logout:hover {
  background: color-mix(in srgb, var(--danger) 88%, #111827);
  border-color: color-mix(in srgb, var(--danger) 88%, transparent);
  color: white;
  box-shadow: 0 14px 28px color-mix(in srgb, var(--danger) 20%, transparent);
}

.profile-fit-card {
  margin-top: 18px;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}

.section-head h2,
.section-head p {
  margin: 0;
}

.section-head .muted {
  margin-top: 6px;
}

.eyebrow {
  margin: 0 0 6px;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.18em;
  color: var(--brand-2);
}

.fit-scope-tabs {
  --fit-scope-pad: 4px;
  --fit-scope-indicator-width: 0px;
  --fit-scope-indicator-x: 0px;
  display: inline-flex;
  position: relative;
  gap: 0;
  max-width: 100%;
  overflow-x: auto;
  padding: var(--fit-scope-pad);
  margin-bottom: 14px;
  border: 1px solid var(--border);
  border-radius: 18px;
  background: color-mix(in srgb, var(--surface-2) 76%, transparent);
  scrollbar-width: thin;
}

.fit-scope-tabs::before {
  content: '';
  position: absolute;
  top: var(--fit-scope-pad);
  bottom: var(--fit-scope-pad);
  left: 0;
  width: var(--fit-scope-indicator-width);
  border-radius: 14px;
  background: linear-gradient(135deg, color-mix(in srgb, var(--brand) 34%, transparent), color-mix(in srgb, var(--brand-2) 22%, transparent));
  box-shadow: 0 10px 24px color-mix(in srgb, var(--brand-2) 14%, transparent);
  transform: translateX(var(--fit-scope-indicator-x));
  transition:
    transform 0.24s ease,
    width 0.24s ease;
  pointer-events: none;
}

.fit-scope-btn {
  position: relative;
  z-index: 1;
  border: 0;
  border-radius: 14px;
  background: transparent;
  color: var(--text);
  padding: 9px 13px;
  white-space: nowrap;
  box-shadow: none;
  transition:
    color 0.18s ease,
    background 0.18s ease;
}

.fit-scope-btn:hover {
  transform: none;
  box-shadow: none;
  color: var(--text-primary);
}

.fit-scope-btn--active {
  color: var(--text-primary);
}

.fit-model-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.fit-model-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px;
  border: 1px solid var(--border);
  border-radius: 18px;
  background: color-mix(in srgb, var(--surface-2) 62%, transparent);
}

.fit-model-card h3 {
  margin: 0 0 6px;
  font-size: 16px;
}

.fit-model-card p {
  margin: 0;
  font-size: 13px;
}

.history-refresh {
  padding: 8px 13px;
}

.history-list {
  display: grid;
  gap: 14px;
}

.history-session {
  border: 1px solid var(--border);
  border-radius: 18px;
  background: color-mix(in srgb, var(--surface-2) 62%, transparent);
  overflow: visible;
}

.history-session-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--border);
}

.history-session-head h3 {
  margin: 0 0 4px;
}

.history-items {
  display: grid;
}

.history-item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(180px, 0.35fr);
  gap: 16px;
  padding: 14px 16px;
  border-bottom: 1px solid color-mix(in srgb, var(--border) 72%, transparent);
}

.history-item:last-child {
  border-bottom: 0;
}

.history-item-main {
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

.history-question {
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

.history-question-wrap {
  position: relative;
  min-width: 0;
}

.answer-preview {
  position: absolute;
  left: 0;
  top: calc(100% + 10px);
  z-index: 20;
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

.history-question-wrap:hover .answer-preview,
.history-question-wrap:focus-within .answer-preview {
  opacity: 1;
  transform: translateY(0);
}

.answer-preview-col {
  max-height: 260px;
  overflow: auto;
  padding: 10px;
  border-radius: 14px;
  background: color-mix(in srgb, var(--surface-2) 76%, transparent);
}

.answer-preview-col strong {
  display: block;
  margin-bottom: 8px;
  color: var(--brand-2);
}

.answer-preview-col p {
  margin: 0;
  white-space: pre-wrap;
  line-height: 1.55;
  font-size: 13px;
}

.history-matchup {
  margin: 0;
  font-size: 13px;
}

.history-result {
  display: grid;
  align-content: start;
  justify-items: end;
  gap: 4px;
  text-align: right;
}

.history-outcome {
  display: inline-flex;
  padding: 4px 9px;
  border-radius: 999px;
  color: var(--brand-2);
  background: color-mix(in srgb, var(--brand-2) 12%, transparent);
  font-size: 12px;
  font-weight: 800;
}

.history-result strong {
  font-size: 14px;
}

.history-pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  margin-top: 16px;
}

@media (max-width: 760px) {
  .section-head {
    align-items: flex-start;
    flex-direction: column;
  }

  .history-item {
    grid-template-columns: 1fr;
  }

  .profile-card--compact,
  .fit-model-grid {
    grid-template-columns: 1fr;
  }

  .profile-stats {
    width: 100%;
  }

  .profile-stats > div {
    flex: 1;
  }

  .answer-preview {
    grid-template-columns: 1fr;
  }

  .history-result {
    justify-items: start;
    text-align: left;
    padding-left: 54px;
  }
}
</style>
