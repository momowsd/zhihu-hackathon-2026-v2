<script setup lang="ts">
import { ref, useTemplateRef } from 'vue'
import { useRouter } from 'vue-router'
import { getAccessToken } from '../auth'

const router = useRouter()

const HERO_KANSHAN_MP4 = '/assets/kanshan_imgs/liukanshan_1_1.mp4'

/** 自动播放必须 muted；开声需用户点击（浏览器策略） */
const heroVideoRef = useTemplateRef<HTMLVideoElement>('heroVideo')
const heroSoundOn = ref(false)

function toggleHeroSound() {
  const v = heroVideoRef.value
  if (!v) return
  v.muted = !v.muted
  heroSoundOn.value = !v.muted
  void v.play().catch(() => {})
}

function primaryAction() {
  router.push(getAccessToken() ? '/dashboard' : '/login')
}
</script>

<template>
  <section class="home page">
    <div class="hero-card">
      <div class="hero-layout">
        <div class="hero-copy">
          <div class="eyebrow">LLM Blind Evaluation Arena · 刘看山在场</div>
          <h1>用真实用户偏好，比较大模型回答效果。</h1>
          <p>
            这是一个面向 hackathon 的大模型盲评与打榜站点：用户在不知道模型身份的情况下比较回答，
            系统用 Elo 聚合胜负结果，让“哪个模型更会回答”变成可观察、可讨论、可迭代的数据。
            知乎 IP「刘看山」会在首页与 Battle 里陪你读题、选档；首屏用循环短片呈现形象，Battle 里仍可点点他听一句主持词。
          </p>
          <div class="hero-actions">
            <button @click="primaryAction">{{ getAccessToken() ? '进入 Dashboard' : '登录开始盲评' }}</button>
            <RouterLink class="link-btn" to="/rankings">查看 Ranking</RouterLink>
          </div>
        </div>

        <div class="hero-visual">
          <div class="hero-visual-stack">
            <div class="hero-kanshan-video-wrap">
              <video
                ref="heroVideo"
                class="hero-kanshan-video"
                :src="HERO_KANSHAN_MP4"
                autoplay
                loop
                muted
                playsinline
                disablepictureinpicture
                aria-label="刘看山形象短片（循环播放，默认静音；可开启声音）"
              />
              <button
                type="button"
                class="hero-kanshan-sound-btn"
                :aria-pressed="heroSoundOn"
                :aria-label="heroSoundOn ? '关闭短片声音' : '开启短片声音'"
                @click="toggleHeroSound"
              >
                {{ heroSoundOn ? '静音' : '开启声音' }}
              </button>
            </div>
          </div>
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
