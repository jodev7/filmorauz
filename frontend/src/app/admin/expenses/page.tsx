"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Wallet, Plus, Trash2, Loader2, Server, Bot, RefreshCw } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  adminGetExpenseSummary,
  adminCreateExpense,
  adminDeleteExpense,
  ExpenseSummary,
} from "@/lib/api";

// Preset categories offered in the form (free-form "other" still allowed).
const CATEGORIES = [
  { value: "vps", label: "VPS / Server" },
  { value: "domain", label: "Domen" },
  { value: "marketing", label: "Marketing" },
  { value: "ai", label: "AI / API" },
  { value: "other", label: "Boshqa" },
];

function fmtUSD(n: number): string {
  return "$" + (n || 0).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

function fmtDate(iso: string): string {
  if (!iso) return "—";
  try {
    return new Intl.DateTimeFormat("uz-UZ", { year: "numeric", month: "short", day: "numeric" }).format(new Date(iso));
  } catch {
    return iso;
  }
}

export default function AdminExpensesPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();

  const [summary, setSummary] = useState<ExpenseSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  // Form state
  const [category, setCategory] = useState("vps");
  const [title, setTitle] = useState("");
  const [amount, setAmount] = useState("");
  const [recurring, setRecurring] = useState(false);
  const [incurredAt, setIncurredAt] = useState("");
  const [note, setNote] = useState("");

  useEffect(() => {
    if (!authLoading && (!token || user?.role !== "superadmin")) {
      router.replace("/admin/dashboard");
    }
  }, [authLoading, token, user, router]);

  const load = useCallback(async () => {
    if (!token) return;
    try {
      const data = await adminGetExpenseSummary(token);
      setSummary(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Yuklashda xatolik");
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    load();
  }, [load]);

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;
    const amt = parseFloat(amount);
    if (!title.trim() || !(amt > 0)) {
      setError("Nomi va musbat summa kiritilishi shart");
      return;
    }
    setSubmitting(true);
    setError("");
    try {
      await adminCreateExpense(token, {
        category,
        title: title.trim(),
        amount_usd: amt,
        recurring,
        note: note.trim(),
        incurred_at: incurredAt ? new Date(incurredAt).toISOString() : undefined,
      });
      setTitle("");
      setAmount("");
      setNote("");
      setRecurring(false);
      setIncurredAt("");
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Saqlashda xatolik");
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!token || !confirm("Bu xarajatni o'chirishni tasdiqlaysizmi?")) return;
    try {
      await adminDeleteExpense(token, id);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "O'chirishda xatolik");
    }
  };

  if (authLoading || (user && user.role !== "superadmin")) {
    return null;
  }

  return (
    <div className="p-4 sm:p-8 max-w-5xl">
      <div className="flex items-center justify-between mb-6 sm:mb-8">
        <div className="flex items-center gap-3">
          <Wallet className="text-brand-red" size={24} />
          <div>
            <h1 className="text-xl sm:text-2xl font-bold text-white">Xarajatlar</h1>
            <p className="text-gray-500 text-sm mt-0.5">Loyiha umumiy xarajatlari (faqat superadmin)</p>
          </div>
        </div>
        <button
          onClick={load}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-brand-border text-xs text-gray-400 hover:text-white hover:border-gray-500 transition-colors"
        >
          <RefreshCw size={13} /> Yangilash
        </button>
      </div>

      {error && (
        <div className="mb-4 rounded-lg border border-red-500/30 bg-red-500/5 px-4 py-2.5 text-sm text-red-300">
          {error}
        </div>
      )}

      {loading ? (
        <div className="flex items-center gap-2 text-gray-500 py-12 justify-center">
          <Loader2 size={18} className="animate-spin" /> Yuklanmoqda...
        </div>
      ) : (
        <>
          {/* Summary cards */}
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-6">
            <div className="bg-brand-card border border-brand-border rounded-xl p-5">
              <p className="text-xs text-gray-500">Umumiy xarajat</p>
              <p className="text-3xl font-bold text-emerald-400 mt-1">{fmtUSD(summary?.grand_total || 0)}</p>
            </div>
            <div className="bg-brand-card border border-brand-border rounded-xl p-5">
              <div className="flex items-center gap-1.5 text-xs text-gray-500">
                <Bot size={13} /> AI klip xarajati
              </div>
              <p className="text-2xl font-semibold text-white mt-1">{fmtUSD(summary?.ai_clip_cost || 0)}</p>
              <p className="text-[11px] text-gray-600 mt-1">Avtomatik hisoblanadi (Gemini)</p>
            </div>
            <div className="bg-brand-card border border-brand-border rounded-xl p-5">
              <div className="flex items-center gap-1.5 text-xs text-gray-500">
                <Server size={13} /> Qo&apos;lda kiritilgan
              </div>
              <p className="text-2xl font-semibold text-white mt-1">{fmtUSD(summary?.manual_total || 0)}</p>
            </div>
          </div>

          {/* Category breakdown */}
          {summary && summary.categories.length > 0 && (
            <div className="mb-6 flex flex-wrap gap-2">
              {summary.categories.map((c) => (
                <span
                  key={c.category}
                  className="inline-flex items-center gap-2 rounded-full border border-brand-border bg-brand-card px-3 py-1.5 text-xs text-gray-300"
                >
                  <span className="uppercase tracking-wide text-gray-500">{c.category}</span>
                  <span className="text-white font-medium">{fmtUSD(c.amount_usd)}</span>
                  <span className="text-gray-600">({c.count})</span>
                </span>
              ))}
            </div>
          )}

          {/* Add expense form */}
          <form onSubmit={handleAdd} className="bg-brand-card border border-brand-border rounded-xl p-5 mb-6">
            <h2 className="text-white font-semibold text-sm mb-4 flex items-center gap-2">
              <Plus size={15} /> Xarajat qo&apos;shish
            </h2>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label className="block text-gray-400 text-xs mb-1.5">Kategoriya</label>
                <select
                  value={category}
                  onChange={(e) => setCategory(e.target.value)}
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
                >
                  {CATEGORIES.map((c) => (
                    <option key={c.value} value={c.value}>
                      {c.label}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-gray-400 text-xs mb-1.5">Summa (USD)</label>
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                  placeholder="0.00"
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
                />
              </div>
              <div className="sm:col-span-2">
                <label className="block text-gray-400 text-xs mb-1.5">Nomi</label>
                <input
                  type="text"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  placeholder="Masalan: Hetzner VPS — May oyi"
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
                />
              </div>
              <div>
                <label className="block text-gray-400 text-xs mb-1.5">Sana (ixtiyoriy)</label>
                <input
                  type="date"
                  value={incurredAt}
                  onChange={(e) => setIncurredAt(e.target.value)}
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
                />
              </div>
              <div className="flex items-end">
                <label className="flex items-center gap-2 text-gray-300 text-sm cursor-pointer">
                  <input
                    type="checkbox"
                    checked={recurring}
                    onChange={(e) => setRecurring(e.target.checked)}
                    className="w-4 h-4 cursor-pointer"
                  />
                  Takrorlanuvchi (oylik)
                </label>
              </div>
              <div className="sm:col-span-2">
                <label className="block text-gray-400 text-xs mb-1.5">Izoh (ixtiyoriy)</label>
                <input
                  type="text"
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
                />
              </div>
            </div>
            <button
              type="submit"
              disabled={submitting}
              className="mt-4 inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-brand-red text-white text-sm font-medium hover:bg-brand-red/80 transition-colors disabled:opacity-50"
            >
              {submitting ? <Loader2 size={14} className="animate-spin" /> : <Plus size={14} />}
              Qo&apos;shish
            </button>
          </form>

          {/* Expense list */}
          <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
            <div className="px-5 py-3 border-b border-brand-border">
              <h2 className="text-white font-semibold text-sm">Qo&apos;lda kiritilgan xarajatlar</h2>
            </div>
            {!summary || summary.expenses.length === 0 ? (
              <p className="text-gray-500 text-sm text-center py-10">Hali xarajat kiritilmagan.</p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-left text-xs text-gray-500 border-b border-brand-border">
                      <th className="px-5 py-2 font-medium">Nomi</th>
                      <th className="px-5 py-2 font-medium">Kategoriya</th>
                      <th className="px-5 py-2 font-medium">Sana</th>
                      <th className="px-5 py-2 font-medium text-right">Summa</th>
                      <th className="px-5 py-2 font-medium"></th>
                    </tr>
                  </thead>
                  <tbody>
                    {summary.expenses.map((ex) => (
                      <tr key={ex.id} className="border-b border-brand-border/50 last:border-0">
                        <td className="px-5 py-2.5 text-white">
                          {ex.title}
                          {ex.recurring && (
                            <span className="ml-2 text-[10px] uppercase tracking-wide text-blue-400">oylik</span>
                          )}
                          {ex.note && <p className="text-xs text-gray-500 mt-0.5">{ex.note}</p>}
                        </td>
                        <td className="px-5 py-2.5 text-gray-400 uppercase text-xs">{ex.category}</td>
                        <td className="px-5 py-2.5 text-gray-400">{fmtDate(ex.incurred_at)}</td>
                        <td className="px-5 py-2.5 text-right text-emerald-400 font-medium">{fmtUSD(ex.amount_usd)}</td>
                        <td className="px-5 py-2.5 text-right">
                          <button
                            onClick={() => handleDelete(ex.id)}
                            className="text-gray-600 hover:text-red-400 transition-colors"
                            title="O'chirish"
                          >
                            <Trash2 size={15} />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}
