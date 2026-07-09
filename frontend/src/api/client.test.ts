import { describe, it, expect } from 'vitest'

describe('api client', () => {
  it('exports auth and repo APIs', async () => {
    const client = await import('../api/client')
    expect(client.authApi).toBeDefined()
    expect(client.repoApi).toBeDefined()
    expect(client.issueApi).toBeDefined()
    expect(client.tokenApi).toBeDefined()
    expect(client.protectedBranchApi).toBeDefined()
    expect(client.collaboratorApi).toBeDefined()
  })
})
