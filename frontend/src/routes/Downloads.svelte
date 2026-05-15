<script lang="ts">
  import { api, type JobStatus } from '../api'

  let jobs = $state<JobStatus[]>([])
  let loading = $state(true)
  let error = $state('')

  $effect(() => {
    refresh()
    const timer = setInterval(refresh, 5000)
    return () => clearInterval(timer)
  })

  async function refresh() {
    try {
      jobs = await api.listJobs()
    } catch (e: any) {
      error = e.message
    } finally {
      loading = false
    }
  }

  function statusChip(s: JobStatus['status']) {
    if (s === 'done') return 'chip-done'
    if (s === 'failed') return 'chip-failed'
    return 'chip-active'
  }
</script>

<div class="downloads-page">
  <div class="page-header">
    <h2 class="page-title">Downloads</h2>
    {#if jobs.length > 0}
      <span class="job-count">{jobs.length} job{jobs.length === 1 ? '' : 's'}</span>
    {/if}
  </div>

  {#if loading}
    <div class="state-msg"><div class="spinner"></div></div>
  {:else if error}
    <div class="state-msg error">{error}</div>
  {:else if jobs.length === 0}
    <div class="state-msg">No downloads yet. Visit a video page to start one.</div>
  {:else}
    <div class="job-list">
      {#each jobs as job}
        <div class="job-row glass">
          <div class="job-id">{job.job_id}</div>
          <div class="job-bar">
            {#if job.status === 'downloading' || job.status === 'uploading'}
              <div class="progress-bar">
                <div class="progress-fill" style="width:{job.progress}%"></div>
              </div>
            {/if}
          </div>
          <span class="chip {statusChip(job.status)}">
            {job.status}{job.progress && job.status !== 'done' ? ` ${job.progress}%` : ''}
          </span>
          {#if job.status === 'done' && job.drive_file_id}
            <a href={`/api/jobs/${job.job_id}/stream`} class="btn-ghost dl-btn" download>↓ Save</a>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
.downloads-page { max-width: 860px; }
.page-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 20px;
}
.page-title { font-size: 1.2rem; font-weight: 600; }
.job-count { font-size: 0.82rem; color: var(--text-secondary); }

.job-list { display: flex; flex-direction: column; gap: 8px; }
.job-row {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 16px;
  transition: border-color var(--t-fast);
}
.job-row:hover { border-color: rgba(255,255,255,0.13); }
.job-id {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 0.78rem;
  color: var(--text-secondary);
  min-width: 90px;
  flex-shrink: 0;
}
.job-bar { flex: 1; }
.progress-bar {
  height: 4px;
  background: var(--glass-bg-hover);
  border-radius: 2px;
  overflow: hidden;
}
.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, var(--accent), var(--accent-hover));
  transition: width 0.4s ease;
  box-shadow: 0 0 6px var(--accent-glow);
}
.dl-btn { font-size: 0.8rem; padding: 5px 12px; }
</style>
