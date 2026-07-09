import { createApp } from 'vue'
import { createPinia } from 'pinia'
import './style.css'
import App from './App.vue'
import router from './router'
import { useThemeStore } from './stores/theme'

const pinia = createPinia()
const app = createApp(App).use(pinia).use(router)
useThemeStore(pinia)
app.mount('#app')
