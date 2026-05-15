<script lang="ts">
  import { api } from '../api'

  let username = $state('')
  let password = $state('')
  let error = $state('')
  let loading = $state(false)

  async function submit(e: Event) {
    e.preventDefault()
    loading = true
    error = ''
    try {
      await api.login(username, password)
      window.location.href = '/'
    } catch (err: any) {
      error = err.message
    } finally {
      loading = false
    }
  }
</script>

<div class="login-wrap">
  <div class="login-card">
    <h1>mattube</h1>
    <form onsubmit={submit}>
      <div class="field">
        <label for="username">Username</label>
        <input id="username" type="text" bind:value={username} autocomplete="username" required />
      </div>
      <div class="field">
        <label for="password">Password</label>
        <input id="password" type="password" bind:value={password} autocomplete="current-password" required />
      </div>
      {#if error}
        <div class="error">{error}</div>
      {/if}
      <button type="submit" disabled={loading} class="submit-btn">
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
  min-height: 70vh;
}
.login-card {
  background: #17212b;
  padding: 32px;
  border-radius: 14px;
  width: 100%;
  max-width: 340px;
  border: 1px solid #2b3a4a;
}
h1 { text-align: center; margin-bottom: 24px; color: #6ab2f2; }
.field { display: flex; flex-direction: column; gap: 6px; margin-bottom: 14px; }
label { font-size: 0.82em; color: #aab8c2; }
input {
  padding: 9px 12px;
  background: #0e1621;
  border: 1.5px solid #2b3a4a;
  border-radius: 8px;
  color: #e8e8e8;
  font-size: 0.9em;
  outline: none;
}
input:focus { border-color: #6ab2f2; }
.error { color: #e74c3c; font-size: 0.82em; margin-bottom: 10px; }
.submit-btn {
  width: 100%;
  padding: 10px;
  background: #2b8fcc;
  border: none;
  border-radius: 8px;
  color: #fff;
  font-size: 0.95em;
  cursor: pointer;
  margin-top: 4px;
}
.submit-btn:disabled { opacity: 0.6; cursor: default; }
</style>
