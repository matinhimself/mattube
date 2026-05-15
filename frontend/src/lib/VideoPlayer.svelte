<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { player } from './player.svelte'

  let el: HTMLVideoElement
  let vjs: any
  const SPEEDS = [0.25, 0.5, 0.75, 1, 1.25, 1.5, 1.75, 2]

  onMount(async () => {
    const videojs = (window as any).videojs
    if (!videojs) return

    vjs = videojs(el, {
      fluid: true,
      playbackRates: SPEEDS,
      controlBar: { playbackRateMenuButton: true },
      html5: { nativeAudioTracks: true, nativeVideoTracks: true },
      userActions: {
        hotkeys(e: KeyboardEvent) {
          if (e.key === ' ' || e.which === 32) {
            e.preventDefault(); vjs.paused() ? vjs.play() : vjs.pause()
          } else if (e.which === 37) {
            e.preventDefault(); vjs.currentTime(Math.max(0, vjs.currentTime() - 10))
          } else if (e.which === 39) {
            e.preventDefault(); vjs.currentTime(Math.min(vjs.duration(), vjs.currentTime() + 10))
          } else if (e.which === 38) {
            e.preventDefault(); vjs.volume(Math.min(1, vjs.volume() + 0.1))
          } else if (e.which === 40) {
            e.preventDefault(); vjs.volume(Math.max(0, vjs.volume() - 0.1))
          } else if (e.which === 70) {
            e.preventDefault(); vjs.isFullscreen() ? vjs.exitFullscreen() : vjs.requestFullscreen()
          } else if (e.which === 77) {
            e.preventDefault(); vjs.muted(!vjs.muted())
          } else if (e.shiftKey && e.which === 188) {
            const idx = SPEEDS.indexOf(vjs.playbackRate())
            if (idx > 0) vjs.playbackRate(SPEEDS[idx - 1])
          } else if (e.shiftKey && e.which === 190) {
            const idx = SPEEDS.indexOf(vjs.playbackRate())
            if (idx < SPEEDS.length - 1) vjs.playbackRate(SPEEDS[idx + 1])
          }
        }
      }
    })

    const vjsEl = el.closest('.video-js') as HTMLElement
    if (vjsEl) {
      let holdTimer: ReturnType<typeof setTimeout>
      let holdActive = false
      vjsEl.addEventListener('touchstart', (e: TouchEvent) => {
        const rect = vjsEl.getBoundingClientRect()
        if (e.touches[0].clientX - rect.left < rect.width * 0.5) return
        holdTimer = setTimeout(() => { holdActive = true; vjs.playbackRate(2) }, 500)
      })
      vjsEl.addEventListener('touchend', () => {
        clearTimeout(holdTimer)
        if (holdActive) { holdActive = false; vjs.playbackRate(1) }
      })
      vjsEl.addEventListener('touchmove', () => clearTimeout(holdTimer))
    }

    vjs.on('timeupdate', () => player.setTime(vjs.currentTime()))
    vjs.on('durationchange', () => player.setDuration(vjs.duration()))
    vjs.on('play', () => player.setPlaying(true))
    vjs.on('pause', () => player.setPlaying(false))

    player.registerCallbacks(
      () => vjs.play(),
      () => vjs.pause(),
      (url: string) => vjs.src({ src: url, type: 'video/mp4' })
    )
  })

  onDestroy(() => { if (vjs) vjs.dispose() })
</script>

{#if player.currentTrack && !player.minimized}
<div class="player-overlay">
  <video
    bind:this={el}
    id="vjs-player"
    class="video-js vjs-big-play-centered"
    controls
    preload="metadata"
    playsinline
    style="width:100%"
  ></video>
  <button class="minimize-btn glass" onclick={() => player.minimize()}>↙ Minimize</button>
</div>
{:else}
<video
  bind:this={el}
  id="vjs-player"
  class="video-js vjs-big-play-centered"
  controls
  preload="metadata"
  playsinline
  style="display:none"
></video>
{/if}

{#if player.currentTrack && player.minimized}
<div class="mini-player">
  <img src={player.currentTrack.thumbnailUrl} alt="" class="mini-thumb" />
  <div class="mini-info">
    <div class="mini-title">{player.currentTrack.title}</div>
    <div class="mini-channel">{player.currentTrack.channelName}</div>
  </div>
  <button onclick={() => player.togglePlay()} class="mini-ctrl">{player.playing ? '⏸' : '▶'}</button>
  <button onclick={() => player.expand()} class="mini-ctrl mini-ctrl-expand">⤢</button>
</div>
{/if}

<style>
.player-overlay {
  position: fixed;
  inset: 0;
  background: #000;
  z-index: 100;
  display: flex;
  align-items: center;
  justify-content: center;
}
.minimize-btn {
  position: absolute;
  top: 14px;
  right: 18px;
  padding: 6px 14px;
  font-size: 0.82rem;
  font-family: inherit;
  cursor: pointer;
  color: var(--text-secondary);
  border-color: var(--glass-border);
  z-index: 101;
  transition: color var(--t-fast), border-color var(--t-fast);
}
.minimize-btn:hover { color: var(--text-primary); border-color: var(--accent); }

.mini-player {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  height: 68px;
  background: rgba(8, 8, 8, 0.90);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-top: 1px solid var(--glass-border);
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 16px;
  z-index: 99;
}
.mini-thumb {
  width: 52px;
  height: 36px;
  object-fit: cover;
  border-radius: var(--radius-sm);
  flex-shrink: 0;
}
.mini-info { flex: 1; overflow: hidden; }
.mini-title {
  font-size: 0.82rem;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.mini-channel { font-size: 0.72rem; color: var(--accent); margin-top: 2px; }
.mini-ctrl {
  background: none;
  border: none;
  color: var(--text-secondary);
  font-size: 1.2rem;
  cursor: pointer;
  padding: 8px;
  transition: color var(--t-fast);
}
.mini-ctrl:hover { color: var(--text-primary); }
.mini-ctrl-expand:hover { color: var(--accent); }
</style>
