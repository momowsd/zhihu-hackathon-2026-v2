<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  adminBrowseTable,
  adminDeleteQuestionsBatch,
  adminImportBundle,
  adminQuestionDeleteImpact,
  adminQuestionDeleteImpactBatch,
  api,
  errorMessage,
  importBundleErrorDetail,
  type ApiEnvelope,
  type Category,
  type ImportFieldError,
  type Model,
  type ModelAnswer,
  type Question,
  type QuestionDeleteImpact,
} from '../api'

const error = ref('')
const categories = ref<Category[]>([])
const questions = ref<Question[]>([])
const models = ref<Model[]>([])
const answers = ref<ModelAnswer[]>([])

const categoryForm = reactive({ code: '', name: '', description: '', enabled: true, sortOrder: 10 })
const questionForm = reactive({ categoryId: '', prompt: '', source: 'admin', difficulty: 'normal', enabled: true })
const modelForm = reactive({ provider: '', name: '', displayName: '', version: '', isBaseline: true, enabled: true })
const answerForm = reactive({ questionId: '', modelId: '', answerText: '', metadataJson: '{}' })

const browseTables = [
  { value: 'users', label: 'users（用户）' },
  { value: 'eval_categories', label: 'eval_categories（评估分类）' },
  { value: 'questions', label: 'questions（题目）' },
  { value: 'models', label: 'models（模型）' },
  { value: 'model_answers', label: 'model_answers（模型回答）' },
  { value: 'eval_sessions', label: 'eval_sessions（评测会话）' },
  { value: 'eval_items', label: 'eval_items（评测小题）' },
  { value: 'eval_votes', label: 'eval_votes（投票）' },
  { value: 'model_stats', label: 'model_stats（模型统计）' },
  { value: 'submitted_endpoints', label: 'submitted_endpoints（提交的 Endpoint）' },
] as const

const browseTable = ref<string>(browseTables[0].value)
const browsePage = ref(1)
const browsePageSize = ref(20)
const browseLoading = ref(false)
const browseErr = ref('')
const browseTotal = ref(0)
const browseItems = ref<Record<string, unknown>[]>([])

const selectedQuestionIds = ref<Set<string>>(new Set())
const deleteModalOpen = ref(false)
const deleteImpactLoading = ref(false)
const deleteImpact = ref<QuestionDeleteImpact | null>(null)
const pendingDeleteIds = ref<string[]>([])
const importLoading = ref(false)
const importErr = ref('')
const importFieldErrors = ref<ImportFieldError[]>([])
const importOkMsg = ref('')
const formatOpen = ref(false)

const browseTotalPages = computed(() => Math.max(1, Math.ceil(browseTotal.value / browsePageSize.value)))

const isQuestionsTable = computed(() => browseTable.value === 'questions')

const browseColumns = computed(() => {
  const row = browseItems.value[0]
  if (!row) return []
  return Object.keys(row).sort((a, b) => a.localeCompare(b))
})

const questionIdsOnPage = computed(() =>
  browseItems.value.map((row) => String(row.id ?? '')).filter(Boolean),
)

const allQuestionsOnPageSelected = computed(() => {
  if (!questionIdsOnPage.value.length) return false
  return questionIdsOnPage.value.every((id) => selectedQuestionIds.value.has(id))
})

async function loadAll() {
  const [catRes, questionRes, modelRes, answerRes] = await Promise.all([
    api.get<ApiEnvelope<Category[]>>('/admin/categories'),
    api.get<ApiEnvelope<Question[]>>('/admin/questions'),
    api.get<ApiEnvelope<Model[]>>('/admin/models'),
    api.get<ApiEnvelope<ModelAnswer[]>>('/admin/answers'),
  ])
  categories.value = catRes.data.data
  questions.value = questionRes.data.data
  models.value = modelRes.data.data
  answers.value = answerRes.data.data
  questionForm.categoryId ||= categories.value[0]?.id ?? ''
  answerForm.questionId ||= questions.value[0]?.id ?? ''
  answerForm.modelId ||= models.value[0]?.id ?? ''
}

