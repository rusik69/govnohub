<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { repoApi, webhookApi } from '../api/client'
import RepoNav from '../components/RepoNav.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const webhooks = ref<any[]>([])
const hookUrl = ref('')
const hookSecret = ref('')

onMounted(async () => {
  const { data } = await webhookApi.list(owner.value, repo.value)
  webhooks.value = data
})

async function addWebhook() {
  await webhookApi.create(owner.value, repo.value, { url: hookUrl.value, secret: hookSecret.value, events: ['push'] })
  const { data } = await webhookApi.list(owner.value, repo.value)
  webhooks.value = data
}

async function star() {
  await repoApi.star(owner.value, repo.value)
}

async function fork() {
  await repoApi.fork(owner.value, repo.value)
}
</script>

<template>
  <div>
    <RepoNav />
    <div class="max-w-4xl mx-auto p-4 space-y-4">
      <div class="card p-4">
        <h3 class="font-semibold mb-2">Repository actions</h3>
        <div class="flex gap-2">
          <button class="btn-secondary" @click="star">Star</button>
          <button class="btn-secondary" @click="fork">Fork</button>
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
      <div class="card p-4">
        <h3 class="font-semibold mb-2">Branch protection</h3>
        <p class="text-sm text-[#656d76]">Configure required checks and review requirements via API.</p>
      </div>
    </div>
  </div>
</template>
