<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink } from 'vue-router'
import { searchApi, type SearchHit } from '../api/client'

const q = ref('')
const results = ref<SearchHit[]>([])
const error = ref('')

function hitLink(hit: SearchHit): string | null {
  if (!hit.repo) return null
  const [owner, repo] = hit.repo.split('/')
  if (!owner || !repo) return null
  if (hit.type === 'issue' && hit.ref) return `/${owner}/${repo}/issues/${hit.ref}`
  if (hit.type === 'pull' && hit.ref) return `/${owner}/${repo}/pulls/${hit.ref}`
  return `/${owner}/${repo}`
}

async function search() {
  error.value = ''
  try {
    const { data } = await searchApi.search(q.value)
    results.value = data
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Search failed'
    results.value = []
  }
}
</script>

<template>
  <div class="max-w-4xl mx-auto p-6">
    <h1 class="text-2xl font-semibold mb-4">Search</h1>
    <div class="flex gap-2 mb-6">
      <input v-model="q" class="input" placeholder="Search repos, issues, code..." @keyup.enter="search" />
      <button class="btn" @click="search">Search</button>
    </div>
    <p v-if="error" class="text-red-600 text-sm mb-4">{{ error }}</p>
    <div class="card">
      <div v-for="hit in results" :key="hit.id" class="px-4 py-3 border-b border-[#d0d7de]">
        <span class="badge bg-[#ddf4ff] text-[#0969da] mr-2">{{ hit.type }}</span>
        <RouterLink v-if="hitLink(hit)" :to="hitLink(hit)!" class="font-medium">{{ hit.title }}</RouterLink>
        <span v-else class="font-medium">{{ hit.title }}</span>
        <p class="text-sm text-[#656d76] mt-1">{{ hit.repo }} — {{ hit.snippet }}</p>
      </div>
      <p v-if="!results.length && q && !error" class="p-4 text-[#656d76]">No results found.</p>
    </div>
  </div>
</template>
