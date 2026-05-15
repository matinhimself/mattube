<script lang="ts">
  import { api, type User } from '../api'
  import { auth } from '../lib/auth.svelte'

  let users = $state<User[]>([])
  let loading = $state(true)
  let error = $state('')

  let newUsername = $state('')
  let newPassword = $state('')
  let newIsAdmin = $state(false)
  let creating = $state(false)
  let createError = $state('')

  let resetTargetId = $state<number | null>(null)
  let resetPassword = $state('')
  let resetting = $state(false)

  let deleteTargetId = $state<number | null>(null)
  let deleting = $state(false)

  $effect(() => { loadUsers() })

  async function loadUsers() {
    loading = true; error = ''
    try { users = await api.listUsers() }
    catch (e: any) { error = e.message }
    finally { loading = false }
  }

  async function createUser(e: Event) {
    e.preventDefault()
    creating = true; createError = ''
    try {
      await api.createUser(newUsername, newPassword, newIsAdmin)
      newUsername = ''; newPassword = ''; newIsAdmin = false
      await loadUsers()
    } catch (e: any) { createError = e.message }
    finally { creating = false }
  }

  async function confirmDelete() {
    if (deleteTargetId === null) return
    deleting = true
    try {
      await api.deleteUser(deleteTargetId)
      deleteTargetId = null
      await loadUsers()
    } catch (e: any) { error = e.message }
    finally { deleting = false }
  }

  async function submitResetPassword(e: Event) {
    e.preventDefault()
    if (resetTargetId === null) return
    resetting = true
    try {
      await api.resetPassword(resetTargetId, resetPassword)
      resetTargetId = null; resetPassword = ''
    } catch (e: any) { error = e.message }
    finally { resetting = false }
  }
</script>

