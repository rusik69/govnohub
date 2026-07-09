<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import { useAuthStore } from './stores/auth'

const auth = useAuthStore()
</script>

<template>
  <div class="min-h-screen">
    <header class="bg-[#24292f] text-white px-4 py-3 flex items-center gap-4">
      <RouterLink to="/" class="font-bold text-white hover:no-underline flex items-center gap-2">
        <span class="text-xl">🐙</span> Govnohub
      </RouterLink>
      <RouterLink to="/search" class="text-sm text-white/80 hover:text-white hover:no-underline">Search</RouterLink>
      <div class="flex-1" />
      <template v-if="auth.isLoggedIn">
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
