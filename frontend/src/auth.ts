import { computed, ref } from 'vue'

const ACCESS_TOKEN = 'llm_arena_access_token'
const REFRESH_TOKEN = 'llm_arena_refresh_token'
const authVersion = ref(0)

type Claims = {
  username?: string
  displayName?: string
  avatarUrl?: string
  role?: string
  exp?: number
}

/** JWT payload 为 Base64URL 编码的 UTF-8 JSON；勿用 atob 直接 JSON.parse，否则会破坏中文等多字节字符。 */
function jwtPayloadJsonUtf8(segment: string): string {
  const base64 = segment.replace(/-/g, '+').replace(/_/g, '/')
  const pad = (4 - (base64.length % 4)) % 4
  const padded = base64 + '='.repeat(pad)
  const binary = atob(padded)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  return new TextDecoder('utf-8').decode(bytes)
}

function decode(token: string): Claims {
  try {
    const payload = token.split('.')[1]
    if (!payload) return {}
    return JSON.parse(jwtPayloadJsonUtf8(payload)) as Claims
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
    displayName: computed(() => {
      authVersion.value
      return (decode(getAccessToken()).displayName ?? '').trim()
    }),
    /** 顶栏/个人中心展示名：知乎昵称优先，否则站内用户名 */
    displayLabel: computed(() => {
      authVersion.value
      const c = decode(getAccessToken())
      const dn = (c.displayName ?? '').trim()
      return dn || (c.username ?? '')
    }),
    avatarUrl: computed(() => {
      authVersion.value
      return (decode(getAccessToken()).avatarUrl ?? '').trim()
    }),
    /** 无头像时用于圆形占位首字 */
    avatarLetter: computed(() => {
      authVersion.value
      const label =
        (decode(getAccessToken()).displayName ?? '').trim() ||
        (decode(getAccessToken()).username ?? '')
      const ch = Array.from(label)[0]
      return ch ? ch.toLocaleUpperCase('zh-CN') : 'U'
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
