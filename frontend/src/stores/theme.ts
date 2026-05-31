import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

export type ThemeMode = 'light' | 'dark' | 'system'

function applyTheme(mode: ThemeMode) {
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  const isDark = mode === 'dark' || (mode === 'system' && prefersDark)
  document.documentElement.setAttribute('data-theme', isDark ? 'dark' : 'light')
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>((localStorage.getItem('theme') as ThemeMode) || 'system')

  // Apply on init
  applyTheme(mode.value)

  // Listen for system preference changes
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  mq.addEventListener('change', () => {
    if (mode.value === 'system') applyTheme('system')
  })

  function setMode(m: ThemeMode) {
    mode.value = m
    localStorage.setItem('theme', m)
    applyTheme(m)
  }

  function toggle() {
    // light → dark → system → light ...
    const next: Record<ThemeMode, ThemeMode> = { light: 'dark', dark: 'system', system: 'light' }
    setMode(next[mode.value])
  }

  return { mode, setMode, toggle }
})
