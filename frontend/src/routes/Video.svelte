<script lang="ts">
  import { api, type VideoInfo, type JobStatus } from '../api'
  import { player } from '../lib/player.svelte'
  import { Link } from 'svelte-routing'

  let { videoId }: { videoId: string } = $props()

  let info = $state<VideoInfo | null>(null)
  let loading = $state(true)
  let error = $state('')

  let selectedQuality = $state('1080p')
  let submitting = $state(false)
  let jobStatus = $state<JobStatus | null>(null)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  $effect(() => {
    loading = true
    api.getVideo(videoId)
      .then(v => { info = v; loading = false })
      .catch(e => { error = e.message; loading = false })
    return () => { if (pollTimer) clearInterval(pollTimer) }
  })

  async function download() {
    if (!info) return
    submitting = true
    try {
      const { job_id } = await api.submitJob(`https://www.youtube.com/watch?v=${videoId}`, selectedQuality)
      jobStatus = { job_id, status: 'pending', progress: 0, updated_at: '' }
      pollTimer = setInterval(() => pollStatus(job_id), 3000)
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

  function play() {
    if (!info || !jobStatus?.drive_file_id) return
    player.loadTrack({
      jobId: jobStatus.job_id,
      videoId: info.video_id,
      title: info.title,
      channelName: info.channel_name,
      thumbnailUrl: info.thumbnail,
      duration: info.duration,
    })
  }

  const QUALITIES = ['best', '2160p', '1440p', '1080p', '720p', '480p', '360p', 'audio']

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
  <div class="video-page">

    <div class="meta glass">
      <h1 class="meta-title">{info.title}</h1>
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

    {#if info.thumbnail}
      <div class="thumb-wrap glass">
        <img src={info.thumbnail} alt={info.title} class="thumb" />
      </div>
    {/if}

    <div class="actions glass">
      <select bind:value={selectedQuality} class="select-base">
        {#each QUALITIES as q}
          <option value={q}>{q}</option>
        {/each}
      </select>

      {#if !jobStatus}
        <button onclick={download} disabled={submitting} class="btn-accent">
          {submitting ? 'Submitting…' : '↓ Download'}
        </button>
      {:else}
        <div class="job-status">
          {#if jobStatus.status === 'done'}
            <button onclick={play} class="btn-play">▶ Play</button>
            <span class="chip chip-done">Ready</span>
          {:else if jobStatus.status === 'failed'}
            <span class="chip chip-failed">{jobStatus.error || 'Failed'}</span>
          {:else}
            <div class="progress-wrap">
              <div class="progress-bar">
                <div class="progress-fill" style="width:{jobStatus.progress}%"></div>
              </div>
              <span class="chip chip-active">{jobStatus.status} {jobStatus.progress}%</span>
            </div>
          {/if}
        </div>
      {/if}

      <a href={`https://www.youtube.com/watch?v=${videoId}`} target="_blank" class="btn-ghost">
        ↗ YouTube
      </a>
    </div>

    {#if info.description}
      <div class="description glass">{info.description}</div>
    {/if}

  </div>
{/if}

<style>
.video-page { max-width: 720px; display: flex; flex-direction: column; gap: 14px; }

.meta { padding: 18px 20px; }
.meta-title { font-size: 1.15rem; font-weight: 600; line-height: 1.4; margin-bottom: 10px; }
.meta-row { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
:global(.meta-channel) { color: var(--accent) !important; font-size: 0.85rem; font-weight: 500; }
.meta-sep { color: var(--text-muted); }
.meta-dim { color: var(--text-secondary); font-size: 0.82rem; }

.thumb-wrap { overflow: hidden; aspect-ratio: 16/9; padding: 0; }
.thumb { width: 100%; height: 100%; object-fit: cover; display: block; }

.actions {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
  padding: 14px 16px;
}
.job-status { display: flex; align-items: center; gap: 10px; }
.progress-wrap { display: flex; align-items: center; gap: 10px; }
.progress-bar {
  width: 120px;
  height: 5px;
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
  font-size: 0.82rem;
  color: var(--text-secondary);
  line-height: 1.65;
  white-space: pre-wrap;
  padding: 16px 20px;
  max-height: 200px;
  overflow-y: auto;
}
</style>
