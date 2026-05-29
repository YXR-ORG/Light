<template>
  <div class="chat-view">
    <div class="empty-state">
      <h2>Wails AI Chat</h2>
      <p>Select a conversation or create a new one to start chatting</p>
    </div>
  </div>
</template>

<style scoped>
.chat-view { flex: 1; display: flex; align-items: center; justify-content: center; }
.empty-state { text-align: center; }
.empty-state h2 { font-size: 24px; margin-bottom: 8px; }
.empty-state p { color: var(--text-secondary); font-size: 14px; }
</style>
