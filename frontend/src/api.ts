import axios from 'axios'
import { clearTokens, getAccessToken, getRefreshToken, saveTokens } from './auth'

export type ApiEnvelope<T> = { data: T }
export type Category = {
  id: string
  code: string
  name: string
  description: string
  enabled: boolean
  sortOrder: number
  domainSlug?: string
  systemPromptMd?: string
}
export type Question = { id: string; categoryId: string; prompt: string; source: string; difficulty: string; enabled: boolean }
export type Model = { id: string; provider: string; name: string; displayName: string; version: string; isBaseline: boolean; enabled: boolean }
export type ModelAnswer = { id: string; questionId: string; modelId: string; answerText: string; metadataJson: string }
export type PeerMatrixModel = { modelId: string; displayName: string; provider: string }
export type PeerMatrixCell = {
  judgeModelId: string
  targetModelId: string
  score: number
  samples: number
  positive: number
  negative: number
  bothGood: number
  bothBad: number
}
export type PeerMatrixResponse = {
  models: PeerMatrixModel[]
  cells: PeerMatrixCell[]
  sampleCount: number
}
export type UserHistoryItem = {
  sessionId: string
  sessionMode: string
  sessionCreatedAt: string
  categoryName: string
  itemId: string
  position: number
  questionId: string
  questionPrompt: string
  outcome: 'left' | 'right' | 'both_good' | 'both_bad' | ''
  outcomeLabel: string
  votedAt: string
  leftModelId: string
  leftModelName: string
  leftAnswerId: string
  leftAnswerText: string
  rightModelId: string
  rightModelName: string
  rightAnswerId: string
  rightAnswerText: string
  winnerModelIds: string[]
  winnerModels: string[]
}
export type UserModelFitRow = {
  scope: string
  modelId: string
  modelName: string
  appearances: number
  positive: number
  rate: number
  rank: number
}
export type UserHistoryResponse = {
  items: UserHistoryItem[]
  total: number
  page: number
  pageSize: number
  fitScopes: string[]
  topModels: UserModelFitRow[]
}

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

/** Admin：分页浏览数据库表 */
export type PaginatedTableBrowse = {
  items: Record<string, unknown>[]
  total: number
  page: number
  pageSize: number
}

export async function adminBrowseTable(table: string, page: number, pageSize: number) {
  const response = await api.get<ApiEnvelope<PaginatedTableBrowse>>(`/admin/browse/${encodeURIComponent(table)}`, {
    params: { page, pageSize },
  })
  return response.data.data
}

export async function loadPeerMatrix(categoryId?: string) {
  const response = await api.get<ApiEnvelope<PeerMatrixResponse>>('/rankings/peer-matrix', {
    params: { categoryId: categoryId || undefined },
  })
  return response.data.data
}

export type QuestionDeleteImpact = {
  votes: number
  evalItems: number
  modelAnswers: number
  questions: number
}

export async function adminQuestionDeleteImpact(id: string) {
  const response = await api.get<ApiEnvelope<QuestionDeleteImpact>>(
    `/admin/questions/${encodeURIComponent(id)}/delete-impact`,
  )
  return response.data.data
}

export async function adminQuestionDeleteImpactBatch(ids: string[]) {
  const response = await api.post<ApiEnvelope<QuestionDeleteImpact>>(`/admin/questions/delete-impact`, { ids })
  return response.data.data
}

export async function adminDeleteQuestion(id: string) {
  await api.delete(`/admin/questions/${encodeURIComponent(id)}`)
}

export async function adminDeleteQuestionsBatch(ids: string[]) {
  await api.delete(`/admin/questions`, { data: { ids } })
}

export type ImportFieldError = {
  row: string
  field: string
  message: string
}

export async function adminImportBundle(bundle: unknown) {
  await api.post(`/admin/import/bundle`, bundle)
}

export function importBundleErrorDetail(error: unknown): { message: string; errors?: ImportFieldError[] } {
  if (axios.isAxiosError(error)) {
    const d = error.response?.data as { message?: string; errors?: ImportFieldError[] } | undefined
    return {
      message: d?.message ?? error.message,
      errors: d?.errors,
    }
  }
  return { message: error instanceof Error ? error.message : '请求失败' }
}

export async function login(username: string, password: string) {
  const response = await api.post<ApiEnvelope<TokenPair>>('/auth/login', { username, password })
  saveTokens(response.data.data.accessToken, response.data.data.refreshToken)
}

export async function register(username: string, password: string) {
  const response = await api.post<ApiEnvelope<TokenPair>>('/auth/register', { username, password })
  saveTokens(response.data.data.accessToken, response.data.data.refreshToken)
}

export async function startZhihuOAuth() {
  const response = await api.get<ApiEnvelope<{ authorizeUrl: string }>>('/auth/zhihu/start')
  return response.data.data.authorizeUrl
}

export async function exchangeZhihuOAuthTicket(ticket: string) {
  const response = await api.post<ApiEnvelope<TokenPair>>('/auth/zhihu/exchange', { ticket })
  saveTokens(response.data.data.accessToken, response.data.data.refreshToken)
}

function normalizeCategory(category: Category): Category {
  if (category.domainSlug === 'ruozhi-eval' || category.code === 'ruozhi-eval' || category.name === '弱智评估') {
    return { ...category, name: '弱智吧Case评估' }
  }
  return category
}

export async function loadCategories() {
  const response = await api.get<ApiEnvelope<Category[]>>('/eval/categories')
  return response.data.data.map(normalizeCategory)
}

export async function loadUserHistory(page = 1, pageSize = 10) {
  const response = await api.get<ApiEnvelope<UserHistoryResponse>>('/user/history', {
    params: { page, pageSize },
  })
  return response.data.data
}

export function errorMessage(error: unknown): string {
  if (axios.isAxiosError(error)) {
    return (error.response?.data as { message?: string } | undefined)?.message ?? error.message
  }
  return error instanceof Error ? error.message : '请求失败'
}
