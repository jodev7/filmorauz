// Mid-roll ad scheduling for the video player (non-premium viewers).
//
// A break is due
//   - after `intervalSeconds` of actually watched time — seeking forward does
//     not count, so skipping ahead doesn't dodge the break; or
//   - when the viewer scrubs the timeline `seekTriggerCount` times within
//     `seekWindowMs`;
// but never sooner than `minGapMs` after the previous break (the pre-roll
// counts as one, since the schedule starts when the player mounts).
//
// Pure logic with an injectable clock so it can be tested without a browser.

export interface AdScheduleOptions {
  intervalSeconds: number;
  seekTriggerCount: number;
  seekWindowMs: number;
  minGapMs: number;
}

export const DEFAULT_AD_SCHEDULE: AdScheduleOptions = {
  intervalSeconds: 10 * 60,
  seekTriggerCount: 3,
  seekWindowMs: 60_000,
  minGapMs: 3 * 60_000,
};

// A timeupdate step larger than this is a seek, not playback.
const MAX_PLAYBACK_STEP = 2.5;

export class AdSchedule {
  private watched = 0;
  private lastTime = 0;
  private lastBreakAt: number;
  private seeks: number[] = [];

  constructor(
    private readonly opts: AdScheduleOptions = DEFAULT_AD_SCHEDULE,
    now: number = Date.now()
  ) {
    this.lastBreakAt = now;
  }

  /** Start over (new source, or right after an ad break finished). */
  reset(now: number = Date.now(), currentTime = 0): void {
    this.watched = 0;
    this.seeks = [];
    this.lastBreakAt = now;
    this.lastTime = currentTime;
  }

  /** Watched seconds since the last break (for debugging / UI). */
  get watchedSeconds(): number {
    return this.watched;
  }

  /**
   * Call on every `timeupdate`. Returns true when a break is due now.
   * `nearEnd` suppresses breaks in the last seconds of the video.
   */
  onTimeUpdate(currentTime: number, playing: boolean, playbackRate = 1, nearEnd = false, now: number = Date.now()): boolean {
    const delta = currentTime - this.lastTime;
    this.lastTime = currentTime;
    if (playing && delta > 0 && delta < MAX_PLAYBACK_STEP) {
      this.watched += delta / (playbackRate > 0 ? playbackRate : 1);
    }
    if (nearEnd || this.watched < this.opts.intervalSeconds || !this.gapOk(now)) return false;
    this.watched = 0;
    return true;
  }

  /** Call on every viewer-initiated `seeked`. Returns true when a break is due now. */
  onSeeked(currentTime: number, nearEnd = false, now: number = Date.now()): boolean {
    this.lastTime = currentTime;
    this.seeks = this.seeks.filter((t) => now - t < this.opts.seekWindowMs);
    this.seeks.push(now);
    if (nearEnd || this.seeks.length < this.opts.seekTriggerCount || !this.gapOk(now)) return false;
    this.seeks = [];
    return true;
  }

  /** Keep the playback baseline in sync after a seek we made ourselves. */
  syncTime(currentTime: number): void {
    this.lastTime = currentTime;
  }

  private gapOk(now: number): boolean {
    return now - this.lastBreakAt >= this.opts.minGapMs;
  }
}
