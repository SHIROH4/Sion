<template>
  <div>
    <div class="page-header">
      <h2 class="page-title">记忆图谱</h2>
      <n-radio-group v-model:value="tab" size="small">
        <n-radio-button value="memory">记忆</n-radio-button>
        <n-radio-button value="identity">身份</n-radio-button>
      </n-radio-group>
    </div>
    <MemoryPage v-if="tab === 'memory'" />
    <IdentityPage v-if="tab === 'identity'" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { NRadioGroup, NRadioButton } from 'naive-ui'
import MemoryPage from './MemoryPage.vue'
import IdentityPage from './IdentityPage.vue'

const tab = ref<'memory' | 'identity'>('memory')
</script>

<style scoped>
.page-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 20px; }
.page-title { font-size: 22px; font-weight: 700; margin: 0; color: #1a1a2e; }
</style>
