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
const ready = ref(false)

onMounted(async () => {
  if (!auth.user && auth.token) {
    try {
      await auth.fetchUser()
    } catch {
      error.value = 'Session expired. Please sign in again.'
      return
    }
  }
  await loadProjects()
  ready.value = true
})

async function loadProjects() {
  try {
    const { data } = await repoApi.list()
    projects.value = Array.isArray(data) ? data : []
  } catch (e: any) {
    projects.value = []
    error.value = e.response?.data?.error || 'Failed to load projects'
  }
}

async function createProject() {
  error.value = ''
  if (!auth.user) {
    try {
      await auth.fetchUser()
    } catch {
      error.value = 'Session expired. Please sign in again.'
      return
    }
  }
  const name = newName.value.trim()
  if (!name) {
    error.value = 'Project name is required'
    return
  }
  loading.value = true
  try {
    const { data } = await repoApi.create(auth.user!.username, name, newDesc.value.trim(), newPrivate.value)
    projects.value = [data, ...projects.value.filter((p) => p.id !== data.id)]
    showNew.value = false
    newName.value = ''
    newDesc.value = ''
    newPrivate.value = false
    error.value = ''
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to create project'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div v-if="!ready" class="max-w-5xl mx-auto p-6 text-sm text-[var(--text-muted)]">Loading projects...</div>
  <div v-else class="max-w-5xl mx-auto p-6">
    <div v-if="auth.isAdmin" class="card p-4 mb-6 flex items-center justify-between">
      <div>
        <h2 class="font-medium">User management</h2>
        <p class="text-sm text-[var(--text-muted)]">Create and manage accounts. Only admins can access this.</p>
      </div>
      <RouterLink to="/admin/users" class="btn">Manage users</RouterLink>
    </div>

    <div class="flex items-center justify-between mb-6">
      <div>
        <h1 class="text-2xl font-semibold">Projects</h1>
        <p class="text-sm text-[var(--text-muted)] mt-1">Create and manage your code projects.</p>
      </div>
      <button type="button" class="btn" @click="showNew = true">New project</button>
    </div>

    <form v-if="showNew" class="card p-4 mb-6 space-y-3" @submit.prevent="createProject">
      <h2 class="font-medium">Create project</h2>
      <input v-model="newName" class="input" placeholder="Project name" required />
      <input v-model="newDesc" class="input" placeholder="Description (optional)" />
      <label class="flex items-center gap-2 text-sm">
        <input v-model="newPrivate" type="checkbox" />
        Private project
      </label>
      <p v-if="error" class="text-red-600 text-sm">{{ error }}</p>
      <div class="flex gap-2">
        <button type="submit" class="btn" :disabled="loading || !newName.trim()">
          {{ loading ? 'Creating...' : 'Create project' }}
        </button>
        <button type="button" class="btn btn-secondary" @click="showNew = false">Cancel</button>
      </div>
    </form>

    <p v-if="error && !showNew" class="text-red-600 text-sm mb-4">{{ error }}</p>

    <div class="card">
      <div v-for="project in projects" :key="project.id" class="px-4 py-3 border-b border-[var(--border)] last:border-0">
        <RouterLink :to="`/${project.owner_name}/${project.name}`" class="font-semibold text-[var(--link)]">
          {{ project.full_name }}
        </RouterLink>
        <p class="text-sm text-[var(--text-muted)] mt-1">{{ project.description || 'No description' }}</p>
      </div>
      <div v-if="projects.length === 0" class="p-8 text-center">
        <p class="text-[var(--text-muted)] mb-4">You do not have any projects yet.</p>
        <button type="button" class="btn" @click="showNew = true">Create your first project</button>
      </div>
    </div>
  </div>
</template>
