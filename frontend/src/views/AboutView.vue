<script setup lang="ts">
import { useRouter } from 'vue-router'
import { getAccessToken } from '../auth'

const router = useRouter()

function primaryAction() {
  router.push(getAccessToken() ? '/eval' : '/login')
}
</script>

<template>
  <div class="page about-page">
    <section class="card about-hero">
      <p class="eyebrow">ABOUT · 关于本项目</p>
      <h1>看山模型竞技场</h1>
      <p class="about-tagline">有问题，就会有答案；有模型，就会有江湖。</p>
      <p class="about-lede">
        一个面向中文社区的<strong>大模型 1v1 盲评与打榜站点</strong>：在不知道模型身份的情况下，让用户对中文场景下的两段回答做四档评分，
        系统用 Elo 与模型互评矩阵聚合出榜单，让「哪个模型更会回答」变成<strong>可观察、可讨论、可迭代的数据</strong>。
      </p>
      <div class="about-actions">
        <button type="button" @click="primaryAction">{{ getAccessToken() ? '进入 Battle' : '登录开始盲评' }}</button>
        <RouterLink class="link-btn" to="/rankings">查看 Ranking</RouterLink>
      </div>
    </section>

    <section class="card about-section about-narrative">
      <p class="eyebrow">立意</p>
      <h2>知乎在大模型时代的生态位</h2>
      <p>
        国外有 LMSYS Chatbot Arena 这样的盲评榜单，但<strong>中文语境、文化梗、社区表达习惯</strong>是缺的；
        而要让一个评测榜单可信，<strong>评审者本身的质量</strong>比模型本身还关键——它需要既懂内容、又愿意认真看回答的人。
      </p>
      <p>
        知乎刚好满足这一前提：<strong>高质量提问者 + 大量大 V 与从业者</strong>，长期沉淀的「什么是好的回答」的判断力，是其它平台很难复制的。
        所以这个项目的产品立意是：
      </p>
      <blockquote>
        知乎不必去和大厂卷训练，但完全可以做<strong>「中文世界里最有公信力的大模型评估场」</strong>。
        这既是对当下用户的内容服务，也是知乎在 AI 时代一个清晰、可长期经营的生态位。
      </blockquote>
    </section>

    <section class="about-section">
      <p class="eyebrow">PLAYBOOK · 玩法</p>
      <h2>核心玩法</h2>
      <div class="about-feature-grid">
        <article class="card about-feature">
          <span class="about-feature-kicker">玩法 1</span>
          <h3>主题化 1 vs 1 盲评</h3>
          <p>
            选一个主题——<strong>弱智吧Case评估</strong>、<strong>小说创作评估</strong>、<strong>短剧剧本生成</strong>、<strong>高情商回复</strong>——
            从题库随机抽题，左右两侧匿名展示两个模型的回答，在 <strong>A 更好 / B 更好 / 都好 / 都不好</strong> 四档中选一提交。
          </p>
          <p class="muted">
            知乎 IP <strong>「刘看山」</strong> 在流程里充当出题主持人，读题、提示评分逻辑，让评估这件事不再是冷冰冰的打分表。
          </p>
        </article>
        <article class="card about-feature">
          <span class="about-feature-kicker">玩法 2</span>
          <h3>自带 Endpoint 打榜</h3>
          <p>
            提交 <strong>OpenAI Chat Completions 兼容 endpoint</strong>，把自己的模型 / 微调版本送进擂台，与基线模型回答盲评。
          </p>
          <p class="muted">
            用户提供的 <strong>Bearer / API Key 只在本次请求内使用，不落库</strong>——这是产品对用户的明确承诺。
            第三方 / 小团队 / 个人开发者，第一次拥有一个面向中文社区、由真实人类评判的擂台。
          </p>
        </article>
        <article class="card about-feature">
          <span class="about-feature-kicker">Ranking</span>
          <h3>可解释、可迭代的榜单</h3>
          <p>
            <strong>Elo 聚合</strong>：胜负更新 Elo，「都好 / 都不好」走平局 Elo，「都不好」调节更轻，避免噪声。
          </p>
          <p>
            <strong>看山榜 + 公认能力榜映射</strong>：把站内 Elo 与国际 Arena 的综合 / 创作榜放在同一张表，看一眼差异。
          </p>
          <p>
            <strong>价格–能力散点图</strong> 与 <strong>模型互评热力图（Peer Matrix）</strong>：让"哪个模型既便宜又好用"和"谁服气谁"都直接可视化。
          </p>
        </article>
        <article class="card about-feature">
          <span class="about-feature-kicker">Admin</span>
          <h3>可运营的题库后台</h3>
          <p>
            管理员维护分类、题目、模型与回答，黑客松期间可快速补题、调整样本，保持评估池新鲜。
            Dashboard 同步展示用户数 / 投票数 / 题目数 / 模型数的近 14 天趋势。
          </p>
        </article>
      </div>
    </section>

    <section class="card about-section">
      <p class="eyebrow">DATA · 数据基础</p>
      <h2>让评估有底气</h2>
      <p>
        仓库内的 <code>eval-workspace/</code> 是这套榜单的内容护城河，每个领域都包含原始 query、system prompt / user prompt、模型调用脚本与各模型的 <code>responses/*.jsonl</code>，
        <strong>所有题库与模型回答都可复现、可审计、可继续扩充</strong>。
      </p>
      <ul class="about-domain-list">
        <li><strong>弱智吧Case评估</strong>（中文逻辑陷阱、抖机灵题）</li>
        <li><strong>小说创作评估</strong>（中文长文本叙事与文风）</li>
        <li><strong>短剧剧本生成</strong>（场景化、人物对白、节奏）</li>
        <li><strong>高情商回复</strong>（情感场景下的得体表达）</li>
      </ul>
      <p>
        <code>model-peer-evals/</code> 用<strong>模型互评做冷启动</strong>：每个模型同时充当评委对其他模型打分，
        生成的 <code>peer_votes.jsonl</code> 在后端启动时幂等导入，
        让用户<strong>第一天打开 Ranking 就能看到一份合理的榜单</strong>，而不是空表。
      </p>
    </section>

    <section class="card about-section">
      <p class="eyebrow">PRODUCT DECISIONS · 产品决策</p>
      <h2>我们做的几个关键选择</h2>
      <ol class="about-decisions">
        <li>
          <strong>盲评是默认形态，不是隐藏选项。</strong>
          相比打分表，1v1 盲评门槛更低、抗背书污染、收敛更快。
        </li>
        <li>
          <strong>四档评分（A / B / 都好 / 都不好）。</strong>
          比"必须二选一"多保留了「都菜」与「都行」的真实信号，让 Elo 不被强制扭曲。
        </li>
        <li>
          <strong>Elo 之外保留 Rank / 胜率 / 票数 / 互评矩阵。</strong>
          单一指标永远会被刷，多视角才稳。
        </li>
        <li>
          <strong>Endpoint 自带打榜 + 不存密钥。</strong>
          把"评估"开放给任何模型作者，但用产品级承诺保护信任。
        </li>
        <li>
          <strong>知乎 IP 嵌入流程。</strong>
          刘看山做"出题主持人"不是装饰，是把社区氛围直接搬进 AI 产品里——这就是知乎在评模型，而不是又一个第三方榜单。
        </li>
      </ol>
    </section>

    <section class="card about-section">
      <p class="eyebrow">STACK · 技术栈</p>
      <h2>怎么实现的</h2>
      <div class="about-stack-grid">
        <div>
          <h4>前端</h4>
          <p class="muted">
            Vue 3 + Vite + TypeScript；自研轻量 Markdown 渲染、SVG 图表（趋势图 / 价格–能力散点 / 互评热力图）；
            刘看山 IP 贯穿首页与 Battle 流程。
          </p>
        </div>
        <div>
          <h4>后端</h4>
          <p class="muted">
            Go + Gin + GORM + SQLite；JWT 登录；盲评 session、投票、Elo 聚合与互评矩阵 API；
            启动时幂等导入题库 / 模型回答 / 互评结果，<strong>支持「删库 → 启动即冷启动可用」</strong>。
          </p>
        </div>
        <div>
          <h4>登录</h4>
          <p class="muted">
            账号密码登录；可选<strong>知乎官方 OAuth 授权码登录</strong>，未来可基于知乎身份做大 V 加权、垂类身份盲评、社区内容沉淀。
          </p>
        </div>
        <div>
          <h4>部署</h4>
          <p class="muted">
            Docker Compose 一键启动，前端映射到主机 80 端口，云上放行 TCP 80 即可访问。
          </p>
        </div>
      </div>
    </section>

    <section class="card about-section about-summary">
      <p class="eyebrow">TL;DR</p>
      <p>
        看山模型竞技场把<strong>大模型评估</strong>做成一种社区化、可玩、可解释的产品形态：
        中文场景下的盲评 + Elo + 模型互评矩阵 + 价格能力对照——
        让<strong>知乎的高质量用户与大 V</strong>，在大模型时代成为<strong>「中文模型评价」的最高公信力</strong>。
      </p>
    </section>
  </div>
</template>

<style scoped>
.about-page {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.about-hero {
  padding: 28px 26px;
  background:
    radial-gradient(circle at 12% 14%, color-mix(in srgb, var(--brand-2) 22%, transparent), transparent 36%),
    radial-gradient(circle at 88% 8%, color-mix(in srgb, var(--brand-3) 18%, transparent), transparent 38%),
    color-mix(in srgb, var(--surface) 92%, transparent);
}

.about-hero h1 {
  margin: 6px 0 8px;
  font-size: clamp(1.8rem, 3.4vw, 2.6rem);
}

.about-tagline {
  margin: 0 0 12px;
  font-size: 1.05rem;
  color: var(--brand-2);
  font-weight: 700;
}

.about-lede {
  margin: 0 0 16px;
  line-height: 1.7;
  font-size: 15px;
  max-width: 760px;
}

.about-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}

.about-section {
  padding: 22px 22px 20px;
}

.about-section h2 {
  margin: 4px 0 12px;
  font-size: 1.4rem;
}

.about-section h3 {
  margin: 0 0 8px;
}

.about-section h4 {
  margin: 0 0 6px;
  color: var(--brand-2);
}

.about-section p {
  margin: 0 0 10px;
  line-height: 1.7;
}

.about-section code {
  padding: 1px 6px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--surface-2) 86%, transparent);
  font-size: 0.9em;
}

.about-narrative blockquote {
  margin: 12px 0 0;
  padding: 12px 16px;
  border-left: 3px solid color-mix(in srgb, var(--brand-2) 65%, var(--border));
  border-radius: 8px;
  background: color-mix(in srgb, var(--brand-2) 8%, transparent);
  line-height: 1.7;
}

.about-feature-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.about-feature {
  padding: 18px 18px 16px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.about-feature p {
  margin: 0 0 6px;
  line-height: 1.65;
  font-size: 14px;
}

.about-feature-kicker {
  display: inline-flex;
  align-self: flex-start;
  padding: 3px 10px;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--brand-2) 38%, var(--border));
  background: color-mix(in srgb, var(--brand-2) 12%, transparent);
  color: var(--brand-2);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.about-domain-list {
  margin: 4px 0 12px;
  padding-left: 1.2em;
  line-height: 1.85;
}

.about-decisions {
  margin: 4px 0 0;
  padding-left: 1.3em;
  line-height: 1.75;
}

.about-decisions li {
  margin-bottom: 8px;
}

.about-stack-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.about-stack-grid p {
  margin: 0;
  font-size: 14px;
}

.about-summary {
  background:
    radial-gradient(circle at 84% 18%, color-mix(in srgb, var(--brand-2) 14%, transparent), transparent 38%),
    color-mix(in srgb, var(--surface-2) 80%, transparent);
}

.about-summary p {
  margin: 0;
  font-size: 15px;
  line-height: 1.75;
}

@media (max-width: 760px) {
  .about-feature-grid,
  .about-stack-grid {
    grid-template-columns: 1fr;
  }
}
</style>
