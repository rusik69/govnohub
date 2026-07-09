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
