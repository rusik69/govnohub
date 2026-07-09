<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { issueApi, type IssueComment } from '../api/client'
import RepoNav from '../components/RepoNav.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const number = computed(() => Number(route.params.number))
const issue = ref<any>(null)
const comments = ref<IssueComment[]>([])
const comment = ref('')
const error = ref('')

async function load() {
  const [issueRes, commentsRes] = await Promise.all([
    issueApi.get(owner.value, repo.value, number.value),
    issueApi.comments(owner.value, repo.value, number.value),
  ])
  issue.value = issueRes.data
  comments.value = commentsRes.data
}

onMounted(async () => {
  try {
    await load()
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to load issue'
  }
})

async function addComment() {
  error.value = ''
  try {
    await issueApi.comment(owner.value, repo.value, number.value, comment.value)
    comment.value = ''
    const { data } = await issueApi.comments(owner.value, repo.value, number.value)
    comments.value = data
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to add comment'
  }
}

async function close() {
  try {
    await issueApi.close(owner.value, repo.value, number.value)
    issue.value.state = 'closed'
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to close issue'
  }
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

      <div v-if="comments.length" class="mt-6 space-y-3">
        <h2 class="font-medium">Comments</h2>
        <div v-for="c in comments" :key="c.id" class="card p-4">
          <p class="text-sm text-[#656d76] mb-2">{{ c.author || 'user' }} · {{ new Date(c.created_at).toLocaleString() }}</p>
          <p class="whitespace-pre-wrap">{{ c.body }}</p>
        </div>
      </div>

      <div class="mt-4 flex gap-2">
        <textarea v-model="comment" class="input flex-1" rows="2" placeholder="Leave a comment" />
        <button class="btn" @click="addComment">Comment</button>
        <button v-if="issue.state === 'open'" class="btn-secondary" @click="close">Close</button>
      </div>
      <p v-if="error" class="text-red-600 text-sm mt-2">{{ error }}</p>
    </div>
    <p v-else-if="error" class="p-4 text-red-600">{{ error }}</p>
  </div>
</template>
