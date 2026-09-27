// Preview of Telegram's HTML parse mode: everything is escaped, then only the
// tags Telegram supports are turned back on (links only for http/https).

const SIMPLE = "b|strong|i|em|u|ins|s|strike|del|code|pre|blockquote";

export function renderTelegramHtml(text: string): string {
  let s = text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
  s = s.replace(new RegExp(`&lt;(/?)(${SIMPLE})&gt;`, "gi"), "<$1$2>");
  s = s.replace(/&lt;a href=&quot;(https?:\/\/(?:(?!&quot;)[^\s])*)&quot;&gt;/gi, '<a href="$1" target="_blank" rel="noopener noreferrer">');
  s = s.replace(/&lt;\/a&gt;/gi, "</a>");
  s = s.replace(/&lt;tg-spoiler&gt;/gi, '<span class="tg-spoiler">').replace(/&lt;\/tg-spoiler&gt;/gi, "</span>");
  return s.replace(/\n/g, "<br/>");
}

/** Visible length as Telegram counts it (tags don't count). */
export function telegramTextLength(text: string): number {
  return text.replace(/<[^>]+>/g, "").replace(/&(amp|lt|gt|quot);/g, "x").length;
}
