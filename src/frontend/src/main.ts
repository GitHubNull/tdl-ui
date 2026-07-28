import { createApp } from 'vue'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import Tooltip from 'primevue/tooltip'
import { definePreset } from '@primeuix/themes'
import Aura from '@primeuix/themes/aura'

import 'primeicons/primeicons.css'
import './style.css'

import App from './App.vue'
import { router } from './router'
import { initTheme } from './theme'

// 品牌蓝主色 + Aura 基调，简约风定制
const preset = definePreset(Aura, {
  semantic: {
    primary: {
      50: '{blue.50}',
      100: '{blue.100}',
      200: '{blue.200}',
      300: '{blue.300}',
      400: '{blue.400}',
      500: '{blue.500}',
      600: '{blue.600}',
      700: '{blue.700}',
      800: '{blue.800}',
      900: '{blue.900}',
      950: '{blue.950}',
    },
  },
})

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(PrimeVue, {
  theme: {
    preset,
    options: {
      darkModeSelector: '.app-dark',
    },
  },
})
app.use(ToastService)
app.directive('tooltip', Tooltip)

initTheme()

app.mount('#app')
