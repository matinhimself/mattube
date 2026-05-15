export interface User {
  id: number
  username: string
  is_admin: boolean
  last_login: string | null
  local_mode?: boolean
}

class AuthStore {
  user = $state<User | null>(null)
  loading = $state(true)

  setUser(u: User | null) { this.user = u }
  setLoading(v: boolean) { this.loading = v }

  get isLoggedIn() { return this.user !== null }
  get isAdmin() { return this.user?.is_admin === true }
  get isLocalMode() { return this.user?.local_mode === true }
}

export const auth = new AuthStore()
