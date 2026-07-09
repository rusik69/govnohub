<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()
const username = ref('')
const password = ref('')
const error = ref('')

async function submit() {
  error.value = ''
  try {
    await auth.login(username.value, password.value)
    router.push('/')
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Login failed'
  }
}
</script>

<template>
  <div class="max-w-md mx-auto mt-20 card p-8">
    <h1 class="text-2xl font-semibold mb-6">Sign in to Govnohub</h1>
    <form @submit.prevent="submit" class="space-y-4">
      <div>
        <label class="block text-sm mb-1">Username</label>
        <input v-model="username" class="input" required />
      </div>
      <div>
        <label class="block text-sm mb-1">Password</label>
        <input v-model="password" type="password" class="input" required />
      </div>
      <p v-if="error" class="text-red-600 text-sm">{{ error }}</p>
      <button type="submit" class="btn w-full justify-center">Sign in</button>
    </form>
    <p class="mt-4 text-sm text-center text-gray-600">
      Contact an administrator to create an account.
    </p>
  </div>
</template>
