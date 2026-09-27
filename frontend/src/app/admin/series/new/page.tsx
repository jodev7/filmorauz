"use client";

import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { useToast } from "@/components/admin/Toast";
import { adminCreateSeries, CreateSeriesData } from "@/lib/api";
import AdminPageHeader from "@/components/admin/form/PageHeader";
import SeriesInfoForm from "@/components/admin/SeriesInfoForm";

export default function NewSeriesPage() {
  const { token } = useAuth();
  const router = useRouter();
  const toast = useToast();

  const create = async (data: CreateSeriesData) => {
    if (!token) throw new Error("Tizimga qayta kiring");
    const series = await adminCreateSeries(token, data);
    toast.success(`"${data.title}" yaratildi — endi fasl va qismlarni qo'shing`);
    // Straight to the editor so seasons/episodes can be added right away.
    const id = (series as { id?: string } | undefined)?.id;
    router.push(id ? `/admin/series/${id}/edit` : "/admin/series");
  };

  return (
    <div className="p-4 sm:p-8">
      <AdminPageHeader
        backHref="/admin/series"
        backLabel="Seriallar"
        title="Yangi serial"
        subtitle="Avval serial ma'lumotini saqlang — keyin fasl va qismlar qo'shish sahifasi ochiladi. Qoralama avtomatik saqlanadi."
      />
      <SeriesInfoForm mode="create" token={token} onSubmit={create} submitLabel="Yaratish va davom etish" />
    </div>
  );
}
