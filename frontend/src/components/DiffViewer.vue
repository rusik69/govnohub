<script setup lang="ts">
defineProps<{ diff: string }>()
</script>

<template>
  <div class="card overflow-auto">
    <pre class="p-4 text-xs font-mono whitespace-pre-wrap">{{ diff || 'No diff available' }}</pre>
  </div>
</template>
