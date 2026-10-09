"use client";

import { useEffect, useRef, useState } from "react";

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
  className = "absolute bottom-full left-0 z-30 mb-2",
}: {
  onPick: (emoji: string) => void;
  onClose: () => void;
  /** Position of the panel relative to its (relative) parent. */
  className?: string;
}) {
  const [group, setGroup] = useState(GROUPS[0].key);
  const [recent, setRecent] = useState<string[]>([]);
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

  return (
    <div ref={ref} className={`${className} w-[min(22rem,calc(100vw-3rem))] overflow-hidden rounded-xl border border-white/10 bg-brand-dark shadow-2xl`}>
      <div className="flex border-b border-white/10" role="tablist" aria-label="Emoji turlari">
        {GROUPS.map((g) => (
          <button
            key={g.key}
            type="button"
            role="tab"
            aria-selected={g.key === group}
            title={g.label}
            onClick={() => setGroup(g.key)}
            className={`flex-1 py-2 text-lg leading-none transition ${g.key === group ? "bg-white/10" : "opacity-60 hover:opacity-100"}`}
          >
            {g.icon}
          </button>
        ))}
      </div>
      <div className="max-h-56 overflow-y-auto p-2">
        {recent.length > 0 && (
          <>
            <p className="px-1 pb-1 text-[10px] uppercase tracking-wide text-gray-500">Oxirgi ishlatilgan</p>
            <div className="mb-2 grid grid-cols-8 gap-0.5">
              {recent.map((e) => (
                <button key={`r-${e}`} type="button" onClick={() => pick(e)} className="rounded-md py-1 text-xl leading-none hover:bg-white/10">
                  {e}
                </button>
              ))}
            </div>
          </>
        )}
        <p className="px-1 pb-1 text-[10px] uppercase tracking-wide text-gray-500">{current.label}</p>
        <div className="grid grid-cols-8 gap-0.5">
          {current.emojis.split(" ").map((e) => (
            <button key={e} type="button" onClick={() => pick(e)} className="rounded-md py-1 text-xl leading-none hover:bg-white/10">
              {e}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
