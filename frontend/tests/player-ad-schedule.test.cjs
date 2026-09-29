const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const ts = require('typescript');
const vm = require('node:vm');
const code = ts.transpileModule(fs.readFileSync(path.join(__dirname, '../src/lib/player-ad-schedule.ts'), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;
const context = { exports: {} };
vm.runInNewContext(code, context);
const { PlayerAdSchedule } = context.exports;
const ad = { id: 'promo', player_ad_interval_minutes: 1, player_ad_max_repeats: 2 };
function watch(schedule, start, seconds, rate = 1) {
  for (let i = 1; i <= seconds; i++) schedule.tick(start + i * rate, true, false, rate);
}
test('repeats at the configured interval and stops at the limit', () => {
  const s = new PlayerAdSchedule();
  watch(s, 0, 59);
  assert.equal(s.due([ad]).length, 0);
  watch(s, 59, 1);
  assert.equal(s.due([ad]).length, 1);
  s.shown(ad.id);
  watch(s, 60, 59);
  assert.equal(s.due([ad]).length, 0);
  watch(s, 119, 1);
  assert.equal(s.due([ad]).length, 1);
  s.shown(ad.id);
  watch(s, 120, 600);
  assert.equal(s.due([ad]).length, 0);
});
test('pause, forward/backward seeking and restored progress do not count', () => {
  const s = new PlayerAdSchedule();
  s.syncTime(1000);
  watch(s, 1000, 30);
  s.tick(1031, false, false);
  s.tick(3000, true, true);
  s.syncTime(20);
  watch(s, 20, 29);
  assert.equal(s.due([ad]).length, 0);
  watch(s, 49, 1);
  assert.equal(s.due([ad]).length, 1);
});
test('2x playback uses watched wall time, not content timestamp', () => {
  const s = new PlayerAdSchedule();
  watch(s, 0, 30, 2);
  assert.equal(s.due([ad]).length, 0);
  watch(s, 60, 30, 2);
  assert.equal(s.due([ad]).length, 1);
});
test('legacy records default to ten minutes with unlimited repeats', () => {
  const s = new PlayerAdSchedule();
  const legacy = { id: 'legacy' };
  for (let n = 0; n < 5; n++) {
    watch(s, n * 600, 599);
    assert.equal(s.due([legacy]).length, 0);
    watch(s, n * 600 + 599, 1);
    assert.equal(s.due([legacy]).length, 1);
    s.shown(legacy.id);
  }
});
test('a new content session resets the repeat limit', () => {
  const s = new PlayerAdSchedule();
  watch(s, 0, 60);
  assert.equal(s.due([ad]).length, 1);
});
test('campaigns keep independent due times and repeat limits', () => {
  const s = new PlayerAdSchedule();
  const slow = { id: 'slow', player_ad_interval_minutes: 2, player_ad_max_repeats: 1 };
  watch(s, 0, 60);
  assert.equal(s.due([ad, slow]).length, 1);
  s.shown(ad.id);
  watch(s, 60, 60);
  assert.equal(s.due([ad, slow]).length, 2);
  s.shown(ad.id);
  watch(s, 120, 60);
  assert.equal(s.due([ad, slow])[0].id, slow.id);
});
