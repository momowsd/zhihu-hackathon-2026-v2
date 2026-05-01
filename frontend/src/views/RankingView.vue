<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { api, loadCategories, type ApiEnvelope, type Category } from '../api'

type RankRow = {
  modelId: string
  displayName: string
  provider: string
  voteCount: number
  winCount: number
  lossCount: number
  eloRating: number
  lastEloDelta: number
  winRate: number
}

const categories = ref<Category[]>([])
const categoryId = ref('')
const rows = ref<RankRow[]>([])

async function load() {
  const response = await api.get<ApiEnvelope<RankRow[]>>('/rankings', { params: { categoryId: categoryId.value || undefined } })
  rows.value = response.data.data
}

onMounted(async () => {
  categories.value = await loadCategories()
  await load()
})

watch(categoryId, load)
</script>

<template>
  <div class="page">
    <div class="header">
      <h1>Ranking</h1>
      <p class="muted">Elo 为主排序指标，同时展示胜率和票数。</p>
    </div>
    <div class="card">
      <label>分类筛选<select v-model="categoryId"><option value="">全部</option><option v-for="cat in categories" :key="cat.id" :value="cat.id">{{ cat.name }}</option></select></label>
    </div>
    <table class="table" style="margin-top: 16px">
      <thead>
        <tr><th>#</th><th>模型</th><th>Provider</th><th>Elo</th><th>胜率</th><th>票数</th><th>最近变化</th></tr>
      </thead>
      <tbody>
        <tr v-for="(row, index) in rows" :key="row.modelId + index">
          <td>{{ index + 1 }}</td>
          <td>{{ row.displayName }}</td>
          <td>{{ row.provider }}</td>
          <td>{{ row.eloRating.toFixed(0) }}</td>
          <td>{{ (row.winRate * 100).toFixed(1) }}%</td>
          <td>{{ row.voteCount }}</td>
          <td>{{ row.lastEloDelta > 0 ? '+' : '' }}{{ row.lastEloDelta.toFixed(1) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
