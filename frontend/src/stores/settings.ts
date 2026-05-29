import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useSettingsStore = defineStore('settings', () => {
  const open = ref(false)

  function toggle() {
    open.value = !open.value
  }

  function setOpen(v: boolean) {
    open.value = v
  }

  return { open, toggle, setOpen }
})
