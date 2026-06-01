<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

const version = ref('...')
onMounted(async () => {
  try {
    version.value = await (window as any).go.main.App.GetVersion()
  } catch {
    version.value = '1.0.1'
  }
})
</script>

<template>
  <div class="about-panel">
    <div class="about-logo">
      <svg width="56" height="56" viewBox="0 0 64 64" fill="none">
        <circle cx="32" cy="32" r="10" fill="#ffe066"/>
        <circle cx="32" cy="32" r="6" fill="#fff"/>
        <g stroke="#ffe066" stroke-linecap="round">
          <line x1="32" y1="4"  x2="32" y2="14" stroke-width="3.5" opacity="0.9"/>
          <line x1="32" y1="50" x2="32" y2="60" stroke-width="3.5" opacity="0.9"/>
          <line x1="4"  y1="32" x2="14" y2="32" stroke-width="3.5" opacity="0.9"/>
          <line x1="50" y1="32" x2="60" y2="32" stroke-width="3.5" opacity="0.9"/>
          <line x1="11.5" y1="11.5" x2="18.5" y2="18.5" stroke-width="2.5" opacity="0.6"/>
          <line x1="45.5" y1="45.5" x2="52.5" y2="52.5" stroke-width="2.5" opacity="0.6"/>
          <line x1="52.5" y1="11.5" x2="45.5" y2="18.5" stroke-width="2.5" opacity="0.6"/>
          <line x1="18.5" y1="45.5" x2="11.5" y2="52.5" stroke-width="2.5" opacity="0.6"/>
        </g>
      </svg>
    </div>
    <div class="about-name">Light</div>
    <div class="about-version">v{{ version }}</div>
    <div class="about-desc">
      轻量、优雅的本地 AI 对话客户端<br/>
      支持多模型、联网搜索、Skills、MCP
    </div>
    <a class="about-link" href="#" @click.prevent="BrowserOpenURL('https://github.com/YXR-ORG/Light')">
      <svg width="15" height="15" viewBox="0 0 24 24" fill="currentColor">
        <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0 1 12 6.844a9.59 9.59 0 0 1 2.504.337c1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.02 10.02 0 0 0 22 12.017C22 6.484 17.522 2 12 2z"/>
      </svg>
      github.com/YXR-ORG/Light
    </a>
    <div class="about-footer">数据存储于本地，API Key 不上传任何服务器</div>
  </div>
</template>

<style scoped>
.about-panel {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-8) var(--space-6);
  height: 100%;
  justify-content: center;
}

.about-logo {
  filter: drop-shadow(0 0 12px oklch(0.85 0.18 85 / 0.5));
  margin-bottom: var(--space-1);
}

.about-name {
  font-size: var(--text-2xl);
  font-weight: 700;
  letter-spacing: -0.02em;
  background: linear-gradient(135deg, #ffe066 0%, #ff9500 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.about-version {
  font-size: var(--text-sm);
  color: var(--color-text-3);
  background: var(--color-paper-3);
  padding: 2px var(--space-3);
  border-radius: var(--radius-full);
}

.about-desc {
  font-size: var(--text-sm);
  color: var(--color-text-2);
  text-align: center;
  line-height: 1.8;
  margin-top: var(--space-1);
}

.about-link {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  color: var(--color-accent);
  text-decoration: none;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  transition: background var(--duration-fast) var(--ease-out);
  margin-top: var(--space-2);
}

.about-link:hover {
  background: var(--color-accent-soft);
  text-decoration: none;
}

.about-footer {
  font-size: var(--text-xs);
  color: var(--color-text-3);
  margin-top: var(--space-4);
}
</style>
