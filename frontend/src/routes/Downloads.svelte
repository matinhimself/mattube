<script lang="ts">
  import { api, type JobStatus } from '../api'

  let jobs = $state<JobStatus[]>([])
  let loading = $state(true)
  let error = $state('')

  $effect(() => {
    api.listJobs()
      .then(j => { jobs = j; loading = false })
      .catch(e => { error = e.message; loading = false })
  })

  function statusColor(s: JobStatus['status']) {
    if (s === 'done') return '#27ae60'
    if (s === 'failed') return '#e74c3c'
    return '#2b8fcc'
  }
</script>

<h2 class="title">Downloads</h2>

{#if loading}
  <div class="loading">Loading...</div>
{:else if error}
  <div class="error">{error}</div>
{:else if jobs.length === 0}
  <div class="empty">No downloads yet.</div>
{:else}
  <div class="job-list">
    {#each jobs as job}
      <div class="job-row">
        <div class="job-id">{job.job_id}</div>
        <div class="job-bar">
          {#if job.status === 'downloading' || job.status === 'uploading'}
            <div class="progress-bar">
              <div class="progress-fill" style="width:{job.progress}%"></div>
            </div>
          {/if}
        </div>
        <div class="job-status" style="color:{statusColor(job.status)}">
          {job.status}{job.progress && job.status !== 'done' ? ` ${job.progress}%` : ''}
        </div>
        {#if job.status === 'done' && job.drive_file_id}
          <a href={`/api/jobs/${job.job_id}/stream`} class="dl-link" download>⬇ Save</a>
        {/if}
      </div>
    {/each}
  </div>
{/if}

<style>
.title { margin-bottom: 16px; font-size: 1.1em; }
.loading, .error, .empty { padding: 40px; text-align: center; color: #aab8c2; }
.error { color: #e74c3c; }
.job-list { display: flex; flex-direction: column; gap: 8px; }
.job-row {
  display: flex;
  align-items: center;
  gap: 12px;
  background: #17212b;
  padding: 10px 14px;
  border-radius: 8px;
}
.job-id { font-family: monospace; font-size: 0.82em; color: #aab8c2; min-width: 80px; }
.job-bar { flex: 1; }
.progress-bar { height: 5px; background: #2b3a4a; border-radius: 3px; overflow: hidden; }
.progress-fill { height: 100%; background: #2b8fcc; transition: width 0.3s; }
.job-status { font-size: 0.82em; min-width: 80px; text-align: right; }
.dl-link {
  font-size: 0.82em;
  padding: 4px 10px;
  background: #2b3a4a;
  border-radius: 6px;
  color: #aab8c2;
  white-space: nowrap;
}
</style>
