<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { prApi, type AIReview, type PRReview } from '../api/client'
import RepoNav from '../components/RepoNav.vue'
import DiffViewer from '../components/DiffViewer.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const number = computed(() => Number(route.params.number))
const pr = ref<any>(null)
const diff = ref('')
const reviews = ref<PRReview[]>([])
const reviewBody = ref('')
const aiEnabled = ref(false)
const aiReviews = ref<AIReview[]>([])
const aiLoading = ref(false)
const aiError = ref('')
const message = ref('')
const error = ref('')

async function loadReviews() {
  const { data } = await prApi.reviews(owner.value, repo.value, number.value)
  reviews.value = data
}

onMounted(async () => {
  try {
    const [prRes, diffRes, cfgRes, reviewsRes, aiReviewsRes] = await Promise.all([
      prApi.get(owner.value, repo.value, number.value),
      prApi.diff(owner.value, repo.value, number.value),
      prApi.aiReviewConfig(owner.value, repo.value),
      prApi.reviews(owner.value, repo.value, number.value),
      prApi.aiReviews(owner.value, repo.value, number.value),
    ])
    pr.value = prRes.data
    diff.value = diffRes.data as string
    aiEnabled.value = cfgRes.data.enabled
    reviews.value = reviewsRes.data
    aiReviews.value = aiReviewsRes.data
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to load pull request'
  }
})

async function review(state: string) {
  error.value = ''
  try {
    await prApi.review(owner.value, repo.value, number.value, state, reviewBody.value)
    reviewBody.value = ''
    message.value = `Review submitted: ${state}`
    await loadReviews()
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to submit review'
  }
}

async function merge() {
  error.value = ''
  try {
    await prApi.merge(owner.value, repo.value, number.value)
    pr.value.state = 'closed'
    message.value = 'Pull request merged'
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to merge pull request'
  }
}

async function requestAIReview() {
  aiLoading.value = true
  aiError.value = ''
  try {
    const res = await prApi.requestAIReview(owner.value, repo.value, number.value)
    aiReviews.value = [res.data, ...aiReviews.value]
  } catch (e: any) {
    aiError.value = e?.response?.data?.error || 'AI review failed'
  } finally {
    aiLoading.value = false
  }
}
</script>

<template>
  <div>
    <RepoNav />
    <div v-if="pr" class="max-w-6xl mx-auto p-4">
      <h1 class="text-2xl font-semibold">{{ pr.title }}</h1>
      <p class="text-sm text-[#656d76] mt-1">{{ pr.head_branch }} → {{ pr.base_branch }}</p>
      <p v-if="message" class="text-sm text-[#1a7f37] mt-2">{{ message }}</p>
      <p v-if="error" class="text-sm text-red-600 mt-2">{{ error }}</p>

      <div class="mt-4 space-y-2">
        <textarea v-model="reviewBody" class="input" rows="3" placeholder="Review comment (optional)" />
        <div class="flex gap-2">
          <button class="btn" @click="review('approved')">Approve</button>
          <button class="btn-secondary" @click="review('changes_requested')">Request changes</button>
          <button class="btn" @click="merge">Merge</button>
        </div>
      </div>

      <div v-if="reviews.length" class="mt-6 card">
        <h2 class="font-medium px-4 py-3 border-b border-[#d0d7de]">Reviews</h2>
        <div v-for="r in reviews" :key="r.id" class="px-4 py-3 border-b border-[#d0d7de] last:border-b-0">
          <p class="text-sm text-[#656d76]">{{ r.reviewer || 'user' }} · {{ r.state }} · {{ new Date(r.created_at).toLocaleString() }}</p>
          <p v-if="r.body" class="mt-1 whitespace-pre-wrap">{{ r.body }}</p>
        </div>
      </div>

      <div v-if="aiEnabled" class="mt-6 border border-[#d0d7de] rounded-md">
        <div class="px-4 py-3 border-b border-[#d0d7de] bg-[#f6f8fa] flex items-center gap-3">
          <span class="font-medium text-sm">AI Code Review</span>
          <button class="btn text-sm" :disabled="aiLoading" @click="requestAIReview">
            {{ aiLoading ? 'Reviewing…' : 'Run AI Review' }}
          </button>
        </div>
        <p v-if="aiError" class="px-4 py-2 text-sm text-red-600">{{ aiError }}</p>
        <div v-for="r in aiReviews" :key="r.id" class="px-4 py-3 border-b border-[#d0d7de] last:border-b-0">
          <p class="text-xs text-[#656d76] mb-2">{{ r.model }} · {{ new Date(r.created_at).toLocaleString() }}</p>
          <pre class="text-sm whitespace-pre-wrap font-sans">{{ r.body }}</pre>
        </div>
      </div>

      <DiffViewer :diff="diff" class="mt-4" />
    </div>
    <p v-else-if="error" class="p-4 text-red-600">{{ error }}</p>
  </div>
</template>