<div class="admin-page">
  <h2 class="page-title">User Management</h2>

  <!-- Create user -->
  <div class="glass admin-section">
    <h3 class="section-title">Create User</h3>
    <form class="create-form" onsubmit={createUser}>
      <input class="input-base" placeholder="Username" bind:value={newUsername} required />
      <input class="input-base" type="password" placeholder="Password" bind:value={newPassword} required />
      <label class="check-label">
        <input type="checkbox" bind:checked={newIsAdmin} />
        Admin
      </label>
      <button class="btn-accent" type="submit" disabled={creating}>
        {creating ? 'Creating…' : '+ Create'}
      </button>
    </form>
    {#if createError}<div class="form-error">{createError}</div>{/if}
  </div>

  <!-- Users table -->
  {#if loading}
    <div class="state-msg"><div class="spinner"></div></div>
  {:else if error}
    <div class="state-msg error">{error}</div>
  {:else}
    <div class="glass admin-section">
      <h3 class="section-title">All Users ({users.length})</h3>
      <div class="user-table">
        <div class="user-table-head">
          <span>Username</span><span>Role</span><span>Last Login</span><span>Actions</span>
        </div>
        {#each users as u}
          <div class="user-row" class:user-row-self={u.id === auth.user?.id}>
            <span class="user-name">
              {u.username}
              {#if u.id === auth.user?.id}
                <span class="self-tag">you</span>
              {/if}
            </span>
            <span>
              {#if u.is_admin}
                <span class="chip chip-done">Admin</span>
              {:else}
                <span class="chip chip-muted">User</span>
              {/if}
            </span>
            <span class="user-login">
              {u.last_login ? new Date(u.last_login).toLocaleDateString() : '—'}
            </span>
            <span class="user-actions">
              <button
                class="btn-ghost action-btn"
                onclick={() => { resetTargetId = u.id; resetPassword = '' }}
              >Reset PW</button>
              {#if u.id !== auth.user?.id}
                <button
                  class="btn-ghost action-btn action-btn-danger"
                  onclick={() => deleteTargetId = u.id}
                >Delete</button>
              {/if}
            </span>
          </div>
        {/each}
      </div>
    </div>
  {/if}
</div>

<!-- Reset password modal -->
{#if resetTargetId !== null}
  <div class="modal-backdrop" onclick={() => resetTargetId = null} onkeydown={(e) => e.key === 'Escape' && (resetTargetId = null)} role="presentation" tabindex="-1">
    <div class="modal glass" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
      <h3 class="modal-title">Reset Password</h3>
      <form onsubmit={submitResetPassword}>
        <input
          class="input-base modal-input"
          type="password"
          placeholder="New password"
          bind:value={resetPassword}
          required
        />
        <div class="modal-actions">
          <button type="button" class="btn-ghost" onclick={() => resetTargetId = null}>Cancel</button>
          <button class="btn-accent" type="submit" disabled={resetting}>
            {resetting ? 'Saving…' : 'Save'}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Delete confirm modal -->
{#if deleteTargetId !== null}
  <div class="modal-backdrop" onclick={() => deleteTargetId = null} onkeydown={(e) => e.key === 'Escape' && (deleteTargetId = null)} role="presentation" tabindex="-1">
    <div class="modal glass" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
      <h3 class="modal-title">Delete User</h3>
      <p class="modal-body">This action cannot be undone.</p>
      <div class="modal-actions">
        <button class="btn-ghost" onclick={() => deleteTargetId = null}>Cancel</button>
        <button class="btn-accent btn-danger" disabled={deleting} onclick={confirmDelete}>
          {deleting ? 'Deleting…' : 'Delete'}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
.admin-page { max-width: 860px; }
.page-title { font-size: 1.2rem; font-weight: 600; margin-bottom: 20px; }
.admin-section { padding: 20px 24px; margin-bottom: 16px; }
.section-title {
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--text-secondary);
  margin-bottom: 16px;
}

.create-form {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  align-items: flex-end;
}
.create-form :global(.input-base) { flex: 1; min-width: 140px; }
.check-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 0.85rem;
  color: var(--text-secondary);
  white-space: nowrap;
  cursor: pointer;
}
.check-label input[type="checkbox"] { accent-color: var(--accent); width: 15px; height: 15px; }
.form-error { font-size: 0.82rem; color: var(--accent-hover); margin-top: 8px; }

.user-table { display: flex; flex-direction: column; gap: 2px; }
.user-table-head {
  display: grid;
  grid-template-columns: 1fr 90px 130px 170px;
  padding: 6px 12px;
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-muted);
}
.user-row {
  display: grid;
  grid-template-columns: 1fr 90px 130px 170px;
  align-items: center;
  padding: 10px 12px;
  border-radius: var(--radius-sm);
  transition: background var(--t-fast);
}
.user-row:hover { background: var(--glass-bg); }
.user-row-self { background: rgba(224,48,48,0.04); }
.user-name {
  font-size: 0.875rem;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 8px;
}
.self-tag {
  font-size: 0.68rem;
  padding: 2px 6px;
  background: rgba(224,48,48,0.15);
  color: var(--accent);
  border-radius: 20px;
  font-weight: 600;
}
.chip-muted {
  background: rgba(255,255,255,0.05);
  color: var(--text-secondary);
  border: 1px solid var(--glass-border);
}
.user-login { font-size: 0.8rem; color: var(--text-secondary); }
.user-actions { display: flex; gap: 6px; }
.action-btn { font-size: 0.78rem; padding: 5px 10px; }
.action-btn-danger {
  border-color: rgba(224,48,48,0.3) !important;
  color: var(--accent-hover) !important;
}
.action-btn-danger:hover {
  background: rgba(224,48,48,0.1) !important;
  border-color: var(--accent) !important;
}

/* Modals */
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  backdrop-filter: blur(4px);
  z-index: 200;
  display: flex;
  align-items: center;
  justify-content: center;
}
.modal {
  width: 100%;
  max-width: 400px;
  padding: 28px 32px;
  box-shadow: 0 0 60px rgba(0,0,0,0.6), 0 0 0 1px rgba(224,48,48,0.12);
}
.modal-title { font-size: 1rem; font-weight: 600; margin-bottom: 16px; }
.modal-body { color: var(--text-secondary); font-size: 0.88rem; margin-bottom: 20px; }
.modal-input { margin-bottom: 16px; }
.modal-actions { display: flex; gap: 10px; justify-content: flex-end; }
.btn-danger { background: var(--accent) !important; }
.btn-danger:hover { background: var(--accent-hover) !important; }
</style>
