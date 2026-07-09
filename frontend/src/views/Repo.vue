<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { repoApi } from '../api/client'
import RepoNav from '../components/RepoNav.vue'
import FileTree from '../components/FileTree.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const entries = ref<any[]>([])
const commits = ref<any[]>([])
const fileContent = ref('')
const selectedPath = ref('')

onMounted(load)
async function load() {
  try {
    const [tree, commitList] = await Promise.all([
      repoApi.contents(owner.value, repo.value, ''),
      repoApi.commits(owner.value, repo.value),
    ])
    entries.value = tree.data
    commits.value = commitList.data
  } catch {}
}

async function openFile(path: string) {
  selectedPath.value = path
  const { data } = await repoApi.contents(owner.value, repo.value, path)
  fileContent.value = typeof data === 'string' ? data : new TextDecoder().decode(data as any)
}
</script>

<template>
  <div>
    <RepoNav />
    <div class="max-w-6xl mx-auto p-4 grid grid-cols-3 gap-4">
      <div class="col-span-2 card">
        <FileTree :entries="entries" @select="openFile" />
        <pre v-if="fileContent" class="p-4 text-sm overflow-auto border-t border-[#d0d7de]">{{ fileContent }}</pre>
      </div>
      <div class="card p-4">
        <h3 class="font-semibold mb-3">Recent commits</h3>
        <div v-for="c in commits" :key="c.sha" class="mb-3 text-sm">
          <p class="font-medium">{{ c.message }}</p>
          <p class="text-[#656d76]">{{ c.author }} · {{ c.sha?.slice(0, 7) }}</p>
        </div>
      </div>
    </div>
  </div>
</template>
