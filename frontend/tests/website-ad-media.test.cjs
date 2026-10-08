const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const ts = require('typescript');
const vm = require('node:vm');

const code = ts.transpileModule(fs.readFileSync(path.join(__dirname, '../src/lib/website-ad-media.ts'), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText;
const context = { exports: {} };
vm.runInNewContext(code, context);
const { getWebsiteAdMedia } = context.exports;

test('one banner creative does not fill adjacent inline or popup slots', () => {
  const ad = { banner_media_url: '/banner.webp', banner_media_type: 'image' };
  assert.equal(getWebsiteAdMedia(ad, 'banner').url, '/banner.webp');
  assert.equal(getWebsiteAdMedia(ad, 'inline'), null);
  assert.equal(getWebsiteAdMedia(ad, 'popup'), null);
});

test('configured variants use only their own creative and type', () => {
  const ad = {
    banner_media_url: '/banner.webp', inline_media_url: '/inline.webp',
    popup_media_url: '/popup.webm', popup_media_type: 'video',
  };
  assert.equal(getWebsiteAdMedia(ad, 'banner').url, '/banner.webp');
  assert.equal(getWebsiteAdMedia(ad, 'inline').url, '/inline.webp');
  assert.equal(getWebsiteAdMedia(ad, 'card').url, '/inline.webp');
  assert.equal(getWebsiteAdMedia(ad, 'popup').type, 'video');
});

test('old image-only ads remain visible as banners alone', () => {
  const ad = { image_url: '/legacy.jpg' };
  assert.equal(getWebsiteAdMedia(ad, 'banner').url, '/legacy.jpg');
  assert.equal(getWebsiteAdMedia(ad, 'inline'), null);
  assert.equal(getWebsiteAdMedia(ad, 'popup'), null);
});

test('phones get the mobile creative only when the slot has a desktop one', () => {
  const ad = {
    banner_media_url: '/banner.webp', banner_mobile_media_url: '/banner-m.webp',
    inline_mobile_media_url: '/inline-m.webp',
    fixed_bottom_media_url: '/bottom.mp4', fixed_bottom_media_type: 'video',
  };
  assert.equal(getWebsiteAdMedia(ad, 'banner', true).url, '/banner-m.webp');
  assert.equal(getWebsiteAdMedia(ad, 'banner', true).mobile, true);
  assert.equal(getWebsiteAdMedia(ad, 'banner', false).url, '/banner.webp');
  // A mobile image alone does not switch a slot on.
  assert.equal(getWebsiteAdMedia(ad, 'inline', true), null);
  // No mobile image: the desktop creative (and its type) is used on phones.
  assert.equal(getWebsiteAdMedia(ad, 'fixed_bottom', true).type, 'video');
  assert.equal(getWebsiteAdMedia(ad, 'fixed_bottom', true).mobile, false);
});
