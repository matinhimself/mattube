<script lang="ts">
  import { api, type VideoInfo, type JobStatus, type SearchResult } from '../api'
  import { player } from '../lib/player.svelte'
  import { Link } from 'svelte-routing'

  let { videoId }: { videoId: string } = $props()

  let info = $state<VideoInfo | null>(null)
  let related = $state<SearchResult[]>([])
  let loading = $state(true)
  let error = $state('')
  let descExpanded = $state(false)

  let selectedQuality = $state('1080p')
  let chunkSizeMB = $state(0)
  let submitting = $state(false)
  let jobStatus = $state<JobStatus | null>(null)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  $effect(() => {
    loading = true
    error = ''
    jobStatus = null
    related = []
    Promise.all([
      api.getVideo(videoId),
      api.getRelatedVideos(videoId).catch(() => [] as SearchResult[]),
    ]).then(([v, r]) => {
      info = v
      related = r ?? []
      loading = false
    }).catch(e => {
      error = e.message
      loading = false
    })
    return () => { if (pollTimer) clearInterval(pollTimer) }
  })

  async function download() {
    if (!info) return
    submitting = true
    try {
      const { job_id } = await api.submitJob(`https://www.youtube.com/watch?v=${videoId}`, selectedQuality, chunkSizeMB)
      jobStatus = { job_id, status: 'pending', progress: 0, updated_at: '' }
      pollTimer = setInterval(() => pollStatus(job_id), 2000)
    } catch (e: any) {
      error = e.message
    } finally {
      submitting = false
    }
  }

  async function pollStatus(jobId: string) {
    try {
      const s = await api.getJobStatus(jobId)
      jobStatus = s
      if (s.status === 'done' || s.status === 'failed') {
        if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
      }
    } catch {}
  }

  function isPlayable(s: JobStatus): boolean {
    if (s.status === 'done') return true
    return s.status === 'chunking' && (s.chunks?.length ?? 0) > 0
  }

  function chunkProgress(s: JobStatus): number {
    if (s.status === 'chunking') {
      const n = s.chunks?.length ?? 0
      if (s.total_chunks) return Math.round(n / s.total_chunks * 100)
      return n > 0 ? 50 : 0 // unknown total: show indeterminate
    }
    return s.progress
  }

  function chunkLabel(s: JobStatus): string {
    if (s.status === 'chunking') {
      const n = s.chunks?.length ?? 0
      return s.total_chunks ? `chunking ${n}/${s.total_chunks}` : `chunking (${n} ready)`
    }
    return `${s.status} ${s.progress}%`
  }

  function play() {
    if (!info || !jobStatus) return
    const isChunked = (jobStatus.chunks?.length ?? 0) > 0
    if (!isChunked && !jobStatus.drive_file_id) return
    player.loadTrack({
      jobId: jobStatus.job_id,
      videoId: info.video_id,
      title: info.title,
      channelName: info.channel_name,
      thumbnailUrl: info.thumbnail,
      duration: info.duration,
      chunked: isChunked,
    })
  }

  const QUALITIES = ['best', '2160p', '1440p', '1080p', '720p', '480p', '360p', 'audio']
  const CHUNK_OPTS = [
    { label: 'No chunks', value: 0 },
    { label: '5 MB chunks', value: 5 },
    { label: '10 MB chunks', value: 10 },
    { label: '20 MB chunks', value: 20 },
    { label: '50 MB chunks', value: 50 },
  ]

  function fmtDuration(s: number) {
    const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60
    return h ? `${h}:${String(m).padStart(2,'0')}:${String(sec).padStart(2,'0')}`
             : `${m}:${String(sec).padStart(2,'0')}`
  }
</script>

