"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Search, X } from "lucide-react";

// Emoji panel for the comment composer. Phones have an emoji keyboard, desktops
// do not, so the composer brings its own. No dependency: the list is static.

const GROUPS: { key: string; icon: string; label: string; emojis: string }[] = [
  {
    key: "smileys",
    icon: "😀",
    label: "Kayfiyat",
    emojis:
      "😀 😃 😄 😁 😆 😅 🤣 😂 🙂 🙃 😉 😊 😇 🥰 😍 🤩 😘 😗 😚 😙 🥲 😋 😛 😜 🤪 😝 🤑 🤗 🤭 🤫 🤔 🤐 🤨 😐 😑 😶 😏 😒 🙄 😬 🤥 😌 😔 😪 🤤 😴 😷 🤒 🤕 🤢 🤮 🤧 🥵 🥶 🥴 😵 🤯 🤠 🥳 🥸 😎 🤓 🧐 😕 😟 🙁 😮 😯 😲 😳 🥺 😦 😧 😨 😰 😥 😢 😭 😱 😖 😣 😞 😓 😩 😫 🥱 😤 😡 😠 🤬 😈 👿 💀 ☠️ 💩 🤡 👹 👺 👻 👽 👾 🤖 😺 😸 😹 😻 😼 😽 🙀 😿 😾 🙈 🙉 🙊",
  },
  {
    key: "gestures",
    icon: "👍",
    label: "Imo-ishora",
    emojis:
      "👍 👎 👌 🤌 🤏 ✌️ 🤞 🤟 🤘 🤙 👈 👉 👆 👇 ☝️ ✋ 🤚 🖐️ 🖖 👋 🤝 🙏 👏 🙌 👐 🤲 ✍️ 💪 🦾 🫶 🫡 🫠 🫢 🫣 🤷 🤦 🙋 🙅 🙆 💁 🙇 🧎 🏃 💃 🕺 👀 👁️ 👂 👃 🧠 👄 💋 👶 🧒 👦 👧 🧑 👨 👩 🧓 👴 👵 👮 🕵️ 🥷 👑 🤴 👸 🦸 🦹 🧙 🧛 🧟 🧞 👼 🎅",
  },
  {
    key: "hearts",
    icon: "❤️",
    label: "Yurak",
    emojis:
      "❤️ 🧡 💛 💚 💙 💜 🖤 🤍 🤎 💔 ❤️‍🔥 ❤️‍🩹 ❣️ 💕 💞 💓 💗 💖 💘 💝 💟 💌 💯 💢 💥 💫 💦 💨 🕳️ 💤 ✨ ⭐ 🌟 🔥 ⚡ 🎉 🎊 🎈 🎁 🏆 🥇 🥈 🥉 🏅 🎖️ ✅ ❌ ❓ ❗ ‼️ ⁉️ ⚠️ 🚫 🔞 💬 🗯️ 💭 🔔 🔕",
  },
  {
    key: "cinema",
    icon: "🎬",
    label: "Kino",
    emojis:
      "🎬 🎥 📽️ 🎞️ 🍿 🎭 🎟️ 🎫 📺 📻 🎤 🎧 🎼 🎵 🎶 🎹 🥁 🎷 🎺 🎸 🎻 🎮 🕹️ 🎲 🎯 🎳 🎰 🧩 🪄 🔮 🎨 🖼️ 📸 📷 📹 💿 📀 🔦 💡 📖 📚 📝 ✏️ 🗡️ ⚔️ 🛡️ 🔫 💣 🧨 🪓 🏹 🚀 🛸 🧬 🕰️ ⏳ 🗝️ 🔒 🕯️ ⚰️ 🪦",
  },
  {
    key: "nature",
    icon: "🐶",
    label: "Tabiat",
    emojis:
      "🐶 🐱 🐭 🐹 🐰 🦊 🐻 🐼 🐨 🐯 🦁 🐮 🐷 🐸 🐵 🐔 🐧 🐦 🐤 🦆 🦅 🦉 🦇 🐺 🐗 🐴 🦄 🐝 🦋 🐌 🐞 🐜 🕷️ 🦂 🐢 🐍 🦎 🐙 🦑 🦐 🦀 🐠 🐟 🐬 🐳 🦈 🐊 🐅 🐆 🦓 🦍 🐘 🦛 🦏 🐪 🦒 🦘 🐃 🐑 🐐 🦌 🐕 🐈 🐓 🦚 🦜 🦢 🕊️ 🐇 🦔 🐉 🐲 🌵 🎄 🌲 🌳 🌴 🌱 🌿 🍀 🍁 🍂 🍃 🌷 🌹 🥀 🌺 🌸 🌼 🌻 🌞 🌝 🌚 🌙 ⭐ ☀️ ⛅ ☁️ 🌧️ ⛈️ 🌩️ ❄️ ☃️ 🌈 🌊 🌪️",
  },
  {
    key: "food",
    icon: "🍕",
    label: "Taom",
    emojis:
      "🍏 🍎 🍐 🍊 🍋 🍌 🍉 🍇 🍓 🫐 🍒 🍑 🥭 🍍 🥥 🥝 🍅 🍆 🥑 🥦 🥒 🌶️ 🌽 🥕 🧄 🧅 🥔 🍞 🥐 🥖 🧀 🥚 🍳 🥞 🧇 🥓 🥩 🍗 🍖 🌭 🍔 🍟 🍕 🥪 🌮 🌯 🥗 🍝 🍜 🍲 🍛 🍣 🍱 🥟 🍤 🍙 🍚 🍧 🍨 🍦 🥧 🧁 🍰 🎂 🍮 🍭 🍬 🍫 🍩 🍪 🥜 🍯 🥛 ☕ 🍵 🧃 🥤 🧋 🍶 🍺 🍻 🥂 🍷 🥃 🍸 🍹 🍾 🧊",
  },
  {
    key: "activity",
    icon: "⚽",
    label: "Sport",
    emojis:
      "⚽ 🏀 🏈 ⚾ 🥎 🎾 🏐 🏉 🥏 🎱 🏓 🏸 🏒 🏑 🥍 🏏 🥅 ⛳ 🏹 🎣 🤿 🥊 🥋 🎽 🛹 🛼 🛷 ⛸️ 🥌 🎿 ⛷️ 🏂 🏋️ 🤼 🤸 ⛹️ 🤺 🤾 🏌️ 🏇 🧘 🏄 🏊 🤽 🚣 🧗 🚵 🚴 🎪 🚗 🚕 🚙 🚌 🏎️ 🚓 🚑 🚒 🚚 🚜 🏍️ 🛵 🚲 ✈️ 🚁 🚂 🚢 ⛵ 🏠 🏰 🗽 🗼 🌋 🏝️ 🏔️ 🌍 🇺🇿",
  },
];

