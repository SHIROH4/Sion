<template>
  <n-dynamic-tags v-model:value="tags" :input-props="{ placeholder: modelValue.length === 0 ? '输入后回车添加' : '继续添加' }" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { NDynamicTags } from 'naive-ui'

const props = defineProps<{ modelValue: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [tags: string[]] }>()

const tags = computed({
  get: () => props.modelValue,
  set: (v: string[]) => emit('update:modelValue', v),
})
</script>
