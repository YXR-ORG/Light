import { createApp } from 'vue'
import App from './App.vue'
import { createPinia } from 'pinia'
import './style.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)

// Init theme before mount to avoid flash
import { useThemeStore } from './stores/theme'
useThemeStore(pinia)

app.mount('#app')
