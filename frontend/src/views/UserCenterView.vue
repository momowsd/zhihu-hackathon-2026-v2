<script setup lang="ts">
import { computed } from 'vue'
import { clearTokens, useAuth } from '../auth'
import { router } from '../router'

const auth = useAuth()
const roleLabel = computed(() => (auth.isAdmin.value ? '管理员' : '普通用户'))

function logout() {
  clearTokens()
  router.push('/')
}
</script>

<template>
  <div class="page">
    <div class="header">
      <h1>个人中心</h1>
      <p class="muted">查看当前登录身份。后续接入 OAuth 后，这里可以承载统一账号信息。</p>
    </div>
    <div class="card profile-card">
      <div class="avatar">{{ auth.username.value.slice(0, 1).toUpperCase() || 'U' }}</div>
      <div>
        <div class="muted">用户名</div>
        <h2>{{ auth.username.value }}</h2>
        <p class="muted">角色：{{ roleLabel }}</p>
      </div>
      <button class="ghost" @click="logout">退出登录</button>
    </div>
  </div>
</template>
