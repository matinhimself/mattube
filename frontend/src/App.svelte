<script lang="ts">
  import { onMount } from 'svelte'
  import { Router, Route, Link, navigate } from 'svelte-routing'
  import VideoPlayer from './lib/VideoPlayer.svelte'
  import Guard from './lib/Guard.svelte'
  import Home from './routes/Home.svelte'
  import Channel from './routes/Channel.svelte'
  import Video from './routes/Video.svelte'
  import Downloads from './routes/Downloads.svelte'
  import Login from './routes/Login.svelte'
  import Admin from './routes/Admin.svelte'
  import { player } from './lib/player.svelte'
  import { auth } from './lib/auth.svelte'
  import { api } from './api'

  let searchQuery = $state('')

  onMount(async () => {
    try {
      const u = await api.getCurrentUser()
      auth.setUser(u)
    } catch {
      auth.setUser(null)
    } finally {
      auth.setLoading(false)
    }
  })

  function handleSearch(e: Event) {
    e.preventDefault()
    const q = searchQuery.trim()
    if (q) navigate(`/?q=${encodeURIComponent(q)}`)
  }

  async function logout() {
    await api.logout().catch(() => {})
    auth.setUser(null)
    navigate('/login')
  }
</script>

<svelte:head>
  <link rel="stylesheet" href="https://lib.arvancloud.ir/video.js/8.1.0/video-js.min.css">
  <script src="https://lib.arvancloud.ir/video.js/8.1.0/video.min.js" crossorigin="anonymous"></script>
</svelte:head>

<Router>
  <div class="app" class:has-mini-player={player.minimized}>

    <header class="navbar">
      <Link to="/" class="navbar-logo">
        matt<span class="logo-accent">ube</span>
      </Link>

      <form class="navbar-search" onsubmit={handleSearch}>
        <input
          type="search"
          class="search-input"
          placeholder="Search YouTube..."
          bind:value={searchQuery}
        />
        <button type="submit" class="search-btn" aria-label="Search">⌕</button>
      </form>

      <nav class="navbar-actions">
        <Link to="/downloads" class="nav-link">Downloads</Link>
        {#if auth.isAdmin}
          <Link to="/admin/users" class="nav-link nav-link-admin">Admin</Link>
        {/if}
        {#if auth.isLoggedIn}
          <button class="btn-ghost navbar-signout" onclick={logout}>Sign out</button>
        {:else if !auth.loading}
          <Link to="/login" class="btn-accent nav-signin">Sign in</Link>
        {/if}
      </nav>
    </header>

    <main class="content">
      <Route path="/login" component={Login} />

      <Route path="/" let:params>
        <Guard><Home /></Guard>
      </Route>

      <Route path="/channel/:channelId" let:params>
        <Guard><Channel channelId={params.channelId} /></Guard>
      </Route>

      <Route path="/video/:videoId" let:params>
        <Guard><Video videoId={params.videoId} /></Guard>
      </Route>

      <Route path="/downloads">
        <Guard><Downloads /></Guard>
      </Route>

      <Route path="/admin/users">
        <Guard adminOnly><Admin /></Guard>
      </Route>
    </main>

    <VideoPlayer />

  </div>
</Router>

<style>
.app { min-height: 100vh; }
.app.has-mini-player { padding-bottom: 68px; }

.navbar {
  position: sticky;
  top: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 20px;
  height: 58px;
  background: rgba(8, 8, 8, 0.80);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-bottom: 1px solid var(--glass-border);
}

:global(.navbar-logo) {
  font-size: 1.25rem;
  font-weight: 700;
  color: var(--text-primary) !important;
  text-decoration: none;
  flex-shrink: 0;
  letter-spacing: -0.5px;
}
.logo-accent { color: var(--accent); }

.navbar-search {
  flex: 1;
  max-width: 480px;
  display: flex;
  align-items: center;
  position: relative;
}
.search-input {
  width: 100%;
  padding: 8px 38px 8px 16px;
  background: rgba(255,255,255,0.05);
  border: 1px solid var(--glass-border);
  border-radius: 100px;
  color: var(--text-primary);
  font-family: inherit;
  font-size: 0.875rem;
  outline: none;
  transition: border-color var(--t-fast), box-shadow var(--t-fast);
}
.search-input:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px rgba(224,48,48,0.1);
}
.search-input::placeholder { color: var(--text-muted); }
.search-btn {
  position: absolute;
  right: 10px;
  background: none;
  border: none;
  cursor: pointer;
  color: var(--text-secondary);
  font-size: 1.1rem;
  padding: 4px;
  transition: color var(--t-fast);
  line-height: 1;
}
.search-btn:hover { color: var(--accent); }

.navbar-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-left: auto;
  flex-shrink: 0;
}
:global(.nav-link) {
  color: var(--text-secondary);
  font-size: 0.875rem;
  text-decoration: none;
  transition: color var(--t-fast);
}
:global(.nav-link:hover) { color: var(--text-primary); }
:global(.nav-link-admin) { color: var(--accent) !important; }
:global(.nav-signin) { font-size: 0.82rem !important; padding: 7px 14px !important; }
.navbar-signout { font-size: 0.82rem; padding: 6px 12px; }

.content { padding: 24px 20px 40px; max-width: 1020px; margin: 0 auto; }
</style>
