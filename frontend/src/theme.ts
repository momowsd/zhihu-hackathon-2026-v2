import { ref } from 'vue'

export type ThemeMode = 'dark' | 'light'

const THEME_KEY = 'llm_arena_theme_mode'
const themeMode = ref<ThemeMode>('dark')

function applyTheme(mode: ThemeMode): void {
  themeMode.value = mode
  document.documentElement.setAttribute('data-theme', mode)
}

export function initTheme(): void {
  const saved = localStorage.getItem(THEME_KEY)
  if (saved === 'light' || saved === 'dark') {
    applyTheme(saved)
    return
  }
  // 未在本地选择过主题时固定默认暗色（不跟随系统）
  applyTheme('dark')
}

export function toggleTheme(): void {
  const next = themeMode.value === 'dark' ? 'light' : 'dark'
  applyTheme(next)
  localStorage.setItem(THEME_KEY, next)
}

export function useTheme() {
  return {
    themeMode,
    toggleTheme,
  }
}
