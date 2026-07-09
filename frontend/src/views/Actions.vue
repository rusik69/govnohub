<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { actionsApi } from '../api/client'
import RepoNav from '../components/RepoNav.vue'
import WorkflowGraph from '../components/WorkflowGraph.vue'
import LogViewer from '../components/LogViewer.vue'

const route = useRoute()
const owner = computed(() => route.params.owner as string)
const repo = computed(() => route.params.repo as string)
const workflows = ref<any[]>([])
const runs = ref<any[]>([])
const selectedRun = ref('')
const logs = ref<any[]>([])

onMounted(load)
async function load() {
  const [wf, r] = await Promise.all([
    actionsApi.workflows(owner.value, repo.value),
    actionsApi.runs(owner.value, repo.value),
  ])
  workflows.value = wf.data
  runs.value = r.data
}

async function viewLogs(runId: string) {
  selectedRun.value = runId
  const { data } = await actionsApi.logs(owner.value, repo.value, runId)
  logs.value = data
}

async function trigger(wf: any) {
  await actionsApi.trigger(owner.value, repo.value, { workflow_id: wf.id, event: 'workflow_dispatch' })
  await load()
}
</script>

<template>
  <div>
    <RepoNav />
    <div class="max-w-6xl mx-auto p-4 grid grid-cols-2 gap-4">
      <div>
        <h2 class="font-semibold mb-2">Workflows</h2>
        <div class="card mb-4">
          <div v-for="wf in workflows" :key="wf.id" class="flex justify-between px-4 py-3 border-b border-[#d0d7de]">
            <span>{{ wf.name }}</span>
            <button class="btn-secondary text-xs" @click="trigger(wf)">Run</button>
          </div>
        </div>
        <h2 class="font-semibold mb-2">Runs</h2>
        <div class="card">
          <div
            v-for="run in runs"
            :key="run.id"
            class="px-4 py-3 border-b border-[#d0d7de] cursor-pointer hover:bg-[#f6f8fa]"
            @click="viewLogs(run.id)"
          >
            <WorkflowGraph :status="run.status" :conclusion="run.conclusion" />
            <span class="text-sm">#{{ run.run_number }} {{ run.event }} · {{ run.head_branch }}</span>
          </div>
        </div>
      </div>
      <LogViewer v-if="selectedRun" :jobs="logs" />
    </div>
  </div>
</template>
