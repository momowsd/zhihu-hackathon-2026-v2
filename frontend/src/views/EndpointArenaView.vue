<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, errorMessage, loadCategories, type ApiEnvelope, type Category } from '../api'
import { router } from '../router'

const categories = ref<Category[]>([])
const form = ref({
  name: '我的模型',
  baseUrl: '',
  modelName: '',
  apiKey: '',
  categoryId: '',
  count: 1,
})
const sample = ref('')
const error = ref('')
const loading = ref(false)

onMounted(async () => {
  categories.value = await loadCategories()
  form.value.categoryId = categories.value[0]?.id ?? ''
})

async function validate() {
  loading.value = true
  error.value = ''
  sample.value = ''
  try {
    const response = await api.post<ApiEnvelope<{ sampleAnswer: string }>>('/arena/endpoints/validate', form.value)
    sample.value = response.data.data.sampleAnswer
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

async function createSession() {
  loading.value = true
  error.value = ''
  try {
    const response = await api.post<ApiEnvelope<{ id: string }>>('/arena/sessions', form.value)
    router.push({ name: 'eval', query: { arenaSessionId: response.data.data.id } })
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="page">
    <div class="header">
      <h1>Endpoint Arena</h1>
      <p class="muted">提交 OpenAI Chat Completions 兼容 endpoint，密钥只用于本次请求，不会落库。</p>
    </div>
    <div class="card form">
      <div class="row">
        <label>显示名<input v-model="form.name" /></label>
        <label>模型名<input v-model="form.modelName" placeholder="gpt-4o-mini" /></label>
      </div>
      <label>Base URL<input v-model="form.baseUrl" placeholder="https://example.com/v1" /></label>
      <label>Bearer/API Key<input v-model="form.apiKey" type="password" placeholder="只在本次请求内使用" /></label>
      <label>盲评主题<select v-model="form.categoryId"><option v-for="cat in categories" :key="cat.id" :value="cat.id">{{ cat.name }}</option></select></label>
      <p v-if="error" class="error">{{ error }}</p>
      <p v-if="sample" class="muted">连通性样例：{{ sample }}</p>
      <div class="row">
        <button class="ghost" :disabled="loading" @click="validate">校验 endpoint</button>
        <button :disabled="loading" @click="createSession">生成盲评对局</button>
      </div>
    </div>
  </div>
</template>
