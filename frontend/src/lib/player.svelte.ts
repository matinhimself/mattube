export interface Track {
  jobId: string
  videoId: string
  title: string
  channelName: string
  thumbnailUrl: string
  duration: number
}

// Svelte 5: share reactive state via a class (properties are reactive by default)
class PlayerStore {
  currentTrack = $state<Track | null>(null)
  playing = $state(false)
  currentTime = $state(0)
  duration = $state(0)
  minimized = $state(false)

  private _play: (() => void) | null = null
  private _pause: (() => void) | null = null
  private _setSrc: ((url: string) => void) | null = null

  registerCallbacks(play: () => void, pause: () => void, setSrc: (url: string) => void) {
    this._play = play
    this._pause = pause
    this._setSrc = setSrc
  }

  loadTrack(track: Track) {
    this.currentTrack = track
    this.playing = false
    this._setSrc?.(`/api/jobs/${track.jobId}/stream`)
    this._play?.()
    this.playing = true
    this.minimized = false
  }

  togglePlay() {
    if (this.playing) {
      this._pause?.()
      this.playing = false
    } else {
      this._play?.()
      this.playing = true
    }
  }

  setTime(t: number) { this.currentTime = t }
  setDuration(d: number) { this.duration = d }
  setPlaying(p: boolean) { this.playing = p }
  minimize() { this.minimized = true }
  expand() { this.minimized = false }
}

export const player = new PlayerStore()
