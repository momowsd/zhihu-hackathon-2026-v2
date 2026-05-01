import { computed, ref } from 'vue'

const ACCESS_TOKEN = 'llm_arena_access_token'
const REFRESH_TOKEN = 'llm_arena_refresh_token'
const authVersion = ref(0)

type Claims = {
  username?: string
  role?: string
  exp?: number
}

function decode(token: string): Claims {
  try {
    const payload = token.split('.')[1]
    return JSON.parse(atob(payload)) as Claims
  } catch {
    return {}
  }
}

export function saveTokens(accessToken: string, refreshToken: string) {
  localStorage.setItem(ACCESS_TOKEN, accessToken)
  localStorage.setItem(REFRESH_TOKEN, refreshToken)
  authVersion.value += 1
}

export function clearTokens() {
  localStorage.removeItem(ACCESS_TOKEN)
  localStorage.removeItem(REFRESH_TOKEN)
  authVersion.value += 1
}

export function getAccessToken() {
  return localStorage.getItem(ACCESS_TOKEN) ?? ''
}

export function getRefreshToken() {
  return localStorage.getItem(REFRESH_TOKEN) ?? ''
}

export function useAuth() {
  return {
    isAuthed: computed(() => {
      authVersion.value
      return Boolean(getAccessToken())
    }),
    username: computed(() => {
      authVersion.value
      return decode(getAccessToken()).username ?? ''
    }),
    role: computed(() => {
      authVersion.value
      return decode(getAccessToken()).role ?? ''
    }),
    isAdmin: computed(() => {
      authVersion.value
      return decode(getAccessToken()).role === 'admin'
    }),
  }
}
