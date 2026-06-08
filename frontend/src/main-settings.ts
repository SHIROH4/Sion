import { createApp } from 'vue'
import { createPinia } from 'pinia'
import './style.css'
import SettingsApp from './SettingsApp.vue'

const app = createApp(SettingsApp)
app.config.errorHandler = (err, _instance, info) => {
  const msg = String(err)
  // ResizeObserver loop errors are benign — browsers throw them when resize
  // callbacks overlap frames, which Naive UI's layout components trigger.
  if (msg.includes('ResizeObserver')) return
  console.error('Vue Error:', err, info)
  const el = document.getElementById('fatal-error')
  if (el) {
    el.style.display = 'block'
    el.textContent = 'Fatal: ' + msg + '\n' + (err instanceof Error ? err.stack ?? '' : '')
  }
}
app.use(createPinia())

const container = document.getElementById('root')
if (!container) {
  document.body.innerHTML = '<div style="padding:40px;font-family:monospace;color:red;"><h3>Fatal: #root not found</h3></div>'
} else {
  app.mount('#root')
}
