<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { prApi } from '../api/client'
import RepoNav from '../components/RepoNav.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const prs = ref<any[]>([])
const title = ref('')
const body = ref('')
const head = ref('')
const base = ref('main')
const error = ref('')

onMounted(async () => {
  try {
    const { data } = await prApi.list(owner.value, repo.value)
    prs.value = data
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to load pull requests'
  }
})

async function create() {
  error.value = ''
  try {
    await prApi.create(owner.value, repo.value, { title: title.value, body: body.value, head: head.value, base: base.value })
    const { data } = await prApi.list(owner.value, repo.value)
    prs.value = data
    title.value = ''
    body.value = ''
    head.value = ''
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to create pull request'
  }
}
</script>

<template>
  <div>
    <RepoNav />
    <div class="max-w-4xl mx-auto p-4">
      <div class="card p-4 mb-4 grid grid-cols-3 gap-2">
        <input v-model="title" class="input col-span-3" placeholder="PR title" />
        <textarea v-model="body" class="input col-span-3" rows="2" placeholder="Description" />
        <input v-model="head" class="input" placeholder="Head branch" />
        <input v-model="base" class="input" placeholder="Base branch" />
        <button class="btn" @click="create">New pull request</button>
      </div>
      <p v-if="error" class="text-red-600 text-sm mb-4">{{ error }}</p>
      <div class="card">
        <RouterLink
          v-for="pr in prs"
          :key="pr.id"
          :to="`/${owner}/${repo}/pulls/${pr.number}`"
          class="flex items-center gap-3 px-4 py-3 border-b border-[#d0d7de] hover:bg-[#f6f8fa] no-underline text-inherit"
        >
          <span :class="['badge', pr.state === 'open' ? 'badge-open' : 'badge-closed']">{{ pr.state }}</span>
          <span class="font-medium">{{ pr.title }}</span>
          <span class="text-sm text-[#656d76]">{{ pr.head_branch }} → {{ pr.base_branch }}</span>
        </RouterLink>
      </div>
    </div>
  </div>
</template>
