<script setup lang="ts">
import { ref } from 'vue'
import { searchApi } from '../api/client'

const q = ref('')
const results = ref<any[]>([])

async function search() {
  const { data } = await searchApi.search(q.value)
  results.value = data
}
</script>

<template>
  <div class="max-w-4xl mx-auto p-6">
    <h1 class="text-2xl font-semibold mb-4">Search</h1>
    <div class="flex gap-2 mb-6">
      <input v-model="q" class="input" placeholder="Search repos, issues, code..." @keyup.enter="search" />
      <button class="btn" @click="search">Search</button>
    </div>
    <div class="card">
      <div v-for="hit in results" :key="hit.id" class="px-4 py-3 border-b border-[#d0d7de]">
        <span class="badge bg-[#ddf4ff] text-[#0969da] mr-2">{{ hit.type }}</span>
        <span class="font-medium">{{ hit.title }}</span>
        <p class="text-sm text-[#656d76] mt-1">{{ hit.repo }} — {{ hit.snippet }}</p>
      </div>
    </div>
  </div>
</template>
