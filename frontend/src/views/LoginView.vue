<script setup lang="ts">
import { ref } from 'vue'
import { errorMessage, login, register } from '../api'
import { router } from '../router'

const username = ref('demo')
const password = ref('user123')
const mode = ref<'login' | 'register'>('login')
const error = ref('')
const loading = ref(false)

async function submit() {
  loading.value = true
  error.value = ''
  try {
    if (mode.value === 'login') await login(username.value, password.value)
    else await register(username.value, password.value)
    router.push('/dashboard')
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="page" style="max-width: 460px; padding-top: 10vh">
    <div class="card">
      <div class="header">
        <h1>模型盲评 Arena</h1>
        <p class="muted">默认账号：demo / user123，管理员：admin / admin123</p>
      </div>
      <form class="form" @submit.prevent="submit">
        <input v-model="username" placeholder="用户名" />
        <input v-model="password" placeholder="密码" type="password" />
        <p v-if="error" class="error">{{ error }}</p>
        <button :disabled="loading">{{ mode === 'login' ? '登录' : '注册' }}</button>
        <button class="ghost" type="button" @click="mode = mode === 'login' ? 'register' : 'login'">
          切换到{{ mode === 'login' ? '注册' : '登录' }}
        </button>
      </form>
    </div>
  </div>
</template>
