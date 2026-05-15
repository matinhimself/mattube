<script lang="ts">
  import { api, type ChannelInfo, type SearchResult } from '../api'
  import VideoCard from '../lib/VideoCard.svelte'

  let { channelId }: { channelId: string } = $props()

  let info = $state<ChannelInfo | null>(null)
  let videos = $state<SearchResult[]>([])
  let loading = $state(true)
  let error = $state('')

  $effect(() => {
    loading = true
    Promise.all([api.getChannel(channelId), api.getChannelVideos(channelId)])
      .then(([ch, vids]) => { info = ch; videos = vids; loading = false })
      .catch(e => { error = e.message; loading = false })
  })
</script>

{#if loading}
  <div class="loading">Loading channel...</div>
{:else if error}
  <div class="error">{error}</div>
{:else if info}
  <div class="channel-header">
    {#if info.avatar}
      <img src={info.avatar} alt={info.name} class="avatar" />
    {/if}
    <div class="channel-meta">
      <h1>{info.name}</h1>
      {#if info.subscribers}
        <div class="subs">{info.subscribers} subscribers</div>
      {/if}
      {#if info.description}
        <div class="desc">{info.description}</div>
      {/if}
    </div>
  </div>

  {#if videos.length > 0}
    <h2 class="section-title">Videos</h2>
    <div class="videos-grid">
      {#each videos as v}
        <VideoCard result={v} />
      {/each}
    </div>
  {:else}
    <div class="empty">No videos found</div>
  {/if}
{/if}

<style>
.loading, .error, .empty { padding: 40px; text-align: center; color: #aab8c2; }
.error { color: #e74c3c; }
.channel-header {
  display: flex;
  gap: 20px;
  align-items: flex-start;
  margin-bottom: 28px;
  padding: 16px;
  background: #17212b;
  border-radius: 12px;
}
.avatar { width: 72px; height: 72px; border-radius: 50%; object-fit: cover; flex-shrink: 0; }
h1 { font-size: 1.3em; margin-bottom: 4px; }
.subs { font-size: 0.82em; color: #aab8c2; margin-bottom: 8px; }
.desc {
  font-size: 0.8em;
  color: #aab8c2;
  line-height: 1.5;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.section-title { font-size: 1em; color: #aab8c2; margin-bottom: 14px; }
.videos-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 14px;
}
</style>
