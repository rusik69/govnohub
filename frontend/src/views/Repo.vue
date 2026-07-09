<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { repoApi, type Repository } from '../api/client'
import RepoNav from '../components/RepoNav.vue'
import FileTree from '../components/FileTree.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const project = ref<Repository | null>(null)
const entries = ref<any[]>([])
const commits = ref<any[]>([])
const fileContent = ref('')
const selectedPath = ref('')
const error = ref('')

onMounted(load)
async function load() {
  error.value = ''
  try {
    const [meta, tree, commitList] = await Promise.all([
      repoApi.get(owner.value, repo.value),
      repoApi.contents(owner.value, repo.value, ''),
      repoApi.commits(owner.value, repo.value),
    ])
    project.value = meta.data
    entries.value = tree.data
    commits.value = commitList.data
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to load project'
  }
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
    <div v-if="project" class="max-w-6xl mx-auto p-4">
      <p class="text-sm text-[#656d76] mb-4">{{ project.description || 'No description' }} · ⭐ {{ project.star_count }}</p>
      <div class="grid grid-cols-3 gap-4">
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
          <p v-if="!commits.length" class="text-sm text-[#656d76]">No commits yet.</p>
        </div>
      </div>
    </div>
    <p v-else-if="error" class="p-4 text-red-600">{{ error }}</p>
  </div>
</template>
