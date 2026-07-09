<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { repoApi, type Repository } from '../api/client'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const projects = ref<Repository[]>([])
const showNew = ref(false)
const newName = ref('')
const newDesc = ref('')
const newPrivate = ref(false)
const error = ref('')
const loading = ref(false)

onMounted(loadProjects)

async function loadProjects() {
  const { data } = await repoApi.list()
  projects.value = data
}

async function createProject() {
  if (!auth.user) return
  error.value = ''
  loading.value = true
  try {
    await repoApi.create(auth.user.username, newName.value.trim(), newDesc.value.trim(), newPrivate.value)
    await loadProjects()
    showNew.value = false
    newName.value = ''
    newDesc.value = ''
    newPrivate.value = false
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to create project'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="max-w-5xl mx-auto p-6">
    <div v-if="auth.isAdmin" class="card p-4 mb-6 flex items-center justify-between">
      <div>
        <h2 class="font-medium">User management</h2>
        <p class="text-sm text-[#656d76]">Create and manage accounts. Only admins can access this.</p>
      </div>
      <RouterLink to="/admin/users" class="btn">Manage users</RouterLink>
    </div>

    <div class="flex items-center justify-between mb-6">
      <div>
        <h1 class="text-2xl font-semibold">Projects</h1>
        <p class="text-sm text-[#656d76] mt-1">Create and manage your code projects.</p>
      </div>
      <button class="btn" @click="showNew = true">New project</button>
    </div>

    <div v-if="showNew" class="card p-4 mb-6 space-y-3">
      <h2 class="font-medium">Create project</h2>
      <input v-model="newName" class="input" placeholder="Project name" required />
      <input v-model="newDesc" class="input" placeholder="Description (optional)" />
      <label class="flex items-center gap-2 text-sm">
        <input v-model="newPrivate" type="checkbox" />
        Private project
      </label>
      <p v-if="error" class="text-red-600 text-sm">{{ error }}</p>
      <div class="flex gap-2">
        <button class="btn" :disabled="loading || !newName.trim()" @click="createProject">
          {{ loading ? 'Creating...' : 'Create project' }}
        </button>
        <button class="btn btn-secondary" type="button" @click="showNew = false">Cancel</button>
      </div>
    </div>

    <div class="card">
      <div v-for="project in projects" :key="project.id" class="px-4 py-3 border-b border-[#d0d7de] last:border-0">
        <RouterLink :to="`/${project.owner_name}/${project.name}`" class="font-semibold text-[#0969da]">
          {{ project.full_name }}
        </RouterLink>
        <p class="text-sm text-[#656d76] mt-1">{{ project.description || 'No description' }}</p>
      </div>
      <div v-if="!projects.length" class="p-8 text-center">
        <p class="text-[#656d76] mb-4">You do not have any projects yet.</p>
        <button class="btn" @click="showNew = true">Create your first project</button>
      </div>
    </div>
  </div>
</template>
