<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { errorMessage, exchangeZhihuOAuthTicket } from '../api'
import { router } from '../router'

const route = useRoute()
const error = ref('')

onMounted(async () => {
  const ticket = route.query.ticket
  if (typeof ticket !== 'string' || !ticket) {
    error.value = '缺少知乎登录票据，请重新发起登录'
    return
  }

  try {
    await exchangeZhihuOAuthTicket(ticket)
    router.replace('/dashboard')
  } catch (err) {
    error.value = errorMessage(err)
  }
})
</script>

<template>
  <div class="page oauth-callback">
    <div class="card">
      <div class="header">
        <h1>知乎登录</h1>
        <p v-if="!error" class="muted">正在完成知乎 OAuth 登录，请稍候...</p>
        <p v-else class="error">{{ error }}</p>
      </div>
      <RouterLink v-if="error" to="/login" class="login-back">返回登录页</RouterLink>
    </div>
  </div>
</template>

<style scoped>
.oauth-callback {
  max-width: 460px;
  padding-top: 10vh;
}

.login-back {
  display: inline-flex;
  margin-top: 12px;
}
</style>
