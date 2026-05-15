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

    // Touch hold right half for 2× speed
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
  <button class="minimize-btn" onclick={() => player.minimize()}>▼</button>
</div>
{:else}
<!-- Video element always in DOM so Video.js can initialize -->
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
  <button onclick={() => player.togglePlay()} class="mini-btn">{player.playing ? '⏸' : '▶'}</button>
  <button onclick={() => player.expand()} class="mini-btn">⤢</button>
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
  top: 12px;
  right: 16px;
  background: rgba(0,0,0,0.6);
  border: none;
  color: #fff;
  font-size: 1.2rem;
  cursor: pointer;
  padding: 6px 10px;
  border-radius: 6px;
  z-index: 101;
}
.mini-player {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  height: 64px;
  background: #17212b;
  border-top: 1px solid #2b3a4a;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  z-index: 99;
}
.mini-thumb { width: 48px; height: 36px; object-fit: cover; border-radius: 4px; }
.mini-info { flex: 1; overflow: hidden; }
.mini-title {
  font-size: 0.82em;
  color: #e8e8e8;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.mini-channel { font-size: 0.72em; color: #6ab2f2; }
.mini-btn { background: none; border: none; color: #aab8c2; font-size: 1.2rem; cursor: pointer; padding: 6px; }
</style>
