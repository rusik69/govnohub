<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { repoApi, type Repository } from '../api/client'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const repos = ref<Repository[]>([])
const showNew = ref(false)
const newName = ref('')
const newDesc = ref('')

onMounted(async () => {
  const { data } = await repoApi.list()
  repos.value = data
})

async function createRepo() {
  if (!auth.user) return
  await repoApi.create(auth.user.username, newName.value, newDesc.value, false)
  const { data } = await repoApi.list()
  repos.value = data
  showNew.value = false
  newName.value = ''
}
</script>

<template>
  <div class="max-w-5xl mx-auto p-6">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-semibold">Dashboard</h1>
      <button class="btn" @click="showNew = true">New repository</button>
    </div>

    <div v-if="showNew" class="card p-4 mb-6 space-y-3">
      <input v-model="newName" class="input" placeholder="Repository name" />
      <input v-model="newDesc" class="input" placeholder="Description" />
      <button class="btn" @click="createRepo">Create</button>
    </div>

    <div class="card">
      <div v-for="repo in repos" :key="repo.id" class="px-4 py-3 border-b border-[#d0d7de] last:border-0">
        <RouterLink :to="`/${repo.owner_name}/${repo.name}`" class="font-semibold text-[#0969da]">
          {{ repo.full_name }}
        </RouterLink>
        <p class="text-sm text-[#656d76] mt-1">{{ repo.description || 'No description' }}</p>
      </div>
      <p v-if="!repos.length" class="p-4 text-[#656d76]">No repositories yet.</p>
    </div>
  </div>
</template>
