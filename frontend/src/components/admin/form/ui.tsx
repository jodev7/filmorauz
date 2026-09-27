"use client";

import { ReactNode, useEffect } from "react";
import { Loader2, Save } from "lucide-react";

// Shared building blocks for admin content forms (movie / series).

export const inputCls =
  "w-full rounded-xl border border-white/10 bg-black/30 px-3.5 py-2.5 text-sm text-white placeholder-gray-600 transition-colors focus:border-orange-500 focus:outline-none focus:ring-2 focus:ring-orange-500/20 disabled:opacity-60";

export function FormSection({
  title,
  description,
  icon,
  actions,
  children,
  id,
}: {
  title: string;
  description?: string;
  icon?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  id?: string;
}) {
  return (
    <section id={id} className="scroll-mt-24 rounded-2xl border border-white/10 bg-[#12121a] p-5 sm:p-6">
      <header className="mb-5 flex items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          {icon && <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-orange-500/10 text-orange-400">{icon}</span>}
          <div>
            <h2 className="font-semibold text-white">{title}</h2>
            {description && <p className="mt-0.5 text-xs text-gray-500">{description}</p>}
          </div>
        </div>
        {actions}
      </header>
      <div className="space-y-4">{children}</div>
    </section>
  );
}

export function Field({
  label,
  required,
  hint,
  error,
  htmlFor,
  right,
  children,
  className = "",
}: {
  label: string;
  required?: boolean;
  hint?: ReactNode;
  error?: string;
  htmlFor?: string;
  right?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={className}>
      <div className="mb-1.5 flex items-center justify-between gap-2">
        <label htmlFor={htmlFor} className="text-xs font-medium text-gray-400">
          {label} {required && <span className="text-orange-400">*</span>}
        </label>
        {right}
      </div>
      {children}
      {error ? <p className="mt-1 text-xs text-red-400">{error}</p> : hint ? <p className="mt-1 text-xs text-gray-500">{hint}</p> : null}
    </div>
  );
}

/** Row of mutually exclusive options (quality, type…). */
export function Segmented<T extends string>({
  value,
  options,
  onChange,
  ariaLabel,
}: {
  value: T;
  options: { value: T; label: ReactNode; disabled?: boolean; title?: string }[];
  onChange: (v: T) => void;
  ariaLabel: string;
}) {
  return (
    <div role="radiogroup" aria-label={ariaLabel} className="flex flex-wrap gap-1 rounded-xl border border-white/10 bg-black/30 p-1">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
          disabled={o.disabled}
          title={o.title}
          onClick={() => onChange(o.value)}
          className={`flex-1 whitespace-nowrap rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
            value === o.value
              ? "bg-orange-500 text-white shadow"
              : o.disabled
                ? "cursor-not-allowed text-gray-700"
                : "text-gray-400 hover:bg-white/5 hover:text-white"
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function SwitchRow({
  checked,
  onChange,
  title,
  description,
  icon,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  title: string;
  description?: string;
  icon?: ReactNode;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={`flex w-full items-center gap-3 rounded-xl border px-4 py-3 text-left transition-colors ${
        checked ? "border-yellow-500/40 bg-yellow-500/[0.07]" : "border-white/10 bg-black/20 hover:border-white/20"
      }`}
    >
      {icon}
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium text-white">{title}</span>
        {description && <span className="block text-xs text-gray-500">{description}</span>}
      </span>
      <span className={`inline-flex h-6 w-11 shrink-0 items-center rounded-full p-0.5 transition-colors ${checked ? "bg-yellow-500" : "bg-white/15"}`}>
        <span className={`h-5 w-5 rounded-full bg-white shadow transition-transform ${checked ? "translate-x-5" : ""}`} />
      </span>
    </button>
  );
}

/** Bottom bar with dirty state + save; Ctrl/⌘+S submits the form. */
export function StickySaveBar({
  formId,
  dirty,
  saving,
  disabled,
  label = "Saqlash",
  status,
  secondary,
}: {
  formId: string;
  dirty: boolean;
  saving: boolean;
  disabled?: boolean;
  label?: string;
  status?: ReactNode;
  secondary?: ReactNode;
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        (document.getElementById(formId) as HTMLFormElement | null)?.requestSubmit();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [formId]);

  return (
    <div className="sticky bottom-0 z-20 -mx-4 mt-6 border-t border-white/10 bg-[#0b0b12]/90 px-4 py-3 backdrop-blur sm:-mx-8 sm:px-8">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0 text-xs">
          {status ?? (
            <span className={`inline-flex items-center gap-1.5 ${dirty ? "text-amber-300" : "text-gray-500"}`}>
              <span className={`h-2 w-2 rounded-full ${dirty ? "bg-amber-400" : "bg-emerald-500"}`} />
              {dirty ? "Saqlanmagan o'zgarishlar bor" : "Hammasi saqlangan"}
              <kbd className="ml-2 hidden rounded border border-white/15 px-1 text-[10px] text-gray-500 sm:inline">Ctrl+S</kbd>
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {secondary}
          <button
            type="submit"
            form={formId}
            disabled={saving || disabled}
            className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-6 py-2.5 text-sm font-semibold text-white shadow-lg shadow-orange-500/20 transition-colors hover:bg-orange-600 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {saving ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
            {saving ? "Saqlanmoqda…" : label}
          </button>
        </div>
      </div>
    </div>
  );
}

/** Warns before closing the tab with unsaved changes or uploads running. */
export function useLeaveGuard(active: boolean) {
  useEffect(() => {
    if (!active) return;
    const h = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", h);
    return () => window.removeEventListener("beforeunload", h);
  }, [active]);
}

export function ErrorBanner({ message }: { message?: string }) {
  if (!message) return null;
  return (
    <div role="alert" className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">
      {message}
    </div>
  );
}

export function SavingSpinner() {
  return <Loader2 size={16} className="animate-spin" />;
}
