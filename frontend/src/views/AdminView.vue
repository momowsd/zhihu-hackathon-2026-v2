<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { api, errorMessage, type ApiEnvelope, type Category, type Model, type ModelAnswer, type Question } from '../api'

const error = ref('')
const categories = ref<Category[]>([])
const questions = ref<Question[]>([])
const models = ref<Model[]>([])
const answers = ref<ModelAnswer[]>([])

const categoryForm = reactive({ code: '', name: '', description: '', enabled: true, sortOrder: 10 })
const questionForm = reactive({ categoryId: '', prompt: '', source: 'admin', difficulty: 'normal', enabled: true })
const modelForm = reactive({ provider: '', name: '', displayName: '', version: '', isBaseline: true, enabled: true })
const answerForm = reactive({ questionId: '', modelId: '', answerText: '', metadataJson: '{}' })

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

async function submit(path: string, payload: unknown) {
  error.value = ''
  try {
    await api.post(path, payload)
    await loadAll()
  } catch (err) {
    error.value = errorMessage(err)
  }
}

onMounted(loadAll)
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
    <div class="card" style="margin-top: 16px">
      <h2>数据概览</h2>
      <p class="muted">分类 {{ categories.length }} / 题目 {{ questions.length }} / 模型 {{ models.length }} / 回答 {{ answers.length }}</p>
    </div>
  </div>
</template>
