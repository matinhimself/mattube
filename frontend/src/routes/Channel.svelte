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
  <div class="state-msg"><div class="spinner"></div></div>
{:else if error}
  <div class="state-msg error">{error}</div>
{:else if info}
  <div class="channel-header glass">
    {#if info.avatar}
      <div class="avatar-wrap">
        <img src={info.avatar} alt={info.name} class="avatar" />
      </div>
    {/if}
    <div class="channel-meta">
      <h1 class="channel-name">{info.name}</h1>
      {#if info.subscribers}
        <div class="channel-subs">{info.subscribers} subscribers</div>
      {/if}
      {#if info.description}
        <div class="channel-desc">{info.description}</div>
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
    <div class="state-msg">No videos found</div>
  {/if}
{/if}

<style>
.channel-header {
  display: flex;
  gap: 20px;
  align-items: flex-start;
  padding: 20px 24px;
  margin-bottom: 28px;
}
.avatar-wrap {
  width: 80px;
  height: 80px;
  flex-shrink: 0;
  border-radius: 50%;
  overflow: hidden;
  border: 2px solid var(--accent);
  box-shadow: 0 0 20px var(--accent-glow);
}
.avatar { width: 100%; height: 100%; object-fit: cover; }
.channel-name { font-size: 1.3rem; font-weight: 600; margin-bottom: 4px; }
.channel-subs { font-size: 0.82rem; color: var(--text-secondary); margin-bottom: 8px; }
.channel-desc {
  font-size: 0.8rem;
  color: var(--text-secondary);
  line-height: 1.55;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.section-title {
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 14px;
  text-transform: uppercase;
  letter-spacing: 0.07em;
}
.videos-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 14px;
}
</style>
