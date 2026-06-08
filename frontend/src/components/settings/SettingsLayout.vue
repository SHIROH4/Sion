<template>
  <n-layout has-sider class="app-layout">
    <Sidebar />
    <n-layout-content class="main-content">
      <slot />
    </n-layout-content>
  </n-layout>
</template>

<script setup lang="ts">
import { NLayout, NLayoutContent } from 'naive-ui'
import Sidebar from './Sidebar.vue'
</script>

<style scoped>
.app-layout { height: 100vh; }
.main-content {
  background: #f5f7fa;
  padding: 32px;
  overflow: hidden;
  height: 100vh;
}
</style>
