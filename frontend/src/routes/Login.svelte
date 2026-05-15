<script lang="ts">
  import { navigate } from 'svelte-routing'
  import { api } from '../api'
  import { auth } from '../lib/auth.svelte'

  let username = $state('')
  let password = $state('')
  let error = $state('')
  let loading = $state(false)

  async function submit(e: Event) {
    e.preventDefault()
    loading = true
    error = ''
    try {
      const user = await api.login(username, password)
      auth.setUser(user)
      navigate('/')
    } catch (err: any) {
      error = err.message
    } finally {
      loading = false
    }
  }
</script>

<div class="login-wrap">
  <div class="login-card glass">
    <div class="login-logo">matt<span class="logo-accent">ube</span></div>
    <p class="login-sub">Sign in to your account</p>

    <form onsubmit={submit}>
      <div class="field">
        <label for="username" class="field-label">Username</label>
        <input
          id="username"
          type="text"
          class="input-base"
          bind:value={username}
          autocomplete="username"
          required
        />
      </div>
      <div class="field">
        <label for="password" class="field-label">Password</label>
        <input
          id="password"
          type="password"
          class="input-base"
          bind:value={password}
          autocomplete="current-password"
          required
        />
      </div>
      {#if error}
        <div class="login-error">{error}</div>
      {/if}
      <button type="submit" disabled={loading} class="btn-accent submit-btn">
        {loading ? 'Signing in...' : 'Sign in'}
      </button>
    </form>
  </div>
</div>

<style>
.login-wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: calc(100vh - 58px);
  padding: 40px 16px;
}
.login-card {
  width: 100%;
  max-width: 360px;
  padding: 36px 32px;
  box-shadow: 0 0 80px rgba(224,48,48,0.10), 0 0 1px rgba(224,48,48,0.2);
}
.login-logo {
  text-align: center;
  font-size: 1.8rem;
  font-weight: 700;
  letter-spacing: -0.5px;
  color: var(--text-primary);
  margin-bottom: 6px;
}
.logo-accent { color: var(--accent); }
.login-sub {
  text-align: center;
  color: var(--text-secondary);
  font-size: 0.85rem;
  margin-bottom: 28px;
}
.field { display: flex; flex-direction: column; gap: 7px; margin-bottom: 16px; }
.field-label { font-size: 0.8rem; font-weight: 500; color: var(--text-secondary); }
.login-error {
  font-size: 0.82rem;
  color: var(--accent-hover);
  background: rgba(224,48,48,0.1);
  border: 1px solid rgba(224,48,48,0.25);
  border-radius: var(--radius-sm);
  padding: 8px 12px;
  margin-bottom: 12px;
}
.submit-btn {
  width: 100%;
  justify-content: center;
  margin-top: 4px;
}
</style>
