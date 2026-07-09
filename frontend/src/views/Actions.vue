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
const wfName = ref('CI')
const wfPath = ref('.govnohub/workflows/ci.yaml')
const wfContent = ref('name: CI\non: [push]\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hello\n')
const message = ref('')
const error = ref('')

onMounted(load)
async function load() {
  try {
    const [wf, r] = await Promise.all([
      actionsApi.workflows(owner.value, repo.value),
      actionsApi.runs(owner.value, repo.value),
    ])
    workflows.value = wf.data
    runs.value = r.data
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to load actions'
  }
}

async function viewLogs(runId: string) {
  selectedRun.value = runId
  const { data } = await actionsApi.logs(owner.value, repo.value, runId)
  logs.value = data
}

async function trigger(wf: any) {
  await actionsApi.trigger(owner.value, repo.value, { workflow_id: wf.id, event: 'workflow_dispatch' })
  message.value = `Triggered ${wf.name}`
  await load()
}

async function saveWorkflow() {
  await actionsApi.upsertWorkflow(owner.value, repo.value, {
    name: wfName.value,
    path: wfPath.value,
    content: wfContent.value,
  })
  message.value = 'Workflow saved'
  await load()
}
</script>

<template>
  <div>
    <RepoNav />
    <div class="max-w-6xl mx-auto p-4 grid grid-cols-2 gap-4">
      <div>
        <p v-if="message" class="text-sm text-[#1a7f37] mb-2">{{ message }}</p>
        <p v-if="error" class="text-sm text-red-600 mb-2">{{ error }}</p>

        <div class="card p-4 mb-4 space-y-2">
          <h2 class="font-semibold">Add workflow</h2>
          <input v-model="wfName" class="input" placeholder="Workflow name" />
          <input v-model="wfPath" class="input" placeholder="Path" />
          <textarea v-model="wfContent" class="input font-mono text-xs" rows="6" />
          <button class="btn" @click="saveWorkflow">Save workflow</button>
        </div>

        <h2 class="font-semibold mb-2">Workflows</h2>
        <div class="card mb-4">
          <div v-for="wf in workflows" :key="wf.id" class="flex justify-between px-4 py-3 border-b border-[#d0d7de]">
            <span>{{ wf.name }}</span>
            <button class="btn-secondary text-xs" @click="trigger(wf)">Run</button>
          </div>
          <p v-if="!workflows.length" class="p-4 text-[#656d76]">No workflows yet.</p>
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
          <p v-if="!runs.length" class="p-4 text-[#656d76]">No runs yet.</p>
        </div>
      </div>
      <LogViewer v-if="selectedRun" :jobs="logs" />
    </div>
  </div>
</template>
