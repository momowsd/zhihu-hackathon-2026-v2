<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { loadCategories, type Category } from '../api'

const categories = ref<Category[]>([])
const form = ref({
  name: '我的模型',
  baseUrl: '',
  modelName: '',
  apiKey: '',
  categoryId: '',
  count: 1,
})
const devNoticeOpen = ref(false)

onMounted(async () => {
  categories.value = await loadCategories()
  form.value.categoryId = categories.value[0]?.id ?? ''
})

function openDevNotice() {
  devNoticeOpen.value = true
}

function closeDevNotice() {
  devNoticeOpen.value = false
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
      <div class="row">
        <button type="button" class="ghost" @click="openDevNotice">校验 endpoint</button>
        <button type="button" @click="openDevNotice">生成盲评对局</button>
      </div>
    </div>

    <Teleport to="body">
      <div
        v-if="devNoticeOpen"
        class="arena-modal-root"
        role="dialog"
        aria-modal="true"
        aria-labelledby="arena-dev-title"
      >
        <div class="arena-modal-backdrop" @click="closeDevNotice"></div>
        <div class="card arena-modal">
          <h3 id="arena-dev-title">功能提示</h3>
          <p class="arena-modal-body">
            此功能开发中，暂时未对外开放。当前不会发起校验或生成对局等任何后台操作。
          </p>
          <div class="arena-modal-actions">
            <button type="button" @click="closeDevNotice">我知道了</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.arena-modal-root {
  position: fixed;
  inset: 0;
  z-index: 80;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
}

.arena-modal-backdrop {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
}

.arena-modal {
  position: relative;
  z-index: 1;
  max-width: 440px;
  width: 100%;
  padding: 22px;
}

.arena-modal h3 {
  margin: 0 0 12px;
}

.arena-modal-body {
  margin: 0;
  line-height: 1.65;
  color: var(--text-secondary);
}

.arena-modal-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 20px;
}
</style>
