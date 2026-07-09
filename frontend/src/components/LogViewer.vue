<script setup lang="ts">
defineProps<{ jobs: { job_id: string; name: string; status: string; log: string }[] }>()
</script>

<template>
  <div class="card p-4">
    <h3 class="font-semibold mb-3">Run logs</h3>
    <div v-for="job in jobs" :key="job.job_id" class="mb-4">
      <p class="font-medium text-sm mb-1">{{ job.name }} — {{ job.status }}</p>
      <pre class="bg-[#0d1117] text-[#c9d1d9] p-3 rounded text-xs overflow-auto max-h-64">{{ job.log || 'No logs yet' }}</pre>
    </div>
  </div>
</template>
