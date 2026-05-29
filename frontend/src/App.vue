<script setup lang="ts">
import { useSettingsStore } from './stores/settings'
import SettingsDialog from './components/SettingsDialog.vue'
import Sidebar from './components/Sidebar.vue'
import ChatView from './components/ChatView.vue'
</script>

<template>
  <div class="app-shell">
    <Sidebar />
    <main class="main-area">
      <ChatView />
    </main>
  </div>
  <SettingsDialog />
</template>

<style>
@import './assets/tokens.css';

.app-shell {
  display: flex;
  height: 100vh;
  background: var(--color-paper);
}

.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
</style>
