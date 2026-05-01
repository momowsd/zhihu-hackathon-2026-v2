<script setup lang="ts">
import { useRouter } from 'vue-router'
import { getAccessToken } from '../auth'

const router = useRouter()

function primaryAction() {
  router.push(getAccessToken() ? '/dashboard' : '/login')
}
</script>

<template>
  <section class="home page">
    <div class="hero-card">
      <div class="eyebrow">LLM Blind Evaluation Arena</div>
      <h1>用真实用户偏好，比较大模型回答效果。</h1>
      <p>
        这是一个面向 hackathon 的大模型盲评与打榜站点：用户在不知道模型身份的情况下比较回答，
        系统用 Elo 聚合胜负结果，让“哪个模型更会回答”变成可观察、可讨论、可迭代的数据。
      </p>
      <div class="hero-actions">
        <button @click="primaryAction">{{ getAccessToken() ? '进入 Dashboard' : '登录开始盲评' }}</button>
        <RouterLink class="link-btn" to="/rankings">查看 Ranking</RouterLink>
      </div>
    </div>

    <div class="feature-grid">
      <div class="feature-card">
        <span class="feature-kicker">玩法 1</span>
        <h3>主题化 1 vs 1 盲评</h3>
        <p>按“弱智评估”“八卦评估”“角色扮演”“搞笑程度”等主题随机抽题，匿名比较主流模型回答。</p>
      </div>
      <div class="feature-card">
        <span class="feature-kicker">玩法 2</span>
        <h3>自带 Endpoint 打榜</h3>
        <p>提交 OpenAI 兼容 endpoint，与已有模型回答盲评。密钥只在单次请求内使用，不落库。</p>
      </div>
      <div class="feature-card">
        <span class="feature-kicker">Ranking</span>
        <h3>Elo 排名与榜单</h3>
        <p>用 1v1 胜负更新 Elo，同时保留胜率、票数和分类维度，降低单次评分噪声。</p>
      </div>
      <div class="feature-card">
        <span class="feature-kicker">Admin</span>
        <h3>可运营题库后台</h3>
        <p>管理员维护分类、题目、模型和回答，方便 hackathon 期间快速补题和调整样本。</p>
      </div>
    </div>
  </section>
</template>
