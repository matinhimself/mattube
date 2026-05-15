<script lang="ts">
  import { Link } from 'svelte-routing'
  import type { SearchResult } from '../api'

  let { result }: { result: SearchResult } = $props()
</script>

<Link to={`/video/${result.video_id}`} class="card">
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
        <Link to={`/channel/${result.channel_id}`} onclick={(e) => e.stopPropagation()}>
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
  background: #17212b;
  border-radius: 10px;
  overflow: hidden;
  transition: background 0.15s;
}
:global(.card:hover) { background: #1e2f3f; }
.thumb-wrap { position: relative; aspect-ratio: 16/9; background: #0e1621; }
.thumb { width: 100%; height: 100%; object-fit: cover; display: block; }
.duration {
  position: absolute;
  bottom: 6px;
  right: 6px;
  background: rgba(0,0,0,0.8);
  color: #fff;
  font-size: 0.75em;
  padding: 2px 5px;
  border-radius: 3px;
}
.info { padding: 10px 12px 12px; }
.title {
  font-size: 0.88em;
  font-weight: 500;
  color: #e8e8e8;
  line-height: 1.3;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  margin-bottom: 6px;
}
.channel { font-size: 0.78em; color: #6ab2f2; margin-bottom: 3px; }
.views { font-size: 0.72em; color: #aab8c2; }
</style>
