<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { prApi } from '../api/client'
import RepoNav from '../components/RepoNav.vue'
import DiffViewer from '../components/DiffViewer.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const number = computed(() => Number(route.params.number))
const pr = ref<any>(null)
const diff = ref('')
const reviewBody = ref('')

onMounted(async () => {
  try {
    const [prRes, diffRes] = await Promise.all([
      prApi.get(owner.value, repo.value, number.value),
      prApi.diff(owner.value, repo.value, number.value),
    ])
    pr.value = prRes.data
    diff.value = diffRes.data as string
  } catch {
    pr.value = null
  }
})

async function review(state: string) {
  try {
    await prApi.review(owner.value, repo.value, number.value, state, reviewBody.value)
  } catch { /* ignore */ }
}

async function merge() {
  try {
    await prApi.merge(owner.value, repo.value, number.value)
    pr.value.state = 'closed'
  } catch { /* ignore */ }
}
</script>

<template>
  <div>
    <RepoNav />
    <div v-if="pr" class="max-w-6xl mx-auto p-4">
      <h1 class="text-2xl font-semibold">{{ pr.title }}</h1>
      <p class="text-sm text-[#656d76] mt-1">{{ pr.head_branch }} → {{ pr.base_branch }}</p>
      <div class="flex gap-2 mt-4">
        <button class="btn" @click="review('approved')">Approve</button>
        <button class="btn-secondary" @click="review('changes_requested')">Request changes</button>
        <button class="btn" @click="merge">Merge</button>
      </div>
      <DiffViewer :diff="diff" class="mt-4" />
    </div>
  </div>
</template>
