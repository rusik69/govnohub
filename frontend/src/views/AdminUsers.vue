<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { adminApi, type User } from '../api/client'

const users = ref<User[]>([])
const username = ref('')
const email = ref('')
const password = ref('')
const role = ref('user')
const error = ref('')
const loading = ref(false)

async function load() {
  const { data } = await adminApi.listUsers()
  users.value = data
}

async function createUser() {
  error.value = ''
  loading.value = true
  try {
    await adminApi.createUser({
      username: username.value,
      email: email.value,
      password: password.value,
      role: role.value,
    })
    username.value = ''
    email.value = ''
    password.value = ''
    role.value = 'user'
    await load()
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to create user'
  } finally {
    loading.value = false
  }
}

async function removeUser(user: User) {
  if (!confirm(`Delete user ${user.username}?`)) return
  try {
    await adminApi.deleteUser(user.id)
    await load()
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to delete user'
  }
}

onMounted(load)
</script>

<template>
  <div class="max-w-4xl mx-auto p-6">
    <h1 class="text-2xl font-semibold mb-6">User management</h1>

    <div class="card p-6 mb-8">
      <h2 class="text-lg font-medium mb-4">Create user</h2>
      <form @submit.prevent="createUser" class="grid gap-4 md:grid-cols-2">
        <div>
          <label class="block text-sm mb-1">Username</label>
          <input v-model="username" class="input" required />
        </div>
        <div>
          <label class="block text-sm mb-1">Email</label>
          <input v-model="email" type="email" class="input" required />
        </div>
        <div>
          <label class="block text-sm mb-1">Password</label>
          <input v-model="password" type="password" class="input" required />
        </div>
        <div>
          <label class="block text-sm mb-1">Role</label>
          <select v-model="role" class="input">
            <option value="user">User</option>
            <option value="admin">Admin</option>
          </select>
        </div>
        <div class="md:col-span-2">
          <p v-if="error" class="text-red-600 text-sm mb-2">{{ error }}</p>
          <button type="submit" class="btn" :disabled="loading">Create user</button>
        </div>
      </form>
    </div>

    <div class="card overflow-hidden">
      <table class="w-full text-sm">
        <thead class="bg-gray-50 text-left">
          <tr>
            <th class="px-4 py-3">Username</th>
            <th class="px-4 py-3">Email</th>
            <th class="px-4 py-3">Role</th>
            <th class="px-4 py-3">Created</th>
            <th class="px-4 py-3"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="user in users" :key="user.id" class="border-t">
            <td class="px-4 py-3">{{ user.username }}</td>
            <td class="px-4 py-3">{{ user.email }}</td>
            <td class="px-4 py-3 capitalize">{{ user.role }}</td>
            <td class="px-4 py-3">{{ new Date(user.created_at || '').toLocaleDateString() }}</td>
            <td class="px-4 py-3 text-right">
              <button class="text-red-600 hover:underline" @click="removeUser(user)">Delete</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
