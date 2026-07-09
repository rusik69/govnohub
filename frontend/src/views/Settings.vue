<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { repoApi, webhookApi, protectedBranchApi, collaboratorApi, issueApi } from '../api/client'
import RepoNav from '../components/RepoNav.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const webhooks = ref<any[]>([])
const hookUrl = ref('')
const hookSecret = ref('')
const protectedBranches = ref<any[]>([])
const protectBranch = ref('main')
const requiredChecks = ref('')
const requireReviews = ref(1)
const collaborators = ref<any[]>([])
const collabUsername = ref('')
const collabPermission = ref('read')
const labels = ref<any[]>([])
const labelName = ref('')
const labelColor = ref('#0366d6')
const branchName = ref('')
const branchBase = ref('main')
const message = ref('')
const error = ref('')

onMounted(async () => {
  try {
    const [hooks, rules, collabs, lbls] = await Promise.all([
      webhookApi.list(owner.value, repo.value),
      protectedBranchApi.list(owner.value, repo.value),
      collaboratorApi.list(owner.value, repo.value),
      issueApi.labels(owner.value, repo.value),
    ])
    webhooks.value = hooks.data
    protectedBranches.value = rules.data
    collaborators.value = collabs.data
    labels.value = lbls.data
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to load settings'
  }
})

async function addWebhook() {
  await webhookApi.create(owner.value, repo.value, { url: hookUrl.value, secret: hookSecret.value, events: ['push'] })
  const { data } = await webhookApi.list(owner.value, repo.value)
  webhooks.value = data
  hookUrl.value = ''
  hookSecret.value = ''
  message.value = 'Webhook added'
}

async function protect() {
  const checks = requiredChecks.value.split(',').map(s => s.trim()).filter(Boolean)
  await protectedBranchApi.protect(owner.value, repo.value, {
    branch: protectBranch.value,
    required_checks: checks,
    require_reviews: requireReviews.value,
  })
  const { data } = await protectedBranchApi.list(owner.value, repo.value)
  protectedBranches.value = data
  message.value = 'Branch protection updated'
}

async function addCollaborator() {
  if (!collabUsername.value) return
  await collaboratorApi.add(owner.value, repo.value, collabUsername.value, collabPermission.value)
  const { data } = await collaboratorApi.list(owner.value, repo.value)
  collaborators.value = data
  collabUsername.value = ''
  message.value = 'Collaborator added'
}

async function removeCollaborator(username: string) {
  await collaboratorApi.remove(owner.value, repo.value, username)
  const { data } = await collaboratorApi.list(owner.value, repo.value)
  collaborators.value = data
}

async function star() {
  await repoApi.star(owner.value, repo.value)
  message.value = 'Project starred'
}

async function watch() {
  await repoApi.watch(owner.value, repo.value)
  message.value = 'Now watching this project'
}

async function fork() {
  const { data } = await repoApi.fork(owner.value, repo.value)
  message.value = `Forked to ${data.full_name}`
}

async function createBranch() {
  await repoApi.createBranch(owner.value, repo.value, branchName.value, branchBase.value)
  branchName.value = ''
  message.value = 'Branch created'
}

async function addLabel() {
  await issueApi.createLabel(owner.value, repo.value, labelName.value, labelColor.value)
  const { data } = await issueApi.labels(owner.value, repo.value)
  labels.value = data
  labelName.value = ''
  message.value = 'Label created'
}
</script>

<template>
  <div>
    <RepoNav />
    <div class="max-w-4xl mx-auto p-4 space-y-4">
      <p v-if="message" class="text-sm text-[#1a7f37]">{{ message }}</p>
      <p v-if="error" class="text-sm text-red-600">{{ error }}</p>

      <div class="card p-4">
        <h3 class="font-semibold mb-2">Repository actions</h3>
        <div class="flex gap-2 flex-wrap">
          <button class="btn-secondary" @click="star">Star</button>
          <button class="btn-secondary" @click="watch">Watch</button>
          <button class="btn-secondary" @click="fork">Fork</button>
        </div>
      </div>

      <div class="card p-4">
        <h3 class="font-semibold mb-2">Branches</h3>
        <div class="flex flex-wrap gap-2">
          <input v-model="branchName" class="input" placeholder="New branch name" />
          <input v-model="branchBase" class="input" placeholder="Base branch" />
          <button class="btn" @click="createBranch">Create branch</button>
        </div>
      </div>

      <div class="card p-4">
        <h3 class="font-semibold mb-2">Labels</h3>
        <div class="flex flex-wrap gap-2 mb-3">
          <input v-model="labelName" class="input" placeholder="Label name" />
          <input v-model="labelColor" class="input w-32" placeholder="#0366d6" />
          <button class="btn" @click="addLabel">Add label</button>
        </div>
        <div class="flex flex-wrap gap-2">
          <span v-for="l in labels" :key="l.id" class="badge" :style="{ backgroundColor: l.color + '33', color: l.color }">{{ l.name }}</span>
        </div>
      </div>

      <div class="card p-4">
        <h3 class="font-semibold mb-2">Branch protection</h3>
        <div class="flex flex-wrap gap-2 mb-3">
          <input v-model="protectBranch" class="input" placeholder="Branch name" />
          <input v-model="requiredChecks" class="input flex-1" placeholder="Required checks (comma-separated)" />
          <input v-model.number="requireReviews" type="number" min="0" class="input w-24" placeholder="Reviews" />
          <button class="btn" @click="protect">Protect</button>
        </div>
        <div v-for="pb in protectedBranches" :key="pb.branch_name" class="text-sm py-1 border-t">
          <span class="font-medium">{{ pb.branch_name }}</span>
          — {{ pb.require_reviews }} review(s), checks: {{ pb.required_checks?.join(', ') || 'none' }}
        </div>
      </div>

      <div class="card p-4">
        <h3 class="font-semibold mb-2">Collaborators</h3>
        <div class="flex gap-2 mb-3">
          <input v-model="collabUsername" class="input" placeholder="Username" />
          <select v-model="collabPermission" class="input">
            <option value="read">Read</option>
            <option value="write">Write</option>
          </select>
          <button class="btn" @click="addCollaborator">Add</button>
        </div>
        <div v-for="c in collaborators" :key="c.user_id" class="flex items-center justify-between text-sm py-1 border-t">
          <span>{{ c.username }} ({{ c.permission }})</span>
          <button class="text-red-600 hover:underline" @click="removeCollaborator(c.username)">Remove</button>
        </div>
      </div>

      <div class="card p-4">
        <h3 class="font-semibold mb-2">Webhooks</h3>
        <div class="flex gap-2 mb-3">
          <input v-model="hookUrl" class="input" placeholder="Payload URL" />
          <input v-model="hookSecret" class="input" placeholder="Secret" />
          <button class="btn" @click="addWebhook">Add</button>
        </div>
        <div v-for="h in webhooks" :key="h.id" class="text-sm py-1">{{ h.url }} ({{ h.events?.join(', ') }})</div>
      </div>
    </div>
  </div>
</template>
