import { createApp } from 'vue'
import App from './App.vue'
import './style.css'

try {
  document.documentElement.dataset.theme = localStorage.getItem('moye.theme') === 'dark' ? 'dark' : 'light'
} catch {
  document.documentElement.dataset.theme = 'light'
}

createApp(App).mount('#app')
