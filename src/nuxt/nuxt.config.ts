// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  future: {
    compatibilityVersion: 4,
  },
  
  appConfig: {
    apiBase: 'http://localhost:8401/api/v1',
    account: 'Michael Brundins',
  },
  
  devServer: {
    port: 8400,
    host: '0.0.0.0',
  },
})
