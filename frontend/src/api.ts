import axios from 'axios'
import { clearTokens, getAccessToken, getRefreshToken, saveTokens } from './auth'

export type ApiEnvelope<T> = { data: T }
export type Category = { id: string; code: string; name: string; description: string; enabled: boolean; sortOrder: number }
export type Question = { id: string; categoryId: string; prompt: string; source: string; difficulty: string; enabled: boolean }
export type Model = { id: string; provider: string; name: string; displayName: string; version: string; isBaseline: boolean; enabled: boolean }
export type ModelAnswer = { id: string; questionId: string; modelId: string; answerText: string; metadataJson: string }

export const api = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
})

api.interceptors.request.use((config) => {
  const token = getAccessToken()
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const original = error.config
    if (error?.response?.status !== 401 || original?._retry) {
      return Promise.reject(error)
    }
    const refreshToken = getRefreshToken()
    if (!refreshToken) {
      clearTokens()
      return Promise.reject(error)
    }
    try {
      original._retry = true
      const response = await axios.post<ApiEnvelope<TokenPair>>('/api/v1/auth/refresh', { refreshToken })
      saveTokens(response.data.data.accessToken, response.data.data.refreshToken)
      original.headers.Authorization = `Bearer ${response.data.data.accessToken}`
      return api(original)
    } catch (refreshError) {
      clearTokens()
      return Promise.reject(refreshError)
    }
  },
)

export type TokenPair = {
  accessToken: string
  refreshToken: string
  expiresIn: number
}

export async function login(username: string, password: string) {
  const response = await api.post<ApiEnvelope<TokenPair>>('/auth/login', { username, password })
  saveTokens(response.data.data.accessToken, response.data.data.refreshToken)
}

export async function register(username: string, password: string) {
  const response = await api.post<ApiEnvelope<TokenPair>>('/auth/register', { username, password })
  saveTokens(response.data.data.accessToken, response.data.data.refreshToken)
}

export async function loadCategories() {
  const response = await api.get<ApiEnvelope<Category[]>>('/eval/categories')
  return response.data.data
}

export function errorMessage(error: unknown): string {
  if (axios.isAxiosError(error)) {
    return (error.response?.data as { message?: string } | undefined)?.message ?? error.message
  }
  return error instanceof Error ? error.message : '请求失败'
}
