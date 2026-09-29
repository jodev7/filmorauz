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