async function loadBrowse() {
  browseErr.value = ''
  browseLoading.value = true
  try {
    const res = await adminBrowseTable(browseTable.value, browsePage.value, browsePageSize.value)
    browseTotal.value = res.total
    browseItems.value = (res.items ?? []) as Record<string, unknown>[]
  } catch (err) {
    browseErr.value = errorMessage(err)
    browseItems.value = []
    browseTotal.value = 0
  } finally {
    browseLoading.value = false
  }
}

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'boolean') return v ? 'true' : 'false'
  if (typeof v === 'object') return JSON.stringify(v)
  const s = String(v)
  return s.length > 400 ? `${s.slice(0, 400)}…` : s
}

async function submit(path: string, payload: unknown) {
  error.value = ''
  try {
    await api.post(path, payload)
    await loadAll()
    await loadBrowse()
  } catch (err) {
    error.value = errorMessage(err)
  }
}

function goBrowsePage(p: number) {
  const next = Math.min(Math.max(1, p), browseTotalPages.value)
  browsePage.value = next
}

function toggleQuestionSelect(id: string) {
  const next = new Set(selectedQuestionIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedQuestionIds.value = next
}

function toggleSelectAllQuestionsOnPage() {
  if (allQuestionsOnPageSelected.value) {
    const next = new Set(selectedQuestionIds.value)
    for (const id of questionIdsOnPage.value) next.delete(id)
    selectedQuestionIds.value = next
  } else {
    const next = new Set(selectedQuestionIds.value)
    for (const id of questionIdsOnPage.value) next.add(id)
    selectedQuestionIds.value = next
  }
}

async function openDeleteModal(ids: string[]) {
  if (!ids.length) return
  pendingDeleteIds.value = ids
  deleteImpact.value = null
  deleteModalOpen.value = true
  deleteImpactLoading.value = true
  browseErr.value = ''
  try {
    deleteImpact.value =
      ids.length === 1
        ? await adminQuestionDeleteImpact(ids[0])
        : await adminQuestionDeleteImpactBatch(ids)
  } catch (err) {
    browseErr.value = errorMessage(err)
    deleteModalOpen.value = false
  } finally {
    deleteImpactLoading.value = false
  }
}

async function confirmDeleteQuestions() {
  error.value = ''
  try {
    await adminDeleteQuestionsBatch(pendingDeleteIds.value)
    deleteModalOpen.value = false
    selectedQuestionIds.value = new Set()
    await loadAll()
    await loadBrowse()
  } catch (err) {
    error.value = errorMessage(err)
  }
}

function cancelDeleteModal() {
  deleteModalOpen.value = false
  pendingDeleteIds.value = []
  deleteImpact.value = null
}

const exampleBundleUrl = `${import.meta.env.BASE_URL.replace(/\/?$/, '/') }import-bundle.example.json`

async function onImportFile(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  importErr.value = ''
  importFieldErrors.value = []
  importOkMsg.value = ''
  importLoading.value = true
  try {
    const text = await file.text()
    const parsed = JSON.parse(text) as unknown
    await adminImportBundle(parsed)
    importOkMsg.value = '导入成功'
    await loadAll()
    await loadBrowse()
  } catch (err) {
    const d = importBundleErrorDetail(err)
    importErr.value = d.message
    importFieldErrors.value = d.errors ?? []
  } finally {
    importLoading.value = false
  }
}

watch([browseTable, browsePage, browsePageSize], ([table, , size], prev) => {
  selectedQuestionIds.value = new Set()
  if (prev && table !== prev[0]) {
    if (browsePage.value !== 1) {
      browsePage.value = 1
      return
    }
  }
  if (prev && size !== prev[2]) {
    if (browsePage.value !== 1) {
      browsePage.value = 1
      return
    }
  }
  loadBrowse()
})

onMounted(async () => {
  await loadAll()
  await loadBrowse()
})
</script>

<template>
  <div class="page">
    <div class="header">
      <h1>Admin</h1>
      <p class="muted">维护评估分类、题目、模型和回答。第一版提供基础新增和列表能力。</p>
    </div>
    <p v-if="error" class="error">{{ error }}</p>
    <div class="grid">
      <div class="card form">
        <h2>新增分类</h2>
        <input v-model="categoryForm.code" placeholder="code" />
        <input v-model="categoryForm.name" placeholder="名称" />
        <textarea v-model="categoryForm.description" placeholder="描述"></textarea>
        <input v-model.number="categoryForm.sortOrder" type="number" />
        <button @click="submit('/admin/categories', categoryForm)">保存分类</button>
      </div>
      <div class="card form">
        <h2>新增题目</h2>
        <select v-model="questionForm.categoryId"><option v-for="cat in categories" :key="cat.id" :value="cat.id">{{ cat.name }}</option></select>
        <textarea v-model="questionForm.prompt" placeholder="题目 prompt"></textarea>
        <input v-model="questionForm.difficulty" />
        <button @click="submit('/admin/questions', questionForm)">保存题目</button>
      </div>
      <div class="card form">
        <h2>新增模型</h2>
        <input v-model="modelForm.provider" placeholder="provider" />
        <input v-model="modelForm.name" placeholder="name" />
        <input v-model="modelForm.displayName" placeholder="display name" />
        <input v-model="modelForm.version" placeholder="version" />
        <button @click="submit('/admin/models', modelForm)">保存模型</button>
      </div>
      <div class="card form">
        <h2>新增回答</h2>
        <select v-model="answerForm.questionId"><option v-for="q in questions" :key="q.id" :value="q.id">{{ q.prompt.slice(0, 40) }}</option></select>
        <select v-model="answerForm.modelId"><option v-for="m in models" :key="m.id" :value="m.id">{{ m.displayName }}</option></select>
        <textarea v-model="answerForm.answerText" placeholder="模型回答"></textarea>
        <button @click="submit('/admin/answers', answerForm)">保存回答</button>
      </div>
    </div>
    <div class="card data-overview-card" style="margin-top: 16px">
      <h2>数据概览</h2>
      <p class="muted summary-line">
        汇总：分类 {{ categories.length }} / 题目 {{ questions.length }} / 模型 {{ models.length }} / 回答 {{ answers.length }}
      </p>

      <section class="admin-import-block">
        <h3 class="admin-subhead">批量导入（JSON Bundle）</h3>
        <p class="muted admin-import-lead">
          上传符合格式的 JSON，单次事务写入：<strong>分类 → 模型 → 题目 → 回答</strong>。分类按 <code>code</code> upsert；模型按 <code>provider+name</code> 匹配则复用已有记录。
        </p>
        <details :open="formatOpen" class="admin-details" @toggle="formatOpen = ($event.target as HTMLDetailsElement).open">
          <summary>格式说明（version 必须为 1）</summary>
          <ul class="muted admin-format-list">
            <li><code>categories[]</code>：<code>code</code>、<code>name</code> 必填；可选 <code>description</code>、<code>enabled</code>、<code>sortOrder</code></li>
            <li><code>models[]</code>：<code>ref</code>（文件内引用）、<code>provider</code>、<code>name</code>、<code>displayName</code> 必填</li>
            <li><code>questions[]</code>：<code>ref</code>、<code>categoryCode</code>、<code>prompt</code> 必填；题目引用的分类须在本包 <code>categories</code> 中声明，或数据库已存在该 <code>code</code></li>
            <li><code>answers[]</code>：<code>questionRef</code>、<code>modelRef</code>、<code>answerText</code>；<code>metadataJson</code> 须为合法 JSON 字符串（可省略，默认 <code>{}</code>）</li>
          </ul>
        </details>
        <div class="admin-import-actions">
          <a class="link-btn admin-sample-link" :href="exampleBundleUrl" download="import-bundle.example.json">下载示例 JSON</a>
          <label class="admin-file-label">
            <input type="file" accept="application/json,.json" :disabled="importLoading" @change="onImportFile" />
            <span>{{ importLoading ? '导入中…' : '选择 JSON 文件' }}</span>
          </label>
        </div>
        <p v-if="importOkMsg" class="admin-ok">{{ importOkMsg }}</p>
        <p v-if="importErr" class="error">{{ importErr }}</p>
        <ul v-if="importFieldErrors.length" class="admin-import-errors">
          <li v-for="(fe, i) in importFieldErrors" :key="i">
            <strong>{{ fe.row }}</strong> · {{ fe.field }} — {{ fe.message }}
          </li>
        </ul>
      </section>

      <div class="data-overview-toolbar">
        <label class="browse-field">
          <span class="muted">数据表</span>
          <select v-model="browseTable">
            <option v-for="opt in browseTables" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
          </select>
        </label>
        <label class="browse-field">
          <span class="muted">每页</span>
          <select v-model.number="browsePageSize">
            <option :value="10">10</option>
            <option :value="20">20</option>
            <option :value="50">50</option>
            <option :value="100">100</option>
          </select>
        </label>
        <span v-if="browseLoading" class="muted">加载中…</span>
        <span v-else class="muted">共 {{ browseTotal }} 条 · 第 {{ browsePage }} / {{ browseTotalPages }} 页</span>
        <template v-if="isQuestionsTable">
          <button
            type="button"
            class="secondary admin-danger admin-danger-toolbar"
            :disabled="!selectedQuestionIds.size || browseLoading"
            @click="openDeleteModal([...selectedQuestionIds])"
          >
            删除选中（{{ selectedQuestionIds.size }}）
          </button>
        </template>
      </div>

      <p v-if="browseErr" class="error">{{ browseErr }}</p>

      <div class="data-table-scroll">
        <table v-if="browseColumns.length" class="table data-overview-table">
          <thead>
            <tr>
              <th v-if="isQuestionsTable" class="admin-col-check">
                <input type="checkbox" :checked="allQuestionsOnPageSelected" @change="toggleSelectAllQuestionsOnPage" />
              </th>
              <th v-for="col in browseColumns" :key="col">{{ col }}</th>
              <th v-if="isQuestionsTable" class="admin-col-action">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, ri) in browseItems" :key="String(row.id ?? ri)">
              <td v-if="isQuestionsTable" class="admin-col-check">
                <input
                  type="checkbox"
                  :checked="selectedQuestionIds.has(String(row.id))"
                  @change="toggleQuestionSelect(String(row.id))"
                />
              </td>
              <td v-for="col in browseColumns" :key="col" class="browse-cell" :title="formatCell(row[col])">
                {{ formatCell(row[col]) }}
              </td>
              <td v-if="isQuestionsTable" class="admin-col-action">
                <button type="button" class="secondary admin-danger-sm" @click="openDeleteModal([String(row.id)])">删除</button>
              </td>
            </tr>
          </tbody>
        </table>
        <p v-else-if="!browseLoading && !browseErr" class="muted">暂无数据</p>
      </div>

      <div v-if="browseTotal > 0" class="data-overview-pagination">
        <button type="button" class="secondary" :disabled="browsePage <= 1" @click="goBrowsePage(browsePage - 1)">上一页</button>
        <button type="button" class="secondary" :disabled="browsePage >= browseTotalPages" @click="goBrowsePage(browsePage + 1)">
          下一页
        </button>
      </div>
    </div>

    <Teleport to="body">
      <div v-if="deleteModalOpen" class="admin-modal-root" role="dialog" aria-modal="true" aria-labelledby="del-title">
        <div class="admin-modal-backdrop" @click="cancelDeleteModal"></div>
        <div class="card admin-modal">
          <h3 id="del-title">确认删除题目</h3>
          <p v-if="deleteImpactLoading" class="muted">正在统计关联数据…</p>
          <template v-else-if="deleteImpact">
            <p>
              将删除 <strong>{{ deleteImpact.questions }}</strong> 道题目，并<strong>同时删除</strong>关联数据：
            </p>
            <ul class="admin-impact-list">
              <li>投票（eval_votes）：<strong>{{ deleteImpact.votes }}</strong> 条</li>
              <li>评测小题（eval_items）：<strong>{{ deleteImpact.evalItems }}</strong> 条</li>
              <li>模型回答（model_answers）：<strong>{{ deleteImpact.modelAnswers }}</strong> 条</li>
            </ul>
            <p class="muted admin-stats-note">
              说明：<code>model_stats</code>（Elo 聚合）不会自动重算，删除后榜单数值可能与真实投票暂时不一致；后续可做全量重算运维。
            </p>
          </template>
          <div class="admin-modal-actions">
            <button type="button" class="secondary" @click="cancelDeleteModal">取消</button>
            <button type="button" class="admin-danger" :disabled="deleteImpactLoading || !deleteImpact" @click="confirmDeleteQuestions">
              确认删除
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.admin-subhead {
  margin: 0 0 8px;
  font-size: 1rem;
}

