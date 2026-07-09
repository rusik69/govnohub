<script setup lang="ts">
defineProps<{ status: string; conclusion?: string | null }>()
</script>

<template>
  <span
    class="inline-block w-3 h-3 rounded-full mr-2"
    :class="{
      'bg-yellow-400': status === 'queued' || status === 'in_progress',
      'bg-green-500': conclusion === 'success',
      'bg-red-500': conclusion === 'failure',
      'bg-gray-300': !conclusion && status === 'completed',
    }"
  />
</template>
