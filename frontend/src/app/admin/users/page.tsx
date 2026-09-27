"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Ban, Crown, Loader2, Shield, ShieldCheck, Unlock, User, UserX, Users, Wallet } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { isModeratorRole, isStaffRole, ROLE_LABELS } from "@/lib/roles";
import { readUrlNumber, readUrlParam, useSyncUrlParams } from "@/lib/url-state";
import { getAdminUsers, updateAdminUserRole, updateAdminUserPremium, updateUserWallet, banUser, unbanUser, AdminUser } from "@/lib/api";
import { useToast } from "@/components/admin/Toast";
import { Avatar, Chip, EmptyState, IconBtn, Modal, PageHead, Pager, SearchBox, SkeletonList, Tabs, Tone, fmtDate, timeAgo } from "@/components/admin/kit";
import { inputCls } from "@/components/admin/form/ui";

const ROLE_META: Record<string, { tone: Tone; icon: typeof User }> = {
  superadmin: { tone: "red", icon: Crown },
  admin: { tone: "orange", icon: Shield },
  moderator: { tone: "blue", icon: ShieldCheck },
  user: { tone: "gray", icon: User },
};

const PREMIUM_DAYS = [1, 7, 30, 90, 180, 365];
const BAN_DAYS = [
  { v: 1, label: "1 kun" },
  { v: 3, label: "3 kun" },
  { v: 7, label: "7 kun" },
  { v: 30, label: "30 kun" },
  { v: 0, label: "Doimiy" },
];
const BAN_REASONS = ["Spam", "Haqoratli xulq-atvor", "Noqonuniy faoliyat", "Qoidabuzarlik"];
const WALLET_QUICK = [10000, 25000, 50000, 100000];

const isSuper = (r?: string) => r?.toLowerCase() === "superadmin";

function nameOf(u: AdminUser): string {
  if (u.first_name || u.last_name) return [u.first_name, u.last_name].filter(Boolean).join(" ");
  if (u.display_name) return u.display_name;
  if (u.username) return `@${u.username}`;
  if (u.telegram_id) return `ID ${u.telegram_id}`;
  return "Noma'lum";
}

