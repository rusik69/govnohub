import axios from 'axios'

const api = axios.create({ baseURL: '/api/v1' })

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

export interface User {
  id: string
  username: string
  email: string
  role: string
  created_at?: string
}

export interface Repository {
  id: string
  owner_name: string
  name: string
  full_name: string
  description: string
  default_branch: string
  is_private: boolean
  star_count: number
}

export const authApi = {
  register: (username: string, email: string, password: string) =>
    api.post('/users', { username, email, password }),
  login: (username: string, password: string) =>
    api.post('/auth/login', { username, password }),
  me: () => api.get<User>('/user'),
}

export const adminApi = {
  listUsers: () => api.get<User[]>('/admin/users'),
  createUser: (data: { username: string; email: string; password: string; role: string }) =>
    api.post<User>('/admin/users', data),
  deleteUser: (id: string) => api.delete(`/admin/users/${id}`),
}

export interface PATInfo {
  id: string
  name: string
  scopes: string[]
  created_at: string
}

export const tokenApi = {
  create: (name: string, scopes: string[]) => api.post<{ token: string }>('/user/tokens', { name, scopes }),
  list: () => api.get<PATInfo[]>('/user/tokens'),
  revoke: (id: string) => api.delete(`/user/tokens/${id}`),
}

export interface ProtectedBranch {
  branch_name: string
  required_checks: string[]
  require_reviews: number
}

export const protectedBranchApi = {
  list: (owner: string, repo: string) => api.get<ProtectedBranch[]>(`/repos/${owner}/${repo}/protected-branches`),
  protect: (owner: string, repo: string, data: { branch: string; required_checks: string[]; require_reviews: number }) =>
    api.post(`/repos/${owner}/${repo}/protected-branches`, data),
}

export interface Collaborator {
  user_id: string
  username: string
  permission: string
}

export const collaboratorApi = {
  list: (owner: string, repo: string) => api.get<Collaborator[]>(`/repos/${owner}/${repo}/collaborators`),
  add: (owner: string, repo: string, username: string, permission: string) =>
    api.put(`/repos/${owner}/${repo}/collaborators/${username}`, { permission }),
  remove: (owner: string, repo: string, username: string) =>
    api.delete(`/repos/${owner}/${repo}/collaborators/${username}`),
}

export const repoApi = {
  list: () => api.get<Repository[]>('/user/repos'),
  get: (owner: string, repo: string) => api.get<Repository>(`/repos/${owner}/${repo}`),
  create: (owner: string, name: string, description: string, isPrivate: boolean) =>
    api.post(`/users/${owner}/repos`, { name, description, private: isPrivate }),
  contents: (owner: string, repo: string, path: string, ref?: string) =>
    api.get(`/repos/${owner}/${repo}/contents/${path}`, { params: { ref } }),
  commits: (owner: string, repo: string, ref?: string) =>
    api.get(`/repos/${owner}/${repo}/commits`, { params: { ref } }),
  star: (owner: string, repo: string) => api.post(`/repos/${owner}/${repo}/star`),
  fork: (owner: string, repo: string) => api.post(`/repos/${owner}/${repo}/fork`),
}

export const issueApi = {
  list: (owner: string, repo: string) => api.get(`/repos/${owner}/${repo}/issues`),
  create: (owner: string, repo: string, title: string, body: string) =>
    api.post(`/repos/${owner}/${repo}/issues`, { title, body }),
  get: (owner: string, repo: string, number: number) =>
    api.get(`/repos/${owner}/${repo}/issues/${number}`),
  comment: (owner: string, repo: string, number: number, body: string) =>
    api.post(`/repos/${owner}/${repo}/issues/${number}/comments`, { body }),
  close: (owner: string, repo: string, number: number) =>
    api.post(`/repos/${owner}/${repo}/issues/${number}/close`),
}

export const prApi = {
  list: (owner: string, repo: string) => api.get(`/repos/${owner}/${repo}/pulls`),
  create: (owner: string, repo: string, data: { title: string; body: string; head: string; base: string }) =>
    api.post(`/repos/${owner}/${repo}/pulls`, data),
  get: (owner: string, repo: string, number: number) =>
    api.get(`/repos/${owner}/${repo}/pulls/${number}`),
  review: (owner: string, repo: string, number: number, state: string, body: string) =>
    api.post(`/repos/${owner}/${repo}/pulls/${number}/reviews`, { state, body }),
  merge: (owner: string, repo: string, number: number, squash = false) =>
    api.post(`/repos/${owner}/${repo}/pulls/${number}/merge`, { squash }),
  diff: (owner: string, repo: string, number: number) =>
    api.get(`/repos/${owner}/${repo}/pulls/${number}/diff`, { responseType: 'text' }),
  aiReviewConfig: (owner: string, repo: string) =>
    api.get<{ enabled: boolean; model?: string; auto?: boolean }>(`/repos/${owner}/${repo}/ai-review/config`),
  aiReviews: (owner: string, repo: string, number: number) =>
    api.get<AIReview[]>(`/repos/${owner}/${repo}/pulls/${number}/ai-reviews`),
  requestAIReview: (owner: string, repo: string, number: number) =>
    api.post<AIReview>(`/repos/${owner}/${repo}/pulls/${number}/ai-reviews`),
}

export interface AIReview {
  id: string
  pr_id: string
  model: string
  body: string
  created_at: string
}

export const actionsApi = {
  workflows: (owner: string, repo: string) => api.get(`/repos/${owner}/${repo}/actions/workflows`),
  upsertWorkflow: (owner: string, repo: string, data: { name: string; path: string; content: string }) =>
    api.post(`/repos/${owner}/${repo}/actions/workflows`, data),
  runs: (owner: string, repo: string) => api.get(`/repos/${owner}/${repo}/actions/runs`),
  trigger: (owner: string, repo: string, data: { workflow_id: string; event: string; branch?: string }) =>
    api.post(`/repos/${owner}/${repo}/actions/runs`, data),
  logs: (owner: string, repo: string, runId: string) =>
    api.get(`/repos/${owner}/${repo}/actions/runs/${runId}/logs`),
}

export const releaseApi = {
  list: (owner: string, repo: string) => api.get(`/repos/${owner}/${repo}/releases`),
  create: (owner: string, repo: string, data: { tag_name: string; name: string; body: string }) =>
    api.post(`/repos/${owner}/${repo}/releases`, data),
}

export const packageApi = {
  list: (owner: string, repo: string) => api.get(`/repos/${owner}/${repo}/packages`),
}

export const searchApi = {
  search: (q: string) => api.get('/search', { params: { q } }),
}

export const webhookApi = {
  list: (owner: string, repo: string) => api.get(`/repos/${owner}/${repo}/webhooks`),
  create: (owner: string, repo: string, data: { url: string; secret: string; events: string[] }) =>
    api.post(`/repos/${owner}/${repo}/webhooks`, data),
}

export default api
