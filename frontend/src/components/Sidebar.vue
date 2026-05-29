<script setup lang="ts">
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-header">
      <h2>Chats</h2>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  width: 260px; height: 100vh; display: flex; flex-direction: column;
  border-right: 1px solid var(--border-color); background: var(--sidebar-bg);
}
.sidebar-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 12px 16px; border-bottom: 1px solid var(--border-color);
}
.sidebar-header h2 { margin: 0; font-size: 16px; }
</style>
