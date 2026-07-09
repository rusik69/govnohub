<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { issueApi } from '../api/client'
import RepoNav from '../components/RepoNav.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const number = computed(() => Number(route.params.number))
const issue = ref<any>(null)
const comment = ref('')

onMounted(async () => {
  try {
    const { data } = await issueApi.get(owner.value, repo.value, number.value)
    issue.value = data
  } catch {
    issue.value = null
  }
})

async function addComment() {
  try {
    await issueApi.comment(owner.value, repo.value, number.value, comment.value)
    comment.value = ''
  } catch { /* ignore */ }
}

async function close() {
  try {
    await issueApi.close(owner.value, repo.value, number.value)
    issue.value.state = 'closed'
  } catch { /* ignore */ }
}
</script>

<template>
  <div>
    <RepoNav />
    <div v-if="issue" class="max-w-4xl mx-auto p-4">
      <h1 class="text-2xl font-semibold mb-2">{{ issue.title }}</h1>
      <span :class="['badge', issue.state === 'open' ? 'badge-open' : 'badge-closed']">{{ issue.state }}</span>
      <div class="card p-4 mt-4">
        <p class="whitespace-pre-wrap">{{ issue.body }}</p>
      </div>
      <div class="mt-4 flex gap-2">
        <textarea v-model="comment" class="input flex-1" rows="2" placeholder="Leave a comment" />
        <button class="btn" @click="addComment">Comment</button>
        <button v-if="issue.state === 'open'" class="btn-secondary" @click="close">Close</button>
      </div>
    </div>
  </div>
</template>
