<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    /** home：首页陪伴；battle：由 spriteFrame 驱动表情 */
    scene: 'home' | 'battle'
    /** 四视图横条当前帧 0–3（battle 下由父组件传入） */
    spriteFrame?: number
    /** 仅 home：是否自动轮播切帧 */
    autoCycle?: boolean
    /** 整体缩放档位；xs 用于 Battle 题干旁小伴读 */
    size?: 'lg' | 'md' | 'sm' | 'xs'
    /** 是否展示底部「刘看山 · …」说明（嵌入大卡片时可关） */
    showCaption?: boolean
    /**
     * 为 true 时不做首页左侧裁切（与 battle 一致整格显示），
     * 用于 Battle 开局卡等窄容器，避免左耳/身体被裁掉。
     */
    noSpriteTrim?: boolean
  }>(),
  { autoCycle: true, size: 'md', showCaption: true, noSpriteTrim: false },
)

const BASE = '/assets/kanshan_imgs/'
const FILE_BASE = '刘看山四视图.png'
const FILE_SCARF = '刘看山围脖四视图.png'

const W0 = 786
const H0 = 207
const CELL0 = W0 / 4

const W1 = 787
const H1 = 208
const CELL1 = W1 / 4

/** 首页：略裁左侧留白；Battle：不裁，避免鼻尖/左耳被切（整格显示） */
const SLICE_TRIM_HOME_BASE = 36
const SLICE_TRIM_HOME_SCARF = 36
const SLICE_TRIM_BATTLE = 0

const isHover = ref(false)
const bubbleText = ref('')
const autoFrame = ref(0)
const prefersReduce = ref(false)

let bubbleTimer: ReturnType<typeof setTimeout> | null = null
let cycleTimer: ReturnType<typeof setInterval> | null = null

const useScarf = computed(() => props.scene === 'home' && isHover.value)

const trimPx = computed(() => {
  if (props.noSpriteTrim || props.scene === 'battle') {
    return SLICE_TRIM_BATTLE
  }
  return useScarf.value ? SLICE_TRIM_HOME_SCARF : SLICE_TRIM_HOME_BASE
})

const spriteMeta = computed(() => {
  if (useScarf.value) {
    return { w: W1, h: H1, cell: CELL1, file: FILE_SCARF }
  }
  return { w: W0, h: H0, cell: CELL0, file: FILE_BASE }
})

const spriteUrl = computed(() => BASE + encodeURIComponent(spriteMeta.value.file))

const effectiveFrame = computed(() => {
  if (props.scene === 'battle' && props.spriteFrame != null) {
    const f = Math.floor(Number(props.spriteFrame))
    if (f >= 0 && f <= 3) return f
    return 0
  }
  if (props.scene === 'home') return autoFrame.value % 4
  return 0
})

const imgStyle = computed(() => {
  const { w, h, cell } = spriteMeta.value
  const t = trimPx.value
  const x = -(effectiveFrame.value * cell + t)
  return {
    width: `${w}px`,
    height: `${h}px`,
    transform: `translate3d(${x}px, 0, 0)`,
    transition: 'transform 0.38s cubic-bezier(0.33, 1, 0.68, 1)',
  }
})

const viewportStyle = computed(() => {
  const { cell, h } = spriteMeta.value
  const t = trimPx.value
  const vw = Math.max(64, cell - t)
  return {
    width: `${vw}px`,
    height: `${h}px`,
  }
})

const rootClass = computed(() => [`kanshan-root`, `kanshan-root--size-${props.size}`, `kanshan-root--${props.scene}`])

function clearBubbleTimer() {
  if (bubbleTimer) {
    clearTimeout(bubbleTimer)
    bubbleTimer = null
  }
}

const homeTips = [
  '去 Battle 里帮我给模型打分吧，我不偷看名字～',
  '鼠标放我身上，会换上围巾款造型哦。',
  '左右两边的回答，你说了算。',
  '这是知乎刘看山的四视图素材，我在帮你「跳出静态」一点点～',
]

const battleTips = [
  '可以先用箭头在题目间来回看，再提交。',
  '按第一印象选也很合理，不用纠结太久。',
  '四档里选最贴近你感受的那一个就好。',
  '选好就点，我会陪你记到服务器里。',
]

function onTap() {
  if (props.scene === 'home') {
    const t = homeTips[Math.floor(Math.random() * homeTips.length)]
    bubbleText.value = t
    autoFrame.value = (autoFrame.value + 1) % 4
  } else {
    bubbleText.value = battleTips[Math.floor(Math.random() * battleTips.length)]
  }
  clearBubbleTimer()
  bubbleTimer = setTimeout(() => {
    bubbleText.value = ''
    bubbleTimer = null
  }, 2800)
}

function startCycle() {
  if (cycleTimer) clearInterval(cycleTimer)
  cycleTimer = null
  if (props.scene !== 'home' || props.autoCycle === false || prefersReduce.value) return
  cycleTimer = setInterval(() => {
    if (!isHover.value) autoFrame.value = (autoFrame.value + 1) % 4
  }, 2600)
}

