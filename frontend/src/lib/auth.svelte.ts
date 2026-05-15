export interface User {
  id: number
  username: string
  is_admin: boolean
  last_login: string | null
}

class AuthStore {
  user = $state<User | null>(null)
  loading = $state(true)

  setUser(u: User | null) { this.user = u }
  setLoading(v: boolean) { this.loading = v }

  get isLoggedIn() { return this.user !== null }
  get isAdmin() { return this.user?.is_admin === true }
}

export const auth = new AuthStore()
