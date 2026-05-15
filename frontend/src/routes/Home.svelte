<script lang="ts">
  import { api, type SearchResult } from '../api'
  import VideoCard from '../lib/VideoCard.svelte'

  // Read query from URL
  const params = new URLSearchParams(window.location.search)
  let query = $state(params.get('q') || '')
  let results = $state<SearchResult[]>([])
  let loading = $state(false)
  let error = $state('')

  $effect(() => {
    if (query) {
      loading = true
      error = ''
      api.search(query)
        .then(r => { results = r; loading = false })
        .catch(e => { error = e.message; loading = false })
    }
  })
</script>

{#if !query}
  <div class="hero">
    <h1>Search YouTube</h1>
    <p>Enter a query in the search bar above to discover videos and channels.</p>
  </div>
{:else if loading}
  <div class="loading">Searching...</div>
{:else if error}
  <div class="error">{error}</div>
{:else if results.length === 0}
  <div class="empty">No results for "{query}"</div>
{:else}
  <div class="results-grid">
    {#each results as r}
      <VideoCard result={r} />
    {/each}
  </div>
{/if}

<style>
.hero { text-align: center; padding: 60px 0; color: #aab8c2; }
.hero h1 { font-size: 1.8em; color: #e8e8e8; margin-bottom: 8px; }
.loading, .error, .empty { padding: 40px; text-align: center; color: #aab8c2; }
.error { color: #e74c3c; }
.results-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 16px;
}
</style>
