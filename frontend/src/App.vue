<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import { useAuthStore } from './stores/auth'
import { useThemeStore } from './stores/theme'

const auth = useAuthStore()
const theme = useThemeStore()
</script>

<template>
  <div class="min-h-screen">
    <header class="bg-[#24292f] dark:bg-[#010409] text-white px-4 py-3 flex items-center gap-4">
      <RouterLink to="/" class="font-bold text-white hover:no-underline flex items-center gap-2">
        <span class="text-xl">🐙</span> Govnohub
      </RouterLink>
      <template v-if="auth.isLoggedIn">
        <RouterLink to="/" class="text-sm text-white/80 hover:text-white hover:no-underline">Projects</RouterLink>
        <RouterLink to="/search" class="text-sm text-white/80 hover:text-white hover:no-underline">Search</RouterLink>
      </template>
      <div class="flex-1" />
      <button
        class="text-sm text-white/80 hover:text-white px-2 py-1 rounded border border-white/20"
        :title="theme.theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'"
        @click="theme.toggle()"
      >
        {{ theme.theme === 'dark' ? '☀️ Light' : '🌙 Dark' }}
      </button>
      <template v-if="auth.isLoggedIn">
        <RouterLink v-if="auth.isAdmin" to="/admin/users" class="text-sm text-white/80 hover:text-white hover:no-underline">User management</RouterLink>
        <RouterLink to="/settings" class="text-sm text-white/80 hover:text-white hover:no-underline">Settings</RouterLink>
        <span class="text-sm">{{ auth.user?.username }}</span>
        <button class="text-sm text-white/80 hover:text-white" @click="auth.logout()">Sign out</button>
      </template>
    </header>
    <main>
      <RouterView />
    </main>
  </div>
</template>
