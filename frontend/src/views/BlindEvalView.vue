<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, errorMessage, loadCategories, type ApiEnvelope, type Category } from '../api'
import { useRoute } from 'vue-router'

type Session = { id: string; requestedCount: number }
type EvalItem = {
  completed: boolean
  itemId?: string
  question?: { id: string; prompt: string }
  left?: { answerId: string; text: string }
  right?: { answerId: string; text: string }
}

const categories = ref<Category[]>([])
const categoryId = ref('')
const count = ref(5)
const session = ref<Session | null>(null)
const item = ref<EvalItem | null>(null)
const confidence = ref(3)
const error = ref('')
const route = useRoute()

onMounted(async () => {
  categories.value = await loadCategories()
  categoryId.value = categories.value[0]?.id ?? ''
  if (typeof route.query.arenaSessionId === 'string') {
    session.value = { id: route.query.arenaSessionId, requestedCount: 1 }
    await loadNext()
  }
})

async function start() {
  error.value = ''
  try {
    const response = await api.post<ApiEnvelope<Session>>('/eval/sessions', { categoryId: categoryId.value, count: count.value })
    session.value = response.data.data
    await loadNext()
  } catch (err) {
    error.value = errorMessage(err)
  }
}

async function loadNext() {
  if (!session.value) return
  const response = await api.get<ApiEnvelope<EvalItem>>(`/eval/sessions/${session.value.id}/next`)
  item.value = response.data.data
}

async function vote(winnerSide: 'left' | 'right') {
  if (!item.value?.itemId) return
  await api.post('/eval/votes', { itemId: item.value.itemId, winnerSide, confidenceScore: confidence.value })
  await loadNext()
}
</script>

<template>
  <div class="page">
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
    <div v-else-if="item?.completed" class="card">
      <h2>本轮已完成</h2>
      <p class="muted">可以去 Ranking 查看 Elo 变化，或重新开始一轮。</p>
      <button @click="session = null; item = null">再来一轮</button>
    </div>
    <div v-else-if="item" class="form">
      <div class="card">
        <div class="muted">题目</div>
        <h2>{{ item.question?.prompt }}</h2>
        <label>差距/信心分：{{ confidence }}<input v-model.number="confidence" type="range" min="1" max="5" /></label>
      </div>
      <div class="grid">
        <div class="card answer-card">
          <h3>回答 A</h3>
          <p>{{ item.left?.text }}</p>
          <button @click="vote('left')">A 更好</button>
        </div>
        <div class="card answer-card">
          <h3>回答 B</h3>
          <p>{{ item.right?.text }}</p>
          <button @click="vote('right')">B 更好</button>
        </div>
      </div>
    </div>
  </div>
</template>
