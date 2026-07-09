import { beforeEach, vi } from 'vitest'

const store: Record<string, string> = {}

vi.stubGlobal('localStorage', {
  getItem: (k: string) => store[k] ?? null,
  setItem: (k: string, v: string) => { store[k] = v },
  removeItem: (k: string) => { delete store[k] },
  clear: () => { Object.keys(store).forEach(k => delete store[k]) },
})

beforeEach(() => {
  localStorage.clear()
})
