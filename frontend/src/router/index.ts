import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('../views/Login.vue'), meta: { guest: true } },
    { path: '/admin/users', component: () => import('../views/AdminUsers.vue'), meta: { admin: true } },
    { path: '/', component: () => import('../views/Dashboard.vue') },
    { path: '/projects', redirect: '/' },
    { path: '/search', component: () => import('../views/Search.vue') },
    { path: '/settings', component: () => import('../views/UserSettings.vue') },
    { path: '/:owner/:repo', component: () => import('../views/Repo.vue') },
    { path: '/:owner/:repo/issues', component: () => import('../views/Issues.vue') },
    { path: '/:owner/:repo/issues/:number', component: () => import('../views/IssueDetail.vue') },
    { path: '/:owner/:repo/pulls', component: () => import('../views/PullRequests.vue') },
    { path: '/:owner/:repo/pulls/:number', component: () => import('../views/PRDetail.vue') },
    { path: '/:owner/:repo/actions', component: () => import('../views/Actions.vue') },
    { path: '/:owner/:repo/releases', component: () => import('../views/Releases.vue') },
    { path: '/:owner/:repo/packages', component: () => import('../views/Packages.vue') },
    { path: '/:owner/:repo/settings', component: () => import('../views/Settings.vue') },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.user && auth.token) {
    try {
      await auth.fetchUser()
    } catch {
      return '/login'
    }
  }
  if (!auth.isLoggedIn && !to.meta.guest) return '/login'
  if (auth.isLoggedIn && to.meta.guest) return '/'
  if (to.meta.admin && !auth.isAdmin) return '/'
})

export default router
