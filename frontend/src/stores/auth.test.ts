import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '../stores/auth'

describe('auth store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('starts logged out', () => {
    const auth = useAuthStore()
    expect(auth.isLoggedIn).toBe(false)
    expect(auth.user).toBeNull()
  })

  it('logout clears token', () => {
    localStorage.setItem('token', 'abc')
    const auth = useAuthStore()
    auth.logout()
    expect(auth.isLoggedIn).toBe(false)
    expect(localStorage.getItem('token')).toBeNull()
  })
})
