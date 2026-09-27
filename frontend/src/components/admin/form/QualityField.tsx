"use client";

import { HardDrive } from "lucide-react";
import { Field, Segmented } from "./ui";
import { QUALITIES, bestQuality, qualityOptions } from "./constants";

/**
 * Quality picker. When renditions already exist in storage (B2), the best
 * one is selected automatically and higher options are disabled — the
 * label can't promise a quality that was never uploaded.
 */
export default function QualityField({
  value,
  onChange,
  storage,
}: {
  value: string;
  onChange: (v: string) => void;
  storage: string[];
}) {
  const best = bestQuality(storage);
  const cap = best ? QUALITIES.indexOf(best) : -1;
  const options = qualityOptions(value).map((o) => {
    const i = QUALITIES.indexOf(o.value);
    const over = cap >= 0 && i > cap;
    return { ...o, disabled: over, title: over ? "Bu sifat storage'da yo'q" : undefined };
  });

  return (
    <Field
      label="Sifat"
      hint={
        storage.length ? (
          <span>
            <HardDrive size={11} className="mr-1 inline -mt-0.5 text-emerald-400" />
            Storage&apos;da: {storage.join(" · ")} — eng yuqorisi avtomatik tanlandi
          </span>
        ) : (
          "Video hali storage'ga yuklanmagan — sifatni qo'lda tanlang"
        )
      }
    >
      <Segmented ariaLabel="Sifat" value={value || best || "1080p"} options={options} onChange={onChange} />
    </Field>
  );
}
