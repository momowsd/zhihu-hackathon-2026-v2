<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { errorMessage, login, register, startZhihuOAuth } from '../api'
import { useRoute } from 'vue-router'
import { router } from '../router'

const username = ref('demo')
const password = ref('user123')
const mode = ref<'login' | 'register'>('login')
const error = ref('')
const loading = ref(false)
const zhihuLoading = ref(false)
const route = useRoute()

onMounted(() => {
  const oauthError = route.query.oauth_error
  if (typeof oauthError === 'string' && oauthError) {
    error.value = oauthError
  }
})

async function submit() {
  loading.value = true
  error.value = ''
  try {
    if (mode.value === 'login') await login(username.value, password.value)
    else await register(username.value, password.value)
    router.push('/')
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

async function loginWithZhihu() {
  zhihuLoading.value = true
  error.value = ''
  try {
    const authorizeUrl = await startZhihuOAuth()
    window.location.href = authorizeUrl
  } catch (err) {
    error.value = errorMessage(err)
    zhihuLoading.value = false
  }
}
</script>

<template>
  <div class="page" style="max-width: 460px; padding-top: 10vh">
    <div class="card">
      <div class="header">
        <h1>模型盲评 Arena</h1>
        <p class="muted">默认账号：demo / user123</p>
      </div>
      <form class="form" @submit.prevent="submit">
        <input v-model="username" placeholder="用户名" />
        <input v-model="password" placeholder="密码" type="password" />
        <p v-if="error" class="error">{{ error }}</p>
        <button :disabled="loading">{{ mode === 'login' ? '登录' : '注册' }}</button>
        <button class="ghost zhihu-login" type="button" :disabled="zhihuLoading" @click="loginWithZhihu">
          {{ zhihuLoading ? '正在跳转知乎...' : '使用知乎账号登录' }}
        </button>
        <button class="ghost" type="button" @click="mode = mode === 'login' ? 'register' : 'login'">
          切换到{{ mode === 'login' ? '注册' : '登录' }}
        </button>
      </form>
    </div>
  </div>
</template>

<style scoped>
.zhihu-login {
  border-color: color-mix(in srgb, #0084ff 45%, var(--border)) !important;
  color: #0084ff !important;
}
</style>
