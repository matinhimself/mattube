<script lang="ts">
  import { Router, Route, Link } from 'svelte-routing'
  import VideoPlayer from './lib/VideoPlayer.svelte'
  import Home from './routes/Home.svelte'
  import Channel from './routes/Channel.svelte'
  import Video from './routes/Video.svelte'
  import Downloads from './routes/Downloads.svelte'
  import Login from './routes/Login.svelte'
  import { player } from './lib/player.svelte'

  let searchQuery = $state('')

  function handleSearch(e: Event) {
    e.preventDefault()
    if (searchQuery.trim()) {
      window.location.href = `/?q=${encodeURIComponent(searchQuery.trim())}`
    }
  }
</script>

<svelte:head>
  <link rel="stylesheet" href="https://lib.arvancloud.ir/video.js/8.1.0/video-js.min.css">
  <script src="https://lib.arvancloud.ir/video.js/8.1.0/video.min.js" crossorigin="anonymous"></script>
</svelte:head>

<Router>
  <div class="app" class:has-mini-player={player.minimized}>

    <header class="top-bar">
      <Link to="/" class="logo">mattube</Link>
      <form class="search-form" onsubmit={handleSearch}>
        <input
          type="search"
          placeholder="Search YouTube..."
          bind:value={searchQuery}
          class="search-input"
        />
      </form>
      <nav class="nav-links">
        <Link to="/downloads">↓ Downloads</Link>
      </nav>
    </header>

    <main class="content">
      <Route path="/" component={Home} />
      <Route path="/channel/:channelId" component={Channel} />
      <Route path="/video/:videoId" component={Video} />
      <Route path="/downloads" component={Downloads} />
      <Route path="/login" component={Login} />
    </main>

    <!-- Player always mounted — persists across navigation -->
    <VideoPlayer />

  </div>
</Router>

<style>
:global(*) { box-sizing: border-box; margin: 0; padding: 0; }
:global(body) {
  background: #0e1621;
  color: #e8e8e8;
  font-family: system-ui, -apple-system, sans-serif;
  font-size: 14px;
}
:global(a) { color: #6ab2f2; text-decoration: none; }

.app { min-height: 100vh; }
.app.has-mini-player { padding-bottom: 64px; }

.top-bar {
  position: sticky;
  top: 0;
  z-index: 50;
  background: #17212b;
  border-bottom: 1px solid #2b3a4a;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
}
:global(.logo) {
  font-weight: 700;
  font-size: 1.1em;
  color: #e8e8e8 !important;
  text-decoration: none;
  white-space: nowrap;
}
.search-form { flex: 1; }
.search-input {
  width: 100%;
  max-width: 480px;
  padding: 7px 12px;
  background: #0e1621;
  border: 1.5px solid #2b3a4a;
  border-radius: 20px;
  color: #e8e8e8;
  font-size: 0.9em;
  outline: none;
}
.search-input:focus { border-color: #6ab2f2; }
.nav-links { font-size: 0.85em; white-space: nowrap; }
.content { padding: 16px; max-width: 960px; margin: 0 auto; }
</style>
