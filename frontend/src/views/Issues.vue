<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { issueApi } from '../api/client'
import RepoNav from '../components/RepoNav.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const issues = ref<any[]>([])
const title = ref('')
const body = ref('')

onMounted(async () => {
  const { data } = await issueApi.list(owner.value, repo.value)
  issues.value = data
})

async function create() {
  await issueApi.create(owner.value, repo.value, title.value, body.value)
  const { data } = await issueApi.list(owner.value, repo.value)
  issues.value = data
  title.value = ''
  body.value = ''
}
</script>

<template>
  <div>
    <RepoNav />
    <div class="max-w-4xl mx-auto p-4">
      <div class="card p-4 mb-4 space-y-2">
        <input v-model="title" class="input" placeholder="Issue title" />
        <textarea v-model="body" class="input" rows="3" placeholder="Description" />
        <button class="btn" @click="create">New issue</button>
      </div>
      <div class="card">
        <RouterLink
          v-for="issue in issues"
          :key="issue.id"
          :to="`/${owner}/${repo}/issues/${issue.number}`"
          class="flex items-center gap-3 px-4 py-3 border-b border-[#d0d7de] hover:bg-[#f6f8fa] no-underline text-inherit"
        >
          <span :class="['badge', issue.state === 'open' ? 'badge-open' : 'badge-closed']">{{ issue.state }}</span>
          <span class="font-medium">{{ issue.title }}</span>
          <span class="text-sm text-[#656d76] ml-auto">#{{ issue.number }}</span>
        </RouterLink>
      </div>
    </div>
  </div>
</template>
