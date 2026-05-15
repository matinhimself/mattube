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
  <div class="loading">Loading...</div>
{:else if error}
  <div class="error">{error}</div>
{:else if info}
  <div class="video-page">

    <div class="meta">
      <h1>{info.title}</h1>
      <div class="sub-meta">
        {#if info.channel_name}
          <Link to={`/channel/${info.channel_id}`} class="channel-link">{info.channel_name}</Link>
        {/if}
        {#if info.duration}
          <span class="dim">{fmtDuration(info.duration)}</span>
        {/if}
        {#if info.view_count}
          <span class="dim">{info.view_count} views</span>
        {/if}
      </div>
    </div>

    {#if info.thumbnail}
      <img src={info.thumbnail} alt={info.title} class="thumbnail" />
    {/if}

    <!-- Download / play controls -->
    <div class="actions">
      <select bind:value={selectedQuality} class="quality-select">
        {#each QUALITIES as q}
          <option value={q}>{q}</option>
        {/each}
      </select>

      {#if !jobStatus}
        <button onclick={download} disabled={submitting} class="btn-primary">
          {submitting ? 'Submitting...' : '⬇ Download'}
        </button>
      {:else}
        <div class="job-status">
          {#if jobStatus.status === 'done'}
            <button onclick={play} class="btn-play">▶ Play</button>
            <span class="status-done">✓ Ready</span>
          {:else if jobStatus.status === 'failed'}
            <span class="status-failed">✗ {jobStatus.error || 'Failed'}</span>
          {:else}
            <div class="progress-bar">
              <div class="progress-fill" style="width:{jobStatus.progress}%"></div>
            </div>
            <span class="status-label">{jobStatus.status} {jobStatus.progress}%</span>
          {/if}
        </div>
      {/if}

      <a href={`https://www.youtube.com/watch?v=${videoId}`} target="_blank" class="btn-yt">
        ↗ YouTube
      </a>
    </div>

    {#if info.description}
      <div class="description">{info.description}</div>
    {/if}

  </div>
{/if}

<style>
.loading, .error { padding: 40px; text-align: center; color: #aab8c2; }
.error { color: #e74c3c; }
.video-page { max-width: 720px; }
h1 { font-size: 1.2em; margin-bottom: 8px; line-height: 1.4; }
.sub-meta { display: flex; gap: 14px; flex-wrap: wrap; margin-bottom: 14px; font-size: 0.82em; }
.dim { color: #aab8c2; }
:global(.channel-link) { color: #6ab2f2 !important; }
.thumbnail { width: 100%; border-radius: 10px; margin-bottom: 16px; }
.actions { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; margin-bottom: 20px; }
.quality-select {
  padding: 8px 12px;
  background: #17212b;
  border: 1.5px solid #2b3a4a;
  border-radius: 8px;
  color: #e8e8e8;
}
.btn-primary, .btn-play {
  padding: 8px 18px;
  background: #2b8fcc;
  border: none;
  border-radius: 8px;
  color: #fff;
  font-size: 0.9em;
  cursor: pointer;
}
.btn-primary:disabled { opacity: 0.6; cursor: default; }
.btn-play { background: #27ae60; }
.btn-yt {
  padding: 8px 14px;
  background: #17212b;
  border: 1.5px solid #2b3a4a;
  border-radius: 8px;
  color: #aab8c2;
  font-size: 0.85em;
}
.job-status { display: flex; align-items: center; gap: 10px; }
.progress-bar {
  width: 120px; height: 6px;
  background: #2b3a4a;
  border-radius: 3px;
  overflow: hidden;
}
.progress-fill { height: 100%; background: #2b8fcc; transition: width 0.4s; }
.status-label { font-size: 0.8em; color: #aab8c2; }
.status-done { color: #27ae60; font-size: 0.85em; }
.status-failed { color: #e74c3c; font-size: 0.85em; }
.description {
  font-size: 0.82em;
  color: #aab8c2;
  line-height: 1.6;
  white-space: pre-wrap;
  background: #17212b;
  padding: 12px;
  border-radius: 8px;
}
</style>
