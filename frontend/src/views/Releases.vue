<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { releaseApi } from '../api/client'
import RepoNav from '../components/RepoNav.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const releases = ref<any[]>([])
const tag = ref('')
const name = ref('')
const body = ref('')

onMounted(async () => {
  const { data } = await releaseApi.list(owner.value, repo.value)
  releases.value = data
})

async function create() {
  await releaseApi.create(owner.value, repo.value, { tag_name: tag.value, name: name.value, body: body.value })
  const { data } = await releaseApi.list(owner.value, repo.value)
  releases.value = data
}
</script>

<template>
  <div>
    <RepoNav />
    <div class="max-w-4xl mx-auto p-4">
      <div class="card p-4 mb-4 space-y-2">
        <input v-model="tag" class="input" placeholder="Tag (v1.0.0)" />
        <input v-model="name" class="input" placeholder="Release title" />
        <textarea v-model="body" class="input" rows="3" placeholder="Release notes" />
        <button class="btn" @click="create">Create release</button>
      </div>
      <div class="card">
        <div v-for="r in releases" :key="r.id" class="px-4 py-3 border-b border-[#d0d7de]">
          <h3 class="font-semibold">{{ r.name || r.tag_name }}</h3>
          <p class="text-sm text-[#656d76]">{{ r.tag_name }}</p>
          <p class="mt-2 text-sm whitespace-pre-wrap">{{ r.body }}</p>
        </div>
      </div>
    </div>
  </div>
</template>