{#if loading}
  <div class="state-msg"><div class="spinner"></div></div>
{:else if error}
  <div class="state-msg error">{error}</div>
{:else if info}
  <div class="video-layout">

    <!-- Main column -->
    <div class="main-col">

      <!-- Thumbnail hero -->
      {#if info.thumbnail}
        <div class="thumb-hero glass">
          <img src={info.thumbnail} alt={info.title} class="thumb-img" />
        </div>
      {/if}

      <!-- Title + meta -->
      <div class="info-block glass">
        <h1 class="video-title">{info.title}</h1>
        <div class="meta-row">
          {#if info.channel_name}
            <Link to={`/channel/${info.channel_id}`} class="meta-channel">{info.channel_name}</Link>
          {/if}
          {#if info.duration}
            <span class="meta-sep">·</span>
            <span class="meta-dim">{fmtDuration(info.duration)}</span>
          {/if}
          {#if info.view_count}
            <span class="meta-sep">·</span>
            <span class="meta-dim">{info.view_count} views</span>
          {/if}
        </div>
      </div>

      <!-- Actions bar -->
      <div class="actions glass">
        <select bind:value={selectedQuality} class="select-base">
          {#each QUALITIES as q}
            <option value={q}>{q}</option>
          {/each}
        </select>

        <select bind:value={chunkSizeMB} class="select-base">
          {#each CHUNK_OPTS as o}
            <option value={o.value}>{o.label}</option>
          {/each}
        </select>

        {#if !jobStatus}
          <button onclick={download} disabled={submitting} class="btn-accent">
            {submitting ? 'Submitting…' : '↓ Download'}
          </button>
        {:else}
          <div class="job-status">
            {#if isPlayable(jobStatus)}
              <button onclick={play} class="btn-play">▶ Play</button>
              <span class="chip chip-done">
                {jobStatus.status === 'done' ? 'Ready' : `Buffering ${jobStatus.chunks?.length}/${jobStatus.total_chunks}`}
              </span>
            {:else if jobStatus.status === 'failed'}
              <span class="chip chip-failed">{jobStatus.error || 'Failed'}</span>
            {:else}
              <div class="progress-wrap">
                <div class="progress-bar">
                  <div class="progress-fill" style="width:{chunkProgress(jobStatus)}%"></div>
                </div>
                <span class="chip chip-active">{chunkLabel(jobStatus)}</span>
              </div>
            {/if}
          </div>
        {/if}

        <a href={`https://www.youtube.com/watch?v=${videoId}`} target="_blank" class="btn-ghost">
          ↗ YouTube
        </a>
      </div>

      <!-- Description -->
      {#if info.description}
        <div class="description glass" class:expanded={descExpanded}>
          <p class="desc-text">{info.description}</p>
          <button
            class="desc-toggle"
            onclick={() => descExpanded = !descExpanded}
          >{descExpanded ? 'Show less' : 'Show more'}</button>
        </div>
      {/if}

    </div>

    <!-- Sidebar: related videos -->
    {#if related.length > 0}
      <aside class="sidebar">
        <h2 class="sidebar-heading">Up next</h2>
        <div class="related-list">
          {#each related as v}
            <Link to={`/video/${v.video_id}`} class="related-card">
              <div class="related-thumb-wrap">
                {#if v.thumbnail}
                  <img src={v.thumbnail} alt={v.title} class="related-thumb" loading="lazy" />
                {:else}
                  <div class="related-thumb-placeholder"></div>
                {/if}
                {#if v.duration}
                  <span class="related-duration">{v.duration}</span>
                {/if}
              </div>
              <div class="related-info">
                <p class="related-title">{v.title}</p>
                {#if v.channel_name}
                  <p class="related-channel">{v.channel_name}</p>
                {/if}
                {#if v.view_count}
                  <p class="related-views">{v.view_count}</p>
                {/if}
              </div>
            </Link>
          {/each}
        </div>
      </aside>
    {/if}

  </div>
{/if}

<style>
.video-layout {
  display: grid;
  grid-template-columns: 1fr 320px;
  gap: 20px;
  align-items: start;
  max-width: 1180px;
}
@media (max-width: 860px) {
  .video-layout { grid-template-columns: 1fr; }
}

/* ── Main column ── */
.main-col { display: flex; flex-direction: column; gap: 12px; min-width: 0; }

.thumb-hero { overflow: hidden; aspect-ratio: 16/9; padding: 0; }
.thumb-img  { width: 100%; height: 100%; object-fit: cover; display: block; }

.info-block { padding: 16px 20px; }
.video-title { font-size: 1.1rem; font-weight: 600; line-height: 1.45; margin-bottom: 10px; }
.meta-row { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
:global(.meta-channel) { color: var(--accent) !important; font-size: 0.82rem; font-weight: 500; }
.meta-sep { color: var(--text-muted); }
.meta-dim { color: var(--text-secondary); font-size: 0.8rem; }

.actions {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
  padding: 12px 16px;
}
.job-status { display: flex; align-items: center; gap: 10px; }
.progress-wrap { display: flex; align-items: center; gap: 10px; }
.progress-bar {
  width: 110px;
  height: 4px;
  background: var(--glass-bg-hover);
  border-radius: 3px;
  overflow: hidden;
}
.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, var(--accent), var(--accent-hover));
  transition: width 0.4s ease;
  box-shadow: 0 0 8px var(--accent-glow);
}

.description {
  padding: 14px 18px;
  position: relative;
  overflow: hidden;
  max-height: 96px;
  transition: max-height 0.3s ease;
}
.description.expanded { max-height: 600px; }
.desc-text {
  font-size: 0.81rem;
  color: var(--text-secondary);
  line-height: 1.65;
  white-space: pre-wrap;
  margin-bottom: 8px;
}
.desc-toggle {
  background: none;
  border: none;
  color: var(--accent);
  font-size: 0.78rem;
  font-family: inherit;
  cursor: pointer;
  padding: 0;
  transition: color var(--t-fast);
}
.desc-toggle:hover { color: var(--accent-hover); }

/* ── Sidebar ── */
.sidebar { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
.sidebar-heading {
  font-size: 0.8rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-muted);
  padding: 0 2px;
}

.related-list { display: flex; flex-direction: column; gap: 8px; }

:global(.related-card) {
  display: flex;
  gap: 10px;
  padding: 8px;
  border-radius: var(--radius-sm);
  border: 1px solid transparent;
  text-decoration: none !important;
  transition: background var(--t-fast), border-color var(--t-fast);
}
:global(.related-card:hover) {
  background: var(--glass-bg);
  border-color: var(--glass-border);
}

.related-thumb-wrap {
  flex-shrink: 0;
  width: 120px;
  aspect-ratio: 16/9;
  border-radius: 6px;
  overflow: hidden;
  position: relative;
  background: var(--glass-bg);
}
.related-thumb {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  transition: transform 0.2s ease;
}
:global(.related-card:hover) .related-thumb { transform: scale(1.03); }
.related-thumb-placeholder { width: 100%; height: 100%; background: var(--glass-bg-hover); }
.related-duration {
  position: absolute;
  bottom: 4px;
  right: 4px;
  background: rgba(0,0,0,0.75);
  color: #fff;
  font-size: 0.68rem;
  font-weight: 500;
  padding: 1px 5px;
  border-radius: 3px;
}

.related-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
.related-title {
  font-size: 0.82rem;
  font-weight: 500;
  color: var(--text-primary);
  line-height: 1.35;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.related-channel {
  font-size: 0.75rem;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.related-views {
  font-size: 0.72rem;
  color: var(--text-muted);
}
</style>
