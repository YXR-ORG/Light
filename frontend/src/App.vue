<script setup lang="ts">
import { useSettingsStore } from './stores/settings'
import SettingsDialog from './components/SettingsDialog.vue'
import Sidebar from './components/Sidebar.vue'
import ChatView from './components/ChatView.vue'
</script>

<template>
  <div class="app-layout">
    <Sidebar />
    <main class="main-area">
      <ChatView />
    </main>
  </div>
  <SettingsDialog />
</template>

<style>
:root {
  --sidebar-bg: #f8f9fa;
  --border-color: #e5e7eb;
  --hover-bg: #f3f4f6;
  --active-bg: #e5e7eb;
  --text-secondary: #6b7280;
  --accent: #3b82f6;
  --bg: #ffffff;
  --text: #111827;
}

* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; color: var(--text); background: var(--bg); }
.app-layout { display: flex; height: 100vh; }
.main-area { flex: 1; display: flex; flex-direction: column; }
</style>
