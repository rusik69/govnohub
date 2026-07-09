<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { packageApi } from '../api/client'
import RepoNav from '../components/RepoNav.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const packages = ref<any[]>([])
const pkgName = ref('')
const pkgVersion = ref('1.0.0')
const pkgContent = ref('sample package content')
const message = ref('')

onMounted(async () => {
  const { data } = await packageApi.list(owner.value, repo.value)
  packages.value = data
})

async function publish() {
  await packageApi.publish(owner.value, repo.value, {
    name: pkgName.value,
    version: pkgVersion.value,
    type: 'generic',
    content: pkgContent.value,
  })
  const { data } = await packageApi.list(owner.value, repo.value)
  packages.value = data
  message.value = 'Package published'
  pkgName.value = ''
}
</script>

<template>
  <div>
    <RepoNav />
    <div class="max-w-4xl mx-auto p-4">
      <div class="card p-4 mb-4 space-y-2">
        <h2 class="font-semibold">Publish package</h2>
        <input v-model="pkgName" class="input" placeholder="Package name" />
        <input v-model="pkgVersion" class="input" placeholder="Version" />
        <textarea v-model="pkgContent" class="input" rows="3" placeholder="Package content" />
        <button class="btn" :disabled="!pkgName" @click="publish">Publish</button>
        <p v-if="message" class="text-sm text-[#1a7f37]">{{ message }}</p>
      </div>
      <div class="card">
        <div v-for="p in packages" :key="p.id" class="px-4 py-3 border-b border-[#d0d7de] flex justify-between items-center">
          <span class="font-medium">{{ p.name }}</span>
          <div class="flex items-center gap-3">
            <span class="text-sm text-[#656d76]">{{ p.package_type }} · {{ p.version }}</span>
            <a class="text-sm" :href="packageApi.downloadUrl(owner, repo, p.name, p.version)" download>Download</a>
          </div>
        </div>
        <p v-if="!packages.length" class="p-4 text-[#656d76]">No packages published.</p>
      </div>
    </div>
  </div>
</template>