// Search index: words people actually type (Uzbek, Russian, English) -> emojis. Matching is by prefix of any word,
// so "kul", "смех" and "laugh" all find the laughing faces. Group names are searchable too.
const KEYWORDS: [string, string][] = [
  ["kulgi kulish kulmoq xursand quvonch tabassum smile laugh haha lol happy смех улыбка радость", "😀 😃 😄 😁 😆 😅 🤣 😂 🙂 😊 😇 😺 😸 😹"],
  ["sevgi muhabbat oshiq love kiss opich bosa любовь поцелуй", "🥰 😍 🤩 😘 😗 😚 😙 💋 😻 😽 ❤️ 💕 💞 💓 💗 💖 💘 💝 🫶"],
  ["yurak qalb heart сердце", "❤️ 🧡 💛 💚 💙 💜 🖤 🤍 🤎 💔 ❤️‍🔥 ❤️‍🩹 ❣️ 💕 💞 💓 💗 💖 💘 💝 💟"],
  ["yigi yiglash kozyosh xafa gamgin sad cry tears грусть слезы плач", "😢 😭 🥲 😥 😿 😔 😞 😟 🙁 😕 🥺 💔"],
  ["jahl achchiq gazab angry mad злой злость", "😤 😡 😠 🤬 😾 👿 💢"],
  ["qorqinch qorquv dahshat fear scared scream ужас страх", "😨 😰 😱 🙀 😧 😦 👻 💀 ☠️ 🧟 🧛 ⚰️ 🪦"],
  ["hayrat ajablanish shok wow surprise shock удивление шок", "😮 😯 😲 😳 🤯 😵 🙀 ⁉️ ‼️"],
  ["oylash fikr hmm think думать", "🤔 🤨 🧐 💭"],
  ["uyqu uxlash charchoq sleep tired сон устал", "😴 😪 🥱 💤 😫 😩"],
  ["kasal bemor sick ill болезнь", "😷 🤒 🤕 🤢 🤮 🤧 🥵 🥶 🥴"],
  ["zor ajoyib cool krutoy круто", "😎 🤩 🔥 💯 👌 👍 🤘 🤙"],
  ["masxara hazil til tongue joke шутка", "😋 😛 😜 🤪 😝 🤡 🙃 😏"],
  ["uyat sir jim secret quiet тихо секрет", "🤫 🤭 🤐 🫢 🫣 🙈 🙉 🙊"],
  ["bayram tabrik party celebrate праздник", "🥳 🎉 🎊 🎈 🎁 🎂 🍾 🥂 ✨"],
  ["pul boy money деньги", "🤑 💰 💵 💸 💳 🪙"],
  ["iblis shayton devil дьявол", "😈 👿 👹 👺"],
  ["mushuk cat кот кошка", "😺 😸 😹 😻 😼 😽 🙀 😿 😾 🐱 🐈"],
  ["maymun monkey обезьяна", "🙈 🙉 🙊 🐵 🦍"],
  ["robot begona alien робот", "🤖 👽 👾 🛸"],
  ["layk yaxshi maqul ok like yes да хорошо лайк", "👍 👌 ✅ 🙆 🤝 💯"],
  ["yomon yoq dislike no нет плохо", "👎 ❌ 🙅 🚫"],
  ["qarsak ofarin barakalla clap bravo аплодисменты браво", "👏 🙌 🎉 🏆"],
  ["salom xayr hello hi bye привет пока", "👋 🤚 ✋ 🖐️ 🫡"],
  ["duo iltimos rahmat tashakkur thanks please pray спасибо пожалуйста", "🙏 🤲 🫶 ❤️"],
  ["kuch muskul strong sila сила", "💪 🦾 🏋️ 🥊"],
  ["qol barmoq hand finger рука палец", "👈 👉 👆 👇 ☝️ ✌️ 🤞 🤟 🤘 🤙 🤌 🤏 👊 ✊ ✋"],
  ["koz qarash eyes look глаза", "👀 👁️ 🧐 🫣"],
  ["raqs dance танец", "💃 🕺 🎶 🎵"],
  ["yugurish qochish run бег", "🏃 💨"],
  ["bilmadim shrug незнаю", "🤷 🤔"],
  ["facepalm voy", "🤦 😑 🙄"],
  ["olov yongin fire hot огонь", "🔥 ❤️‍🔥 🥵 🌶️ 💥 🧨"],
  ["yulduz star звезда", "⭐ 🌟 ✨ 💫 🤩"],
  ["yuz foiz 100", "💯"],
  ["savol undov question вопрос", "❓ ❗ ‼️ ⁉️"],
  ["ogoh xavf taqiq warning stop внимание", "⚠️ 🚫 🔞 ❌"],
  ["galaba kubok medal sovrin win trophy победа кубок", "🏆 🥇 🥈 🥉 🏅 🎖️ 👑"],
  ["toj qirol shoh malika king queen crown король корона", "👑 🤴 👸"],
  ["kino film movie cinema кино фильм", "🎬 🎥 📽️ 🎞️ 🍿 🎭 🎟️ 🎫 📺"],
  ["popkorn popcorn попкорн", "🍿"],
  ["serial tv televizor", "📺 🎬 🍿"],
  ["musiqa qoshiq music song музыка песня", "🎤 🎧 🎼 🎵 🎶 🎹 🥁 🎷 🎺 🎸 🎻"],
  ["oyin game игра", "🎮 🕹️ 🎲 🎯 🎳 🎰 🧩"],
  ["sehr magic магия", "🪄 🔮 🧙 ✨ 🧞"],
  ["jang qurol urush fight weapon war оружие бой", "🗡️ ⚔️ 🛡️ 🔫 💣 🧨 🪓 🏹 🥷 🥊"],
  ["kosmos raketa space rocket космос ракета", "🚀 🛸 👽 🌍 🌙 ⭐"],
  ["qahramon superhero hero герой", "🦸 🦹 🥷 💪"],
  ["detektiv politsiya police полиция", "🕵️ 👮 🚓 🔦 🗝️"],
  ["vaqt soat time время", "🕰️ ⏳"],
  ["kitob oqish book книга", "📖 📚 📝 ✏️"],
  ["rasm foto kamera photo camera фото", "📸 📷 📹 🎨 🖼️"],
  ["it kuchuk dog собака", "🐶 🐕 🐺"],
  ["hayvon animal животное", "🐶 🐱 🐭 🐹 🐰 🦊 🐻 🐼 🐨 🐯 🦁 🐮 🐷 🐸 🐵"],
  ["sher yolbars lion tiger лев тигр", "🦁 🐯 🐅 🐆"],
  ["ayiq bear медведь", "🐻 🐼 🐨"],
  ["qush bird птица", "🐔 🐧 🐦 🐤 🦆 🦅 🦉 🦚 🦜 🦢 🕊️"],
  ["ot horse лошадь", "🐴 🦄 🏇"],
  ["ilon snake змея", "🐍 🐉 🐲"],
  ["ajdar dragon дракон", "🐉 🐲"],
  ["baliq fish рыба", "🐠 🐟 🐬 🐳 🦈 🐙"],
  ["hasharot ari kapalak bug bee butterfly насекомое", "🐝 🦋 🐌 🐞 🐜 🕷️ 🦂"],
  ["gul flower цветок", "🌷 🌹 🥀 🌺 🌸 🌼 🌻"],
  ["daraxt orgmon tree дерево", "🌵 🎄 🌲 🌳 🌴 🌱 🌿 🍀 🍁 🍂 🍃"],
  ["quyosh sun солнце", "🌞 ☀️ ⛅"],
  ["oy tun moon night луна ночь", "🌝 🌚 🌙 ⭐"],
  ["yomgir bulut rain cloud дождь", "☁️ 🌧️ ⛈️ 🌩️ 🌈 💦"],
  ["qor qish snow winter снег зима", "❄️ ☃️ 🥶 🎄"],
  ["suv dengiz tolqin water sea вода море", "🌊 💦 🏄 🏊 🏝️"],
  ["meva fruit фрукт", "🍏 🍎 🍐 🍊 🍋 🍌 🍉 🍇 🍓 🫐 🍒 🍑 🥭 🍍 🥥 🥝"],
  ["olma apple яблоко", "🍏 🍎"],
  ["tarvuz qovun watermelon арбуз", "🍉"],
  ["sabzavot vegetable овощ", "🍅 🍆 🥑 🥦 🥒 🌶️ 🌽 🥕 🧄 🧅 🥔"],
  ["non bread хлеб", "🍞 🥐 🥖"],
  ["gosht kabob meat мясо", "🥩 🍗 🍖 🥓"],
  ["ovqat taom fastfood food еда", "🍔 🍟 🍕 🌭 🥪 🌮 🌯 🥗 🍝 🍜 🍲 🍛 🍣 🍱 🥟"],
  ["pitsa pizza пицца", "🍕"],
  ["burger gamburger бургер", "🍔 🍟"],
  ["osh palov guruch rice плов рис", "🍛 🍚 🍲"],
  ["shirinlik tort muzqaymoq sweet cake ice cream торт сладкое", "🍧 🍨 🍦 🥧 🧁 🍰 🎂 🍮 🍭 🍬 🍫 🍩 🍪 🍯"],
  ["choy qahva kofe tea coffee чай кофе", "☕ 🍵"],
  ["ichimlik sharbat drink напиток", "🥛 🧃 🥤 🧋 🍺 🍻 🥂 🍷 🥃 🍸 🍹 🍾 🧊"],
  ["futbol football soccer футбол", "⚽ 🥅 🏆"],
  ["basketbol basketball", "🏀 ⛹️"],
  ["sport", "⚽ 🏀 🏈 ⚾ 🎾 🏐 🏓 🏸 🥊 🥋 🏋️ 🤸 🚴 🏊"],
  ["boks kurash box борьба", "🥊 🥋 🤼"],
  ["mashina avto car машина", "🚗 🚕 🚙 🏎️ 🚓 🚑 🚒 🚚"],
  ["samolyot uchish plane самолет", "✈️ 🚁 🚀"],
  ["uy home дом", "🏠 🏰"],
  ["ozbekiston uzbekistan bayroq flag узбекистан", "🇺🇿"],
  ["dunyo yer world earth мир земля", "🌍"],
  ["tog mountain гора", "🏔️ 🌋"],
  ["xabar gap chat message сообщение", "💬 🗯️ 💭 💌"],
  ["qongiroq bildirishnoma bell notification", "🔔 🔕"],
  ["sovga gift подарок", "🎁 💝"],
  ["bola chaqaloq baby ребенок", "👶 🧒 👦 👧"],
  ["odam erkak ayol man woman человек", "🧑 👨 👩 🧓 👴 👵"],
  ["najas poop", "💩"],
  ["bosh suyak olim skull death смерть череп", "💀 ☠️ ⚰️ 🪦"],
  ["arvoh ghost призрак", "👻"],
  ["qorbobo yangi yil santa new year новый год", "🎅 🎄 ☃️ 🎁 🎉"],
];

