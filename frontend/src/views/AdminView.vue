<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  adminBrowseTable,
  api,
  errorMessage,
  type ApiEnvelope,
  type Category,
  type Model,
  type ModelAnswer,
  type Question,
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

const browseTotalPages = computed(() => Math.max(1, Math.ceil(browseTotal.value / browsePageSize.value)))

const browseColumns = computed(() => {
  const row = browseItems.value[0]
  if (!row) return []
  return Object.keys(row).sort((a, b) => a.localeCompare(b))
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

watch([browseTable, browsePage, browsePageSize], ([table, , size], prev) => {
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
      </div>

      <p v-if="browseErr" class="error">{{ browseErr }}</p>

      <div class="data-table-scroll">
        <table v-if="browseColumns.length" class="table data-overview-table">
          <thead>
            <tr>
              <th v-for="col in browseColumns" :key="col">{{ col }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, ri) in browseItems" :key="ri">
              <td v-for="col in browseColumns" :key="col" class="browse-cell" :title="formatCell(row[col])">
                {{ formatCell(row[col]) }}
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
  </div>
</template>
