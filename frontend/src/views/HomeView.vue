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
      <div class="hero-layout">
        <div class="hero-copy">
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

        <div class="hero-visual" aria-hidden="true">
          <svg class="hero-arena-svg" viewBox="0 0 440 300" fill="none" xmlns="http://www.w3.org/2000/svg">
            <defs>
              <linearGradient id="heroArenaStroke" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stop-color="var(--brand)" />
                <stop offset="100%" stop-color="var(--brand-2)" />
              </linearGradient>
            </defs>

            <ellipse cx="220" cy="168" rx="190" ry="118" class="hero-arena-ellipse" />

            <path
              class="hero-arena-blind"
              d="M108 138 H332"
              stroke-dasharray="6 10"
              stroke-width="1.5"
              stroke-linecap="round"
            />

            <rect x="36" y="78" width="148" height="172" rx="22" class="hero-arena-card hero-arena-card--l" />
            <rect x="256" y="78" width="148" height="172" rx="22" class="hero-arena-card hero-arena-card--r" />

            <text x="110" y="182" text-anchor="middle" class="hero-arena-label">A</text>
            <text x="330" y="182" text-anchor="middle" class="hero-arena-label">B</text>

            <circle cx="220" cy="156" r="36" class="hero-arena-vs-ring" />
            <text x="220" y="166" text-anchor="middle" class="hero-arena-vs">VS</text>

            <path
              d="M220 52 v28 M208 68 l12 12 12-12"
              class="hero-arena-arrow"
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
            />

            <text x="220" y="268" text-anchor="middle" class="hero-arena-caption">Blind · Elo · Ranking</text>
          </svg>
        </div>
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
