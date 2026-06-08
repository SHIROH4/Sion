<template>
  <div v-if="visible" class="thinking-dots">
    <span v-for="i in 3" :key="i" class="dot" :style="{ animationDelay: `${(i - 1) * 0.15}s` }" />
  </div>
</template>

<script setup lang="ts">
defineProps<{ visible: boolean }>()
</script>

<style scoped>
.thinking-dots {
  position: absolute; top: 18%; left: 50%;
  transform: translateX(-50%); z-index: 12;
  display: flex; gap: 6px; pointer-events: none;
}
.dot {
  display: inline-block; width: 8px; height: 8px;
  border-radius: 50%; background: #999;
  animation: thinking-bounce 1.4s infinite ease-in-out both;
}
</style>
