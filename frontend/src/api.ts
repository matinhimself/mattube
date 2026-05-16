export interface User {
  id: number
  username: string
  is_admin: boolean
  last_login: string | null
}

export interface VideoInfo {
  video_id: string
  title: string
  channel_id: string
  channel_name: string
  duration: number
  view_count: string
  description: string
  thumbnail: string
}

export interface SearchResult {
  video_id: string
  title: string
  channel_id: string
  channel_name: string
  duration: string
  view_count: string
  thumbnail: string
}

export interface ChannelInfo {
  channel_id: string
  name: string
  description: string
  avatar: string
  subscribers: string
}

export interface ChunkRef {
  index: number
  drive_file_id: string
  duration_s: number
}

export interface JobStatus {
  job_id: string
  status: 'pending' | 'queued' | 'downloading' | 'uploading' | 'chunking' | 'done' | 'failed'
  progress: number
  drive_file_id?: string
  error?: string
  updated_at: string
  total_chunks?: number
  chunk_duration_s?: number
  chunks?: ChunkRef[]
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const resp = await fetch(path, { credentials: 'include', ...options })
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: resp.statusText }))
    throw new Error(err.error || resp.statusText)
  }
  return resp.json()
}

export const api = {
  login: (username: string, password: string): Promise<User> =>
    request('/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    }),

  logout: () => request('/auth/logout', { method: 'POST' }),

  getCurrentUser: (): Promise<User> => request('/auth/me'),

  search: (q: string, n = 20): Promise<SearchResult[]> =>
    request(`/api/search?q=${encodeURIComponent(q)}&n=${n}`),

  getVideo: (videoId: string): Promise<VideoInfo> =>
    request(`/api/video/${videoId}`),

  getRelatedVideos: (videoId: string): Promise<SearchResult[]> =>
    request(`/api/video/${videoId}/related`),

  getChannel: (channelId: string): Promise<ChannelInfo> =>
    request(`/api/channel/${channelId}`),

  getChannelVideos: (channelId: string): Promise<SearchResult[]> =>
    request(`/api/channel/${channelId}/videos`),

  submitJob: (url: string, quality = '1080p', chunkDurationS = 0): Promise<{ job_id: string }> =>
    request('/api/jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url, quality, chunk_duration_s: chunkDurationS }),
    }),

  getJobStatus: (jobId: string): Promise<JobStatus> =>
    request(`/api/jobs/${jobId}/status`),

  listJobs: (): Promise<JobStatus[]> =>
    request('/api/jobs'),

  streamUrl: (jobId: string) => `/api/jobs/${jobId}/stream`,

  listUsers: (): Promise<User[]> =>
    request('/admin/users'),

  createUser: (username: string, password: string, is_admin = false): Promise<User> =>
    request('/admin/users', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password, is_admin }),
    }),

  resetPassword: (userId: number, password: string): Promise<void> =>
    request(`/admin/users/${userId}/password`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password }),
    }),

  deleteUser: (userId: number): Promise<void> =>
    request(`/admin/users/${userId}`, { method: 'DELETE' }),

  driveStatus: (): Promise<{ connected: boolean; creds_ready: boolean }> =>
    request('/admin/drive/status'),

  driveConnectUrl: () => '/admin/drive/connect',
}
