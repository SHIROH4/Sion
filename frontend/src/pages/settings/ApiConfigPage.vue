<template>
  <div class="api-page">
    <h2 class="page-title">API 配置</h2>

    <n-spin v-if="loading" style="display:flex;justify-content:center;padding:60px;" />
    <n-text v-else-if="!cfg" type="error">无法加载配置，请检查主进程是否运行</n-text>

    <div v-else class="form">
      <!-- LLM -->
      <n-card title="LLM 对话" :bordered="false" size="small" style="margin-bottom:20px;">
        <n-form label-placement="left" label-width="130" :show-feedback="false">
          <n-form-item label="Provider"><n-input v-model:value="cfg.llm_provider" placeholder="deepseek" /></n-form-item>
          <n-form-item label="Base URL"><n-input v-model:value="cfg.llm_base_url" placeholder="https://api.deepseek.com" /></n-form-item>
          <n-form-item label="Model"><n-input v-model:value="cfg.llm_model" placeholder="deepseek-chat" /></n-form-item>
          <n-form-item label="API Key"><n-input v-model:value="cfg.llm_api_key" type="password" placeholder="sk-..." show-password-on="click" /></n-form-item>
        </n-form>
      </n-card>

      <!-- Vision -->
      <n-card title="Vision 截图分析" :bordered="false" size="small" style="margin-bottom:20px;">
        <n-form label-placement="left" label-width="130" :show-feedback="false">
          <n-form-item label="Vision Model"><n-input v-model:value="cfg.vision_model" placeholder="qwen-vl-max" /></n-form-item>
          <n-form-item label="Base URL"><n-input v-model:value="cfg.vision_base_url" placeholder="默认同 LLM Base URL" /></n-form-item>
          <n-form-item label="API Key"><n-input v-model:value="cfg.vision_api_key" type="password" placeholder="默认同 LLM API Key" show-password-on="click" /></n-form-item>
        </n-form>
      </n-card>

      <!-- Emotion -->
      <n-card title="Emotion 情绪评估" :bordered="false" size="small" style="margin-bottom:20px;">
        <n-form label-placement="left" label-width="130" :show-feedback="false">
          <n-form-item label="Emotion Model"><n-input v-model:value="cfg.emotion_model" placeholder="DeepSeek-V4-Flash" /></n-form-item>
          <n-form-item label="Base URL"><n-input v-model:value="cfg.emotion_base_url" placeholder="https://api.siliconflow.cn/v1" /></n-form-item>
          <n-form-item label="API Key"><n-input v-model:value="cfg.emotion_api_key" type="password" placeholder="sk-..." show-password-on="click" /></n-form-item>
        </n-form>
      </n-card>

      <!-- User Info -->
      <n-card title="用户信息" :bordered="false" size="small" style="margin-bottom:20px;">
        <n-form label-placement="left" label-width="100" :show-feedback="false">
          <n-form-item label="称呼"><n-input v-model:value="cfg.user_name" placeholder="主人" /></n-form-item>
          <n-form-item label="技术栈"><TagInput v-model="cfg.user_tech_stack" /></n-form-item>
        </n-form>
      </n-card>

      <!-- Actions -->
      <n-space justify="end" :size="10">
        <n-button @click="testConnection('Chat', 'chat')" :loading="testing === 'Chat'" :disabled="!cfg.llm_model">测试 Chat</n-button>
        <n-button v-if="cfg.vision_model" @click="testConnection('Vision', 'vision')" :loading="testing === 'Vision'">测试 Vision</n-button>
        <n-button v-if="cfg.emotion_model" @click="testConnection('Emotion', 'emotion')" :loading="testing === 'Emotion'">测试 Emotion</n-button>
        <n-button type="primary" @click="handleSave" :loading="saving">保存配置</n-button>
      </n-space>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NForm, NFormItem, NInput, NButton, NSpace, NSpin, NText, useMessage } from 'naive-ui'
import { getConfig, saveConfig, GlobalConfig } from '../../stores/api'
import TagInput from '../../components/settings/TagInput.vue'

const message = useMessage()
const cfg = ref<GlobalConfig | null>(null)
const loading = ref(true)
const saving = ref(false)
const testing = ref<string | null>(null)

onMounted(async () => {
  try { cfg.value = await getConfig() }
  catch (e: any) { message.error('加载配置失败: ' + (e?.message || e)) }
  finally { loading.value = false }
})

async function handleSave() {
  if (!cfg.value || !cfg.value.llm_provider || !cfg.value.llm_model || !cfg.value.llm_base_url) {
    message.warning('请填写 LLM Provider、Model 和 Base URL'); return
  }
  saving.value = true
  try { await saveConfig(cfg.value); message.success('配置已保存') }
  catch (e: any) { message.error('保存失败: ' + (e?.message || e)) }
  finally { saving.value = false }
}

async function testConnection(label: string, target: string) {
  testing.value = label
  try {
    const res = await fetch('http://127.0.0.1:19840/api/test-connection', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ target }),
    })
    const data = await res.json()
    if (data.ok) message.success(`${label} 连接成功`)
    else message.error(`${label} 失败: ${data.error}`)
  } catch (e: any) { message.error(`${label} 失败: ${e?.message || e}`) }
  finally { testing.value = null }
}
</script>

<style scoped>
.api-page { max-width: 680px; height: 100%; overflow-y: auto; }
.page-title { font-size: 22px; font-weight: 700; margin: 0 0 24px; color: #1a1a2e; }
.form :deep(.n-card__content) { padding-top: 4px; }
</style>
