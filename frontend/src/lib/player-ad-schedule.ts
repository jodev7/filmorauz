export interface PlayerAdTiming {
  id: string;
  player_ad_interval_minutes?: number;
  player_ad_max_repeats?: number;
}

/** Per-content session. Only actual playback counts; seeking never adds time. */
export class PlayerAdSchedule {
  private watched = 0;
  private lastTime = 0;
  private lastBreak = 0;
  private repeats = new Map<string, number>();
  private lastShown = new Map<string, number>();

  syncTime(currentTime: number) { this.lastTime = currentTime; }

  tick(currentTime: number, playing: boolean, seeking: boolean, rate = 1) {
    const delta = currentTime - this.lastTime;
    this.lastTime = currentTime;
    if (playing && !seeking && delta > 0 && delta <= 2.5 * Math.max(1, rate)) {
      this.watched += delta / Math.max(0.25, rate);
    }
  }

  due<T extends PlayerAdTiming>(ads: T[]): T[] {
    // Space competing campaigns apart; each still keeps its own due time.
    const gap = Math.min(...ads.map((ad) => (ad.player_ad_interval_minutes || 10) * 60));
    if (this.watched - this.lastBreak < gap) return [];
    return ads.filter((ad) => {
      const interval = (ad.player_ad_interval_minutes || 10) * 60;
      const limit = ad.player_ad_max_repeats || 0;
      return this.watched - (this.lastShown.get(ad.id) || 0) >= interval &&
        (limit === 0 || (this.repeats.get(ad.id) || 0) < limit);
    });
  }

  shown(id: string) {
    this.repeats.set(id, (this.repeats.get(id) || 0) + 1);
    this.lastBreak = this.watched;
    this.lastShown.set(id, this.watched);
  }
}