.admin-import-block {
  margin-bottom: 20px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--border);
}

.admin-import-lead {
  margin: 0 0 10px;
  line-height: 1.6;
}

.admin-import-lead code {
  font-size: 0.9em;
}

.admin-details {
  margin: 10px 0;
}

.admin-details summary {
  cursor: pointer;
  color: var(--brand-2);
  font-weight: 600;
}

.admin-format-list {
  margin: 8px 0 0;
  padding-left: 1.2em;
  line-height: 1.65;
}

.admin-format-list code {
  font-size: 0.85em;
}

.admin-import-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  margin-top: 12px;
}

.admin-sample-link {
  text-decoration: none;
}

.admin-file-label input {
  display: none;
}

.admin-file-label span {
  display: inline-flex;
  align-items: center;
  border: 1px solid var(--border);
  border-radius: 999px;
  padding: 8px 14px;
  cursor: pointer;
  font-weight: 600;
  background: color-mix(in srgb, var(--surface-2) 78%, transparent);
}

.admin-file-label span:hover {
  border-color: color-mix(in srgb, var(--brand-2) 45%, var(--border));
}

.admin-ok {
  margin: 10px 0 0;
  color: var(--brand);
  font-weight: 600;
}

.admin-import-errors {
  margin: 8px 0 0;
  padding-left: 1.2em;
  color: var(--danger);
  font-size: 13px;
}

.admin-col-check {
  width: 40px;
  text-align: center;
}

.admin-col-action {
  width: 88px;
  white-space: nowrap;
}

.admin-danger {
  border-color: color-mix(in srgb, var(--danger) 45%, var(--border)) !important;
  color: var(--danger) !important;
  background: color-mix(in srgb, var(--danger) 12%, transparent) !important;
}

.admin-danger-sm {
  font-size: 12px;
  padding: 6px 10px;
}

/* 数据概览工具栏「删除选中」：收敛尺寸与圆角 */
.admin-danger-toolbar {
  padding: 6px 12px;
  font-size: 13px;
  font-weight: 600;
  border-radius: 12px;
}

.admin-modal-root {
  position: fixed;
  inset: 0;
  z-index: 80;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
}

.admin-modal-backdrop {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
}

.admin-modal {
  position: relative;
  z-index: 1;
  max-width: 480px;
  width: 100%;
  padding: 22px;
}

.admin-impact-list {
  margin: 12px 0;
  padding-left: 1.2em;
}

.admin-stats-note {
  font-size: 13px;
  line-height: 1.55;
}

.admin-stats-note code {
  font-size: 0.88em;
}

.admin-modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
}
</style>