onMounted(() => {
  prefersReduce.value =
    typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  startCycle()
})

watch(
  () => [props.scene, props.autoCycle],
  () => {
    startCycle()
  },
)

onBeforeUnmount(() => {
  if (cycleTimer) clearInterval(cycleTimer)
  clearBubbleTimer()
})
</script>

<template>
  <div
    :class="rootClass"
    @mouseenter="scene === 'home' ? (isHover = true) : null"
    @mouseleave="scene === 'home' ? (isHover = false) : null"
  >
    <button
      type="button"
      class="kanshan-hit"
      :aria-label="scene === 'home' ? '与刘看山互动' : '刘看山给你一句提示'"
      @click="onTap"
    >
      <div class="kanshan-viewport" :style="viewportStyle">
        <img class="kanshan-img" :src="spriteUrl" alt="刘看山" draggable="false" :style="imgStyle" />
      </div>
    </button>
    <p v-if="bubbleText" class="kanshan-bubble" role="status">{{ bubbleText }}</p>
    <span v-if="showCaption" class="kanshan-caption muted">{{ scene === 'home' ? '刘看山 · 点我聊聊' : '刘看山 · 主持中' }}</span>
  </div>
</template>

<style scoped>
.kanshan-root {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  user-select: none;
}

.kanshan-root--size-xs {
  transform: scale(0.52);
  transform-origin: 50% 100%;
}

/*
 * Battle + xs：缩放放在 .kanshan-hit 上，根节点固定为「视觉尺寸」，
 * 避免 flex 行高被 ~207px 占位撑大、底部大块留白。
 */
.kanshan-root--battle.kanshan-root--size-xs {
  transform: none !important;
  width: calc((786px / 4) * 0.4);
  height: calc(207px * 0.4);
  overflow: hidden;
  flex-shrink: 0;
}

.kanshan-root--battle.kanshan-root--size-xs .kanshan-hit {
  transform: scale(0.4);
  transform-origin: top left;
  display: block;
}

.kanshan-root--battle.kanshan-root--size-xs .kanshan-hit:hover {
  transform: scale(0.4) translateY(-2px);
  filter: drop-shadow(0 10px 22px color-mix(in srgb, var(--brand-2) 20%, transparent));
}

/* Battle + sm（完成页等）：仍整根缩放，顶对齐 */
.kanshan-root--battle.kanshan-root--size-sm {
  transform-origin: 50% 0;
}

.kanshan-root--size-sm {
  transform: scale(0.72);
  transform-origin: 50% 100%;
}

.kanshan-root--size-md {
  transform: scale(0.9);
  transform-origin: 50% 100%;
}

.kanshan-root--size-lg {
  transform: scale(1);
  transform-origin: 50% 100%;
}

.kanshan-hit {
  border: 0;
  padding: 0;
  margin: 0;
  background: transparent;
  cursor: pointer;
  border-radius: 18px;
  transition:
    transform 0.22s ease,
    filter 0.22s ease;
}

.kanshan-hit:hover {
  transform: translateY(-3px);
  filter: drop-shadow(0 14px 28px color-mix(in srgb, var(--brand-2) 22%, transparent));
}

.kanshan-hit:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--brand-2) 65%, transparent);
  outline-offset: 4px;
}

.kanshan-root--battle {
  align-items: flex-start;
}

.kanshan-root--battle .kanshan-hit:hover {
  transform: translateY(-2px);
}

.kanshan-viewport {
  overflow: hidden;
  border-radius: 16px;
  border: 1px solid color-mix(in srgb, var(--border) 88%, transparent);
  background: color-mix(in srgb, var(--surface-solid) 55%, transparent);
  box-shadow: 0 10px 28px color-mix(in srgb, #000 12%, transparent);
}

.kanshan-img {
  display: block;
  max-width: none;
  height: auto;
  pointer-events: none;
}

.kanshan-bubble {
  position: absolute;
  left: 50%;
  bottom: calc(100% + 8px);
  transform: translateX(-50%);
  margin: 0;
  min-width: 180px;
  max-width: min(280px, 86vw);
  padding: 10px 12px;
  font-size: 12px;
  line-height: 1.45;
  text-align: left;
  color: var(--text-primary);
  background: color-mix(in srgb, var(--surface) 94%, transparent);
  border: 1px solid var(--border);
  border-radius: 14px;
  box-shadow: var(--shadow-xl);
  z-index: 2;
  animation: kanshan-pop 0.28s ease;
}

@keyframes kanshan-pop {
  from {
    opacity: 0;
    transform: translateX(-50%) translateY(6px);
  }
  to {
    opacity: 1;
    transform: translateX(-50%) translateY(0);
  }
}

.kanshan-caption {
  font-size: 11px;
  letter-spacing: 0.04em;
}

.kanshan-root--battle .kanshan-caption {
  font-size: 10px;
}
</style>
