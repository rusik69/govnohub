<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, RouterLink } from 'vue-router'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const base = computed(() => `/${owner.value}/${repo.value}`)

const tabs = [
  { name: 'Code', path: '' },
  { name: 'Issues', path: '/issues' },
  { name: 'Pull requests', path: '/pulls' },
  { name: 'Actions', path: '/actions' },
  { name: 'Releases', path: '/releases' },
  { name: 'Packages', path: '/packages' },
  { name: 'Settings', path: '/settings' },
]
</script>

<template>
  <div class="border-b border-[#d0d7de] bg-white">
    <div class="max-w-6xl mx-auto px-4 py-4">
      <h1 class="text-xl font-semibold">
        <span class="text-[#656d76]">{{ owner }}/</span>{{ repo }}
      </h1>
      <nav class="flex gap-4 mt-4">
        <RouterLink
          v-for="tab in tabs"
          :key="tab.name"
          :to="base + tab.path"
          class="text-sm py-2 border-b-2 border-transparent hover:border-[#fd8c73]"
          active-class="!border-[#fd8c73] font-semibold"
        >
          {{ tab.name }}
        </RouterLink>
      </nav>
    </div>
  </div>
</template>
