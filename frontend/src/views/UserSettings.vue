<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { tokenApi, type PATInfo } from '../api/client'

const tokens = ref<PATInfo[]>([])
const tokenName = ref('')
const newToken = ref('')
const selectedScopes = ref<string[]>(['repo'])

const allScopes = [
  { id: 'repo', label: 'Repository read' },
  { id: 'repo:write', label: 'Repository write' },
  { id: 'workflow', label: 'Workflow trigger' },
  { id: 'read:user', label: 'Read user profile' },
]

onMounted(loadTokens)

async function loadTokens() {
  const { data } = await tokenApi.list()
  tokens.value = data
}

function toggleScope(scope: string) {
  const i = selectedScopes.value.indexOf(scope)
  if (i >= 0) selectedScopes.value.splice(i, 1)
  else selectedScopes.value.push(scope)
}

async function createToken() {
  if (!tokenName.value) return
  const { data } = await tokenApi.create(tokenName.value, selectedScopes.value)
  newToken.value = data.token
  tokenName.value = ''
  await loadTokens()
}

async function revoke(id: string) {
  await tokenApi.revoke(id)
  await loadTokens()
}
</script>

<template>
  <div class="max-w-2xl mx-auto p-4 space-y-4">
    <h2 class="text-xl font-semibold">Personal access tokens</h2>

    <div v-if="newToken" class="card p-4 bg-yellow-50 border-yellow-200">
      <p class="text-sm font-medium mb-1">Copy your token now — it won't be shown again:</p>
      <code class="text-sm break-all">{{ newToken }}</code>
      <button class="btn-secondary mt-2" @click="newToken = ''">Dismiss</button>
    </div>

    <div class="card p-4">
      <h3 class="font-semibold mb-2">Create token</h3>
      <input v-model="tokenName" class="input mb-2 w-full" placeholder="Token name" />
      <div class="flex flex-wrap gap-3 mb-3">
        <label v-for="s in allScopes" :key="s.id" class="flex items-center gap-1 text-sm">
          <input type="checkbox" :checked="selectedScopes.includes(s.id)" @change="toggleScope(s.id)" />
          {{ s.label }}
        </label>
      </div>
      <button class="btn" @click="createToken">Generate token</button>
    </div>

    <div class="card p-4">
      <h3 class="font-semibold mb-2">Active tokens</h3>
      <div v-if="tokens.length === 0" class="text-sm text-[#656d76]">No tokens yet.</div>
      <div v-for="t in tokens" :key="t.id" class="flex items-center justify-between text-sm py-2 border-t">
        <div>
          <span class="font-medium">{{ t.name }}</span>
          <span class="text-[#656d76] ml-2">{{ t.scopes?.join(', ') }}</span>
        </div>
        <button class="text-red-600 hover:underline" @click="revoke(t.id)">Revoke</button>
      </div>
    </div>
  </div>
</template>
