<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { packageApi } from '../api/client'
import RepoNav from '../components/RepoNav.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const packages = ref<any[]>([])

onMounted(async () => {
  const { data } = await packageApi.list(owner.value, repo.value)
  packages.value = data
})
</script>

<template>
  <div>
    <RepoNav />
    <div class="max-w-4xl mx-auto p-4">
      <div class="card">
        <div v-for="p in packages" :key="p.id" class="px-4 py-3 border-b border-[#d0d7de] flex justify-between">
          <span class="font-medium">{{ p.name }}</span>
          <span class="text-sm text-[#656d76]">{{ p.package_type }} · {{ p.version }}</span>
        </div>
        <p v-if="!packages.length" class="p-4 text-[#656d76]">No packages published.</p>
      </div>
    </div>
  </div>
</template>
