<script lang="ts">
  import { Link } from 'svelte-routing'
  import type { SearchResult } from '../api'

  let { result }: { result: SearchResult } = $props()
</script>

<Link to={`/video/${result.video_id}`} class="card glass">
  <div class="thumb-wrap">
    <img src={result.thumbnail} alt={result.title} class="thumb" loading="lazy" />
    {#if result.duration}
      <span class="duration">{result.duration}</span>
    {/if}
  </div>
  <div class="info">
    <div class="title">{result.title}</div>
    {#if result.channel_name}
      <div class="channel">
        <Link to={`/channel/${result.channel_id}`} onclick={(e: MouseEvent) => e.stopPropagation()}>
          {result.channel_name}
        </Link>
      </div>
    {/if}
    {#if result.view_count}
      <div class="views">{result.view_count} views</div>
    {/if}
  </div>
</Link>

<style>
:global(.card) {
  display: block;
  text-decoration: none;
  overflow: hidden;
  transition: transform var(--t-base), box-shadow var(--t-base), border-color var(--t-base);
}
:global(.card:hover) {
  transform: translateY(-3px);
  border-color: rgba(224, 48, 48, 0.3);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5), 0 0 0 1px rgba(224,48,48,0.1);
}
.thumb-wrap {
  position: relative;
  aspect-ratio: 16/9;
  background: #111;
  overflow: hidden;
}
.thumb {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  transition: transform var(--t-slow);
}
:global(.card:hover) .thumb { transform: scale(1.04); }
.duration {
  position: absolute;
  bottom: 6px;
  right: 6px;
  background: rgba(0,0,0,0.85);
  color: #fff;
  font-size: 0.72rem;
  font-weight: 500;
  padding: 2px 6px;
  border-radius: 4px;
  backdrop-filter: blur(4px);
}
.info { padding: 12px 14px 14px; }
.title {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-primary);
  line-height: 1.35;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  margin-bottom: 6px;
}
.channel { font-size: 0.78rem; color: var(--accent); margin-bottom: 3px; }
.channel :global(a) { color: inherit; }
.channel :global(a:hover) { color: var(--accent-hover); }
.views { font-size: 0.72rem; color: var(--text-secondary); }
</style>
