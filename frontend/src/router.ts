import { createRouter, createWebHistory } from 'vue-router'
import { getAccessToken, useAuth } from './auth'
import LoginView from './views/LoginView.vue'
import HomeView from './views/HomeView.vue'
import DashboardView from './views/DashboardView.vue'
import BlindEvalView from './views/BlindEvalView.vue'
import RankingView from './views/RankingView.vue'
import EndpointArenaView from './views/EndpointArenaView.vue'
import AdminView from './views/AdminView.vue'
import UserCenterView from './views/UserCenterView.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: HomeView },
    { path: '/login', name: 'login', component: LoginView },
    { path: '/dashboard', name: 'dashboard', component: DashboardView, meta: { requiresAuth: true } },
    { path: '/eval', name: 'eval', component: BlindEvalView, meta: { requiresAuth: true } },
    { path: '/rankings', name: 'rankings', component: RankingView, meta: { requiresAuth: true } },
    { path: '/arena', name: 'arena', component: EndpointArenaView, meta: { requiresAuth: true } },
    { path: '/user', name: 'user', component: UserCenterView, meta: { requiresAuth: true } },
    { path: '/admin', name: 'admin', component: AdminView, meta: { requiresAuth: true, requiresAdmin: true } },
  ],
})

router.beforeEach((to) => {
  if (to.meta.requiresAuth && !getAccessToken()) return { name: 'login' }
  if (to.meta.requiresAdmin && !useAuth().isAdmin.value) return { name: 'dashboard' }
  if (to.name === 'login' && getAccessToken()) return { name: 'dashboard' }
  return true
})
