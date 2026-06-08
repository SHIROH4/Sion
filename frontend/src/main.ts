import { createApp } from 'vue'
import { createPinia } from 'pinia'
import './style.css'
import App from './App.vue'

const app = createApp(App)
app.config.errorHandler = (err) => {
  if (String(err).includes('ResizeObserver')) return
  console.error('Pet Error:', err)
}
app.use(createPinia())
app.mount('#root')