/** Lowercases and drops apostrophes so "qo'rqinch", "qo‘rqinch" and "qorqinch" are the same word. */
const fold = (text: string) => text.toLowerCase().replace(/['‘’ʻʼ`]/g, "").trim();

function searchEmojis(query: string): string[] {
  const q = fold(query);
  if (!q) return [];
  const found = new Set<string>();
  // A pasted emoji finds itself.
  for (const g of GROUPS) for (const e of g.emojis.split(" ")) if (e === query.trim()) found.add(e);
  for (const [words, emojis] of KEYWORDS) {
    if (words.split(" ").some((w) => w.startsWith(q))) emojis.split(" ").forEach((e) => found.add(e));
  }
  for (const g of GROUPS) {
    if (fold(g.label).startsWith(q)) g.emojis.split(" ").forEach((e) => found.add(e));
  }
  return Array.from(found).slice(0, 96);
}

const RECENT_KEY = "comment_recent_emojis";

function loadRecent(): string[] {
  try {
    const raw = JSON.parse(localStorage.getItem(RECENT_KEY) || "[]");
    return Array.isArray(raw) ? raw.filter((x) => typeof x === "string").slice(0, 24) : [];
  } catch {
    return [];
  }
}

export default function EmojiPicker({
  onPick,
  onClose,
  className = "absolute bottom-full left-0 z-30 mb-2 w-[min(22rem,calc(100vw-3rem))]",
  cols = 8,
}: {
  onPick: (emoji: string) => void;
  onClose: () => void;
  /** Position and width of the panel relative to its (relative) parent. */
  className?: string;
  /** Emojis per row; fewer for a narrow panel such as the watch-room chat. */
  cols?: 6 | 8;
}) {
  const [group, setGroup] = useState(GROUPS[0].key);
  const [recent, setRecent] = useState<string[]>([]);
  const [query, setQuery] = useState("");
  const results = useMemo(() => searchEmojis(query), [query]);
  const searching = fold(query) !== "";
  const grid = cols === 6 ? "grid grid-cols-6 gap-0.5" : "grid grid-cols-8 gap-0.5";
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => setRecent(loadRecent()), []);

  useEffect(() => {
    const onDown = (e: MouseEvent) => {
      // The button that opened the panel toggles it itself.
      if ((e.target as Element).closest?.("[data-picker-toggle]")) return;
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [onClose]);

  const pick = (emoji: string) => {
    onPick(emoji);
    const next = [emoji, ...recent.filter((e) => e !== emoji)].slice(0, 24);
    setRecent(next);
    try {
      localStorage.setItem(RECENT_KEY, JSON.stringify(next));
    } catch {
      // private mode: recents just do not persist
    }
  };

  const current = GROUPS.find((g) => g.key === group) || GROUPS[0];

  const cell = (e: string, key: string) => (
    <button key={key} type="button" onClick={() => pick(e)} className="rounded-md py-1 text-xl leading-none hover:bg-white/10 font-emoji">
      {e}
    </button>
  );

  return (
    <div ref={ref} className={`${className} overflow-hidden rounded-xl border border-white/10 bg-brand-dark shadow-2xl`}>
      <div className="flex items-center gap-2 border-b border-white/10 px-2 py-1.5">
        <Search className="h-4 w-4 shrink-0 text-gray-400" />
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Emoji qidirish… (kulgi, yurak, kino)"
          aria-label="Emoji qidirish"
          className="min-w-0 flex-1 bg-transparent text-sm text-white placeholder-gray-500 focus:outline-none"
        />
        {query && (
          <button type="button" onClick={() => setQuery("")} aria-label="Qidiruvni tozalash" className="shrink-0 p-0.5 text-gray-400 hover:text-white">
            <X className="h-4 w-4" />
          </button>
        )}
      </div>
      {!searching && (
        <div className="flex border-b border-white/10" role="tablist" aria-label="Emoji turlari">
          {GROUPS.map((g) => (
            <button
              key={g.key}
              type="button"
              role="tab"
              aria-selected={g.key === group}
              title={g.label}
              onClick={() => setGroup(g.key)}
              className={`flex-1 py-2 text-lg leading-none transition font-emoji ${g.key === group ? "bg-white/10" : "opacity-60 hover:opacity-100"}`}
            >
              {g.icon}
            </button>
          ))}
        </div>
      )}
      <div className="max-h-56 overflow-y-auto p-2">
        {searching ? (
          results.length > 0 ? (
            <div className={grid}>{results.map((e) => cell(e, `s-${e}`))}</div>
          ) : (
            <p className="py-6 text-center text-xs text-gray-500">Hech narsa topilmadi. Boshqa so&apos;z bilan urinib ko&apos;ring.</p>
          )
        ) : (
          <>
            {recent.length > 0 && (
              <>
                <p className="px-1 pb-1 text-[10px] uppercase tracking-wide text-gray-500">Oxirgi ishlatilgan</p>
                <div className={`mb-2 ${grid}`}>{recent.map((e) => cell(e, `r-${e}`))}</div>
              </>
            )}
            <p className="px-1 pb-1 text-[10px] uppercase tracking-wide text-gray-500">{current.label}</p>
            <div className={grid}>{current.emojis.split(" ").map((e) => cell(e, e))}</div>
          </>
        )}
      </div>
    </div>
  );
}