export default function AdminUsersPage() {
  const { token, isLoading: authLoading, user } = useAuth();
  const router = useRouter();
  const toast = useToast();
  const me = user?.role;
  const superMe = me === "superadmin";

  const [users, setUsers] = useState<AdminUser[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(() => readUrlNumber("page", 1));
  const [totalPages, setTotalPages] = useState(0);
  const [searchInput, setSearchInput] = useState(() => readUrlParam("search", ""));
  const [search, setSearch] = useState(() => readUrlParam("search", "").trim());
  const [role, setRole] = useState(() => readUrlParam("role", "all"));
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState<string | null>(null);

  const [premiumFor, setPremiumFor] = useState<AdminUser | null>(null);
  const [walletFor, setWalletFor] = useState<AdminUser | null>(null);
  const [walletAmount, setWalletAmount] = useState("");
  const [banFor, setBanFor] = useState<AdminUser | null>(null);
  const [banDays, setBanDays] = useState(1);
  const [banReason, setBanReason] = useState("");
  const [banCustom, setBanCustom] = useState("");
  const [saving, setSaving] = useState(false);

  // Filters live in the URL (?search=&role=&page=) so refresh/back keep the
  // view and the Ctrl+K palette can deep-link to a user.
  useSyncUrlParams({ search, role, page }, { search: "", role: "all", page: 1 });

  useEffect(() => {
    const next = searchInput.trim();
    if (next === search) return;
    const t = setTimeout(() => {
      setSearch(next);
      setPage(1);
    }, 300);
    return () => clearTimeout(t);
  }, [searchInput, search]);

  useEffect(() => {
    if (!authLoading && (!token || !isStaffRole(me))) router.push("/");
  }, [authLoading, token, me, router]);

  const load = useCallback(
    async (quiet = false) => {
      if (!token) return;
      if (!quiet) setLoading(true);
      try {
        const data = await getAdminUsers(token, { page, limit: 20, search: search || undefined, role: role !== "all" ? role : undefined });
        setUsers(data.data);
        setTotal(data.total);
        setTotalPages(data.total_pages);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Yuklab bo'lmadi");
      } finally {
        setLoading(false);
      }
    },
    [token, page, search, role, toast]
  );

  useEffect(() => {
    load();
  }, [load]);

  const act = async (id: string, fn: () => Promise<unknown>, ok: string) => {
    setBusyId(id);
    try {
      await fn();
      toast.success(ok);
      await load(true);
      return true;
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Xatolik");
      return false;
    } finally {
      setBusyId(null);
    }
  };

  const changeRole = (u: AdminUser, r: string) => token && act(u.id, () => updateAdminUserRole(token, u.id, r), `Rol o'zgartirildi: ${ROLE_LABELS[r] ?? r}`);

  const setPremium = async (u: AdminUser, on: boolean, days: number | null) => {
    if (!token) return;
    setSaving(true);
    const done = await act(u.id, () => updateAdminUserPremium(token, u.id, on, null, days), on ? `Premium: +${days} kun` : "Premium olib tashlandi");
    setSaving(false);
    if (done) setPremiumFor(null);
  };

  const topUp = async () => {
    if (!token || !walletFor) return;
    const amount = parseFloat(walletAmount);
    if (!(amount > 0)) return toast.error("Miqdorni kiriting");
    setSaving(true);
    const done = await act(walletFor.id, () => updateUserWallet(token, walletFor.id, amount), `Hamyonga ${amount.toLocaleString()} so'm qo'shildi`);
    setSaving(false);
    if (done) setWalletFor(null);
  };

  const doBan = async () => {
    if (!token || !banFor) return;
    const reason = (banReason === "other" ? banCustom : banReason).trim();
    if (!reason) return toast.error("Sababni tanlang");
    setSaving(true);
    const done = await act(banFor.id, () => banUser(token, banFor.id, banDays, reason), `${nameOf(banFor)} ban qilindi`);
    setSaving(false);
    if (done) setBanFor(null);
  };

  const openBan = (u: AdminUser) => {
    setBanFor(u);
    setBanDays(1);
    setBanReason("");
    setBanCustom("");
  };

  if (authLoading || !token || !isStaffRole(me)) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="animate-spin text-gray-500" />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl p-4 sm:p-6">
      <PageHead
        icon={Users}
        gradient="from-sky-500 to-blue-700"
        title="Foydalanuvchilar"
        subtitle={total ? `Jami ${total.toLocaleString()} ta foydalanuvchi` : "Barcha foydalanuvchilarni boshqarish"}
        actions={
          <Link
            href="/admin/users/banned"
            className="inline-flex items-center gap-2 rounded-xl border border-white/10 bg-white/[0.03] px-3.5 py-2.5 text-sm text-gray-300 transition hover:bg-white/[0.07] hover:text-white"
          >
            <UserX size={15} /> Ban olganlar
          </Link>
        }
      />

      <div className="mb-4 flex flex-col gap-3 lg:flex-row">
        <SearchBox value={searchInput} onChange={setSearchInput} placeholder="Ism, username, Telegram ID yoki ID..." />
        <Tabs<string>
          value={role}
          onChange={(r) => {
            setRole(r);
            setPage(1);
          }}
          items={[
            { key: "all", label: "Hammasi" },
            { key: "user", label: "Foydalanuvchi" },
            { key: "moderator", label: "Moderator" },
            { key: "admin", label: "Admin" },
            { key: "superadmin", label: "Super admin" },
          ]}
        />
      </div>

      {loading ? (
        <SkeletonList rows={8} height={68} />
      ) : users.length === 0 ? (
        <EmptyState icon={Users} title="Foydalanuvchi topilmadi" text="Qidiruv yoki rol filtrini o'zgartirib ko'ring." />
      ) : (
        <div className="overflow-hidden rounded-2xl border border-white/10 bg-[#12121a]">
          <div className="hidden grid-cols-[minmax(0,2fr)_140px_150px_minmax(0,1fr)_auto] gap-4 border-b border-white/5 px-4 py-2.5 text-[11px] font-medium uppercase tracking-wider text-gray-500 lg:grid">
            <span>Foydalanuvchi</span>
            <span>Rol</span>
            <span>Premium</span>
            <span>Faollik</span>
            <span className="w-[120px] text-right">Amallar</span>
          </div>
          <ul className="divide-y divide-white/5">
            {users.map((u) => {
              const rm = ROLE_META[u.role] ?? ROLE_META.user;
              const protectedAcc = isSuper(u.role) || (isModeratorRole(me) && u.role !== "user");
              const busy = busyId === u.id;
              const premiumActive = u.is_premium_active ?? u.is_premium;
              return (
                <li key={u.id} className={`grid gap-3 px-4 py-3 transition hover:bg-white/[0.02] lg:grid-cols-[minmax(0,2fr)_140px_150px_minmax(0,1fr)_auto] lg:items-center lg:gap-4 ${busy ? "opacity-60" : ""}`}>
                  <div className="flex min-w-0 items-center gap-3">
                    <Avatar name={nameOf(u)} size={38} />
                    <div className="min-w-0">
                      <div className="flex items-center gap-1.5">
                        <Link href={`/user/${u.id}`} className="truncate font-medium text-white hover:text-orange-300">
                          {nameOf(u)}
                        </Link>
                        {u.is_banned && (
                          <Chip tone="red" icon={Ban}>
                            Ban
                          </Chip>
                        )}
                      </div>
                      <p className="truncate text-xs text-gray-500">
                        {u.username && `@${u.username} · `}
                        <span className="font-mono">{u.telegram_id || "—"}</span>
                      </p>
                    </div>
                  </div>

                  <div>
                    {superMe && !isSuper(u.role) ? (
                      <select
                        value={u.role}
                        disabled={busy}
                        onChange={(e) => changeRole(u, e.target.value)}
                        className="w-full cursor-pointer rounded-lg border border-white/10 bg-black/30 px-2 py-1.5 text-xs text-white focus:border-orange-500 focus:outline-none lg:w-[130px]"
                        aria-label="Rol"
                      >
                        <option value="user">Foydalanuvchi</option>
                        <option value="moderator">Moderator</option>
                        <option value="admin">Admin</option>
                        <option value="superadmin">Super admin</option>
                      </select>
                    ) : (
                      <Chip tone={rm.tone} icon={rm.icon}>
                        {ROLE_LABELS[u.role] ?? "Foydalanuvchi"}
                      </Chip>
                    )}
                  </div>

                  <div>
                    {superMe ? (
                      <button
                        type="button"
                        onClick={() => setPremiumFor(u)}
                        className={`inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs transition ${
                          premiumActive ? "border-yellow-500/30 bg-yellow-500/10 text-yellow-300 hover:bg-yellow-500/20" : "border-white/10 text-gray-400 hover:border-yellow-500/30 hover:text-yellow-300"
                        }`}
                      >
                        <Crown size={12} />
                        {premiumActive ? (u.premium_expires_at ? `${fmtDate(u.premium_expires_at, false)} gacha` : "Premium") : "Berish"}
                      </button>
                    ) : premiumActive ? (
                      <Chip tone="yellow" icon={Crown}>
                        {u.premium_expires_at ? `${fmtDate(u.premium_expires_at, false)} gacha` : "Premium"}
                      </Chip>
                    ) : (
                      <span className="text-xs text-gray-600">—</span>
                    )}
                  </div>

                  <div className="text-xs text-gray-500">
                    <p title={fmtDate(u.last_login_at)}>Kirgan: {timeAgo(u.last_login_at)}</p>
                    <p title={fmtDate(u.created_at)}>Qo&apos;shilgan: {fmtDate(u.created_at, false)}</p>
                  </div>

                  <div className="flex items-center justify-end gap-0.5 lg:w-[120px]">
                    {superMe && (
                      <IconBtn label={`Hamyon: ${(u.wallet_balance ?? 0).toLocaleString()} so'm`} tone="yellow" onClick={() => {
                        setWalletFor(u);
                        setWalletAmount("");
                      }}>
                        <Wallet size={16} />
                      </IconBtn>
                    )}
                    {protectedAcc ? (
                      <span className="inline-flex items-center gap-1 px-2 text-[11px] text-amber-300/80" title="Himoyalangan hisob">
                        <ShieldCheck size={13} /> Himoyada
                      </span>
                    ) : u.is_banned ? (
                      <IconBtn label="Bandan chiqarish" tone="green" disabled={busy} onClick={() => token && act(u.id, () => unbanUser(token, u.id), `${nameOf(u)} bandan chiqarildi`)}>
                        <Unlock size={16} />
                      </IconBtn>
                    ) : (
                      <IconBtn label="Ban qilish" tone="red" disabled={busy} onClick={() => openBan(u)}>
                        <Ban size={16} />
                      </IconBtn>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>
        </div>
      )}

      <Pager page={page} totalPages={totalPages} total={total} unit="ta foydalanuvchi" onChange={setPage} />

      {/* Premium */}
      <Modal open={!!premiumFor} onClose={() => setPremiumFor(null)} busy={saving} size="sm" icon={Crown} iconTone="yellow" title="Premium" subtitle={premiumFor ? nameOf(premiumFor) : undefined}>
        {premiumFor && (
          <div className="space-y-4">
            <div className="rounded-xl bg-black/20 p-3 text-sm">
              {premiumFor.is_premium_active ?? premiumFor.is_premium ? (
                <p className="text-yellow-300">
                  Faol{premiumFor.premium_expires_at && <span className="text-gray-400"> · {fmtDate(premiumFor.premium_expires_at)} gacha</span>}
                </p>
              ) : (
                <p className="text-gray-400">Premium yo&apos;q</p>
              )}
            </div>
            <div>
              <p className="mb-2 text-xs font-medium text-gray-400">{premiumFor.is_premium ? "Uzaytirish" : "Premium berish"}</p>
              <div className="grid grid-cols-3 gap-2">
                {PREMIUM_DAYS.map((d) => (
                  <button
                    key={d}
                    type="button"
                    disabled={saving}
                    onClick={() => setPremium(premiumFor, true, d)}
                    className="rounded-xl border border-yellow-500/20 bg-yellow-500/[0.06] py-2.5 text-sm font-medium text-yellow-200 transition hover:bg-yellow-500/15 disabled:opacity-50"
                  >
                    +{d} kun
                  </button>
                ))}
              </div>
            </div>
            {premiumFor.is_premium && (
              <button type="button" disabled={saving} onClick={() => setPremium(premiumFor, false, null)} className="w-full rounded-xl border border-red-500/20 py-2 text-sm text-red-300 hover:bg-red-500/10 disabled:opacity-50">
                Premiumni olib tashlash
              </button>
            )}
          </div>
        )}
      </Modal>

      {/* Wallet */}
      <Modal
        open={!!walletFor}
        onClose={() => setWalletFor(null)}
        busy={saving}
        size="sm"
        icon={Wallet}
        iconTone="yellow"
        title="Hamyonni to'ldirish"
        subtitle={walletFor ? nameOf(walletFor) : undefined}
        footer={
          <>
            <button type="button" onClick={() => setWalletFor(null)} className="rounded-xl px-4 py-2 text-sm text-gray-300 hover:bg-white/5">
              Bekor qilish
            </button>
            <button type="button" onClick={topUp} disabled={saving || !(parseFloat(walletAmount) > 0)} className="inline-flex items-center gap-2 rounded-xl bg-yellow-500 px-5 py-2 text-sm font-semibold text-black hover:bg-yellow-400 disabled:opacity-50">
              {saving && <Loader2 size={15} className="animate-spin" />} Qo&apos;shish
            </button>
          </>
        }
      >
        {walletFor && (
          <div className="space-y-3">
            <p className="text-sm text-gray-400">
              Joriy balans: <span className="font-semibold text-yellow-300">{(walletFor.wallet_balance ?? 0).toLocaleString()} so&apos;m</span>
            </p>
            <input type="number" min={1} step={1000} autoFocus value={walletAmount} onChange={(e) => setWalletAmount(e.target.value)} placeholder="Miqdor (so'm)" className={inputCls} />
            <div className="grid grid-cols-4 gap-1.5">
              {WALLET_QUICK.map((a) => (
                <button key={a} type="button" onClick={() => setWalletAmount(String(a))} className="rounded-lg border border-white/10 py-1.5 text-xs text-gray-300 hover:border-yellow-500/40 hover:text-white">
                  {a / 1000}k
                </button>
              ))}
            </div>
          </div>
        )}
      </Modal>

      {/* Ban */}
      <Modal
        open={!!banFor}
        onClose={() => setBanFor(null)}
        busy={saving}
        icon={Ban}
        iconTone="red"
        title="Ban qilish"
        subtitle={banFor ? nameOf(banFor) : undefined}
        footer={
          <>
            <button type="button" onClick={() => setBanFor(null)} className="rounded-xl px-4 py-2 text-sm text-gray-300 hover:bg-white/5">
              Bekor qilish
            </button>
            <button
              type="button"
              onClick={doBan}
              disabled={saving || !banReason || (banReason === "other" && !banCustom.trim())}
              className="inline-flex items-center gap-2 rounded-xl bg-red-600 px-5 py-2 text-sm font-semibold text-white hover:bg-red-500 disabled:opacity-50"
            >
              {saving && <Loader2 size={15} className="animate-spin" />} Ban qilish
            </button>
          </>
        }
      >
        <div className="space-y-4">
          <div>
            <p className="mb-2 text-xs font-medium text-gray-400">Muddat</p>
            <div className="grid grid-cols-5 gap-1.5">
              {BAN_DAYS.map((d) => (
                <button
                  key={d.v}
                  type="button"
                  onClick={() => setBanDays(d.v)}
                  className={`rounded-xl border py-2 text-xs font-medium transition ${
                    banDays === d.v ? (d.v === 0 ? "border-red-500 bg-red-500/15 text-red-200" : "border-orange-500 bg-orange-500/10 text-white") : "border-white/10 text-gray-400 hover:text-white"
                  }`}
                >
                  {d.label}
                </button>
              ))}
            </div>
            {banDays === 0 && <p className="mt-1.5 text-xs text-red-400">Foydalanuvchi muddatsiz bloklanadi</p>}
          </div>
          <div>
            <p className="mb-2 text-xs font-medium text-gray-400">Sabab</p>
            <div className="flex flex-wrap gap-1.5">
              {[...BAN_REASONS, "other"].map((r) => (
                <button
                  key={r}
                  type="button"
                  onClick={() => setBanReason(r)}
                  className={`rounded-full border px-3 py-1.5 text-xs transition ${banReason === r ? "border-orange-500 bg-orange-500/10 text-white" : "border-white/10 text-gray-400 hover:text-white"}`}
                >
                  {r === "other" ? "Boshqa sabab" : r}
                </button>
              ))}
            </div>
            {banReason === "other" && <input autoFocus value={banCustom} onChange={(e) => setBanCustom(e.target.value)} placeholder="Sababni yozing..." className={`${inputCls} mt-2`} />}
          </div>
          <p className="rounded-xl border border-amber-500/20 bg-amber-500/[0.06] px-3 py-2 text-xs text-amber-200">Foydalanuvchi saytdan to&apos;liq bloklanadi va apellyatsiya yubora oladi.</p>
        </div>
      </Modal>
    </div>
  );
}
