<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { errorMessage, login, register, startZhihuOAuth } from '../api'
import { useRoute } from 'vue-router'
import { router } from '../router'

const username = ref('')
const password = ref('')
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
        <h1>看山模型竞技场</h1>
      </div>
      <form class="form" @submit.prevent="submit">
        <p v-if="error" class="error">{{ error }}</p>
        <p class="login-zhihu-hint">推荐使用知乎账号登录</p>
        <button
          class="zhihu-login-primary"
          type="button"
          :disabled="zhihuLoading"
          @click="loginWithZhihu"
        >
          <span class="zhihu-login-icon" aria-hidden="true" />
          <span>{{ zhihuLoading ? '正在跳转知乎…' : '使用知乎账号登录' }}</span>
        </button>
        <p class="login-divider muted">或使用账号密码</p>
        <input v-model="username" placeholder="用户名" />
        <input v-model="password" placeholder="密码" type="password" />
        <button type="submit" class="secondary" :disabled="loading">
          {{ mode === 'login' ? '登录' : '注册' }}
        </button>
        <button class="ghost" type="button" @click="mode = mode === 'login' ? 'register' : 'login'">
          切换到{{ mode === 'login' ? '注册' : '登录' }}
        </button>
      </form>
    </div>
  </div>
</template>

<style scoped>
.login-zhihu-hint {
  margin: 4px 0 8px;
  font-size: 13px;
  font-weight: 650;
  line-height: 1.45;
  text-align: center;
  color: color-mix(in srgb, var(--brand-2) 88%, var(--text-secondary));
}

.login-divider {
  margin: 18px 0 10px;
  font-size: 12px;
  font-weight: 600;
  text-align: center;
}

.zhihu-login-primary {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  width: 100%;
  border: 0 !important;
  background: linear-gradient(135deg, #0084ff 0%, #0066cc 55%, #00a8e8 100%) !important;
  color: #fff !important;
  box-shadow: 0 12px 28px color-mix(in srgb, #0084ff 32%, transparent) !important;
}

/* 四视图横排第 3 格（侧视），水平镜像；与顶栏 brand-mark 同源雪碧 */
.zhihu-login-icon {
  flex-shrink: 0;
  width: 22px;
  height: 22px;
  background-color: transparent;
  background-image: url('/assets/kanshan_imgs/%E5%88%98%E7%9C%8B%E5%B1%B1%E5%9B%B4%E8%84%96%E5%9B%9B%E8%A7%86%E5%9B%BE.png');
  background-repeat: no-repeat;
  background-size: 400% 100%;
  background-position: 66.666% calc(50% + 2px);
  transform: scaleX(-1);
}

.zhihu-login-primary:hover:not(:disabled) {
  color: #fff !important;
  box-shadow: 0 14px 32px color-mix(in srgb, #0084ff 40%, transparent) !important;
}

.zhihu-login-primary:disabled {
  opacity: 0.65;
}
</style>
