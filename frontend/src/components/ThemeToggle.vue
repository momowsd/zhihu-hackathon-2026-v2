<script setup lang="ts">
import { computed } from 'vue'
import { useTheme } from '../theme'

const { themeMode, toggleTheme } = useTheme()
const isLight = computed(() => themeMode.value === 'light')
</script>

<template>
  <button
    class="theme-btn"
    type="button"
    role="switch"
    :aria-checked="isLight"
    :aria-label="isLight ? '切换到暗色模式' : '切换到亮色模式'"
    :class="{ 'is-dark': !isLight }"
    @click="toggleTheme"
  >
    <span class="theme-track" aria-hidden="true"></span>
    <span class="theme-thumb" aria-hidden="true"></span>
    <span class="theme-icon sun" aria-hidden="true">☀</span>
    <span class="theme-icon moon" aria-hidden="true">☾</span>
  </button>
</template>

<style scoped>
.theme-btn {
  width: 40px;
  height: 22px;
  border: 0;
  border-radius: 999px;
  padding: 0;
  color: var(--text-secondary);
  background: transparent;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  position: relative;
}

.theme-track {
  position: absolute;
  inset: 0;
  border-radius: 999px;
  background: color-mix(in srgb, var(--surface-2) 78%, transparent);
  border: 1px solid var(--border);
}

.theme-thumb {
  position: absolute;
  left: 2px;
  top: 2px;
  width: 16px;
  height: 16px;
  border-radius: 999px;
  background: #ffffff;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
  transition: transform 0.22s ease, background 0.22s ease;
}

.theme-btn.is-dark .theme-thumb {
  transform: translateX(20px);
  background: #cbd5e1;
}

.theme-icon {
  position: absolute;
  z-index: 2;
  font-size: 9px;
  line-height: 1;
  opacity: 0.7;
}

.sun {
  left: 6px;
}

.moon {
  right: 6px;
}
</style>
