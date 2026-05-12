<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { clearTokens, useAuth } from './auth'
import ThemeToggle from './components/ThemeToggle.vue'

const auth = useAuth()
const route = useRoute()
const router = useRouter()
const profileOpen = ref(false)

const navItems = computed(() => [
  { to: '/eval', label: 'Battle', show: auth.isAuthed.value },
  { to: '/rankings', label: 'Ranking', show: auth.isAuthed.value },
  { to: '/dashboard', label: 'Dashboard', show: auth.isAuthed.value },
  { to: '/arena', label: 'Endpoint Arena', show: auth.isAuthed.value },
  { to: '/admin', label: 'Admin', show: auth.isAdmin.value },
])

watch(
  () => route.fullPath,
  () => {
    profileOpen.value = false
  },
)

function logout() {
  clearTokens()
  profileOpen.value = false
  router.push('/')
}
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <RouterLink to="/" class="brand" aria-label="回到首页">
        <span class="brand-mark"></span>
        <span class="brand-text">看山模型竞技场</span>
      </RouterLink>
      <nav class="top-nav">
        <RouterLink
          v-for="item in navItems.filter((x) => x.show)"
          :key="item.to"
          :to="item.to"
          class="top-link"
        >
          {{ item.label }}
        </RouterLink>
      </nav>
      <div class="top-actions">
        <span class="theme-inline" title="切换亮色 / 暗色模式">
          <ThemeToggle />
        </span>
        <RouterLink v-if="!auth.isAuthed.value" to="/login" class="login-link">登录</RouterLink>
        <div v-else class="profile-menu">
          <button class="profile-trigger" type="button" @click="profileOpen = !profileOpen">
            <span class="profile-avatar">{{ auth.username.value.slice(0, 1).toUpperCase() }}</span>
            <span>{{ auth.username.value }}</span>
          </button>
          <div v-if="profileOpen" class="profile-dropdown">
            <RouterLink to="/user" class="dropdown-item">个人中心</RouterLink>
            <button class="dropdown-item danger" type="button" @click="logout">退出登录</button>
          </div>
        </div>
      </div>
    </header>
    <main class="main">
      <RouterView />
    </main>
  </div>
</template>
