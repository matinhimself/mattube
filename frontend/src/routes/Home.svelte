<script lang="ts">
  import { onMount } from 'svelte'
  import { useLocation } from 'svelte-routing'
  import { api, type SearchResult } from '../api'
  import VideoCard from '../lib/VideoCard.svelte'

  const location = useLocation()

  let query = $state(new URLSearchParams(window.location.search).get('q') || '')
  let results = $state<SearchResult[]>([])
  let loading = $state(false)
  let error = $state('')

  onMount(() => {
    return location.subscribe(loc => {
      query = new URLSearchParams(loc?.search || '').get('q') || ''
    })
  })

  $effect(() => {
    if (query) {
      loading = true
      error = ''
      results = []
      api.search(query)
        .then(r => { results = r; loading = false })
        .catch(e => { error = e.message; loading = false })
    }
  })
</script>

{#if !query}
  <div class="hero">
    <h1 class="hero-title">Find & Download <span class="hero-accent">Anything</span></h1>
    <p class="hero-sub">Search YouTube, download at any quality, stream instantly.</p>
  </div>
{:else if loading}
  <div class="state-msg">
    <div class="spinner"></div>
    <p>Searching for "{query}"…</p>
  </div>
{:else if error}
  <div class="state-msg error">{error}</div>
{:else if results.length === 0}
  <div class="state-msg">No results for "{query}"</div>
{:else}
  <div class="results-grid">
    {#each results as r}
      <VideoCard result={r} />
    {/each}
  </div>
{/if}

<style>
.hero {
  text-align: center;
  padding: 80px 20px 60px;
}
.hero-title {
  font-size: clamp(2rem, 5vw, 3.2rem);
  font-weight: 700;
  letter-spacing: -1px;
  margin-bottom: 16px;
  color: var(--text-primary);
  line-height: 1.15;
}
.hero-accent {
  color: var(--accent);
  text-shadow: 0 0 40px rgba(224,48,48,0.45);
}
.hero-sub {
  color: var(--text-secondary);
  font-size: 1rem;
  max-width: 420px;
  margin: 0 auto;
}
.results-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 16px;
}
</style>
