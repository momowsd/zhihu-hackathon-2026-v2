<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type ApiEnvelope } from '../api'

type Summary = {
  users: number
  votes: number
  questions: number
  models: number
  categories: Array<{ id: string; name: string; description: string }>
  topModels: Array<{ display_name: string; elo_rating: number; vote_count: number }>
}

const summary = ref<Summary | null>(null)

onMounted(async () => {
  const response = await api.get<ApiEnvelope<Summary>>('/dashboard/summary')
  summary.value = response.data.data
})
</script>

<template>
  <div class="page">
    <div class="header">
      <h1>Dashboard</h1>
      <p class="muted">查看当前盲评数据、分类和模型榜单概览。</p>
    </div>
    <div v-if="summary" class="grid">
      <div class="card"><div class="muted">用户数</div><div class="stat">{{ summary.users }}</div></div>
      <div class="card"><div class="muted">投票数</div><div class="stat">{{ summary.votes }}</div></div>
      <div class="card"><div class="muted">题目数</div><div class="stat">{{ summary.questions }}</div></div>
      <div class="card"><div class="muted">模型数</div><div class="stat">{{ summary.models }}</div></div>
    </div>
    <div class="grid" style="margin-top: 16px">
      <div class="card">
        <h2>评估主题</h2>
        <p v-for="cat in summary?.categories" :key="cat.id">
          <strong>{{ cat.name }}</strong><br />
          <span class="muted">{{ cat.description }}</span>
        </p>
      </div>
      <div class="card">
        <h2>Top Models</h2>
        <p v-if="!summary?.topModels?.length" class="muted">暂无投票，完成一次盲评后会出现排名。</p>
        <p v-for="model in summary?.topModels" :key="model.display_name">
          <strong>{{ model.display_name }}</strong>
          <span class="muted"> Elo {{ Number(model.elo_rating).toFixed(0) }} / {{ model.vote_count }} 票</span>
        </p>
      </div>
    </div>
  </div>
</template>
