"use client";

import { useRouter } from "next/navigation";
import MovieForm from "@/components/MovieForm";
import AdminPageHeader from "@/components/admin/form/PageHeader";
import { useAuth } from "@/lib/auth-context";
import { useToast } from "@/components/admin/Toast";
import { adminCreateMovie, IngestionJob, MovieInput } from "@/lib/api";

export default function NewMoviePage() {
  const { token } = useAuth();
  const router = useRouter();
  const toast = useToast();

  const handleSubmit = async (data: MovieInput) => {
    if (!token) throw new Error("Tizimga qayta kiring");
    await adminCreateMovie(token, data);
    toast.success(`"${data.title}" qo'shildi`);
    router.push("/admin/movies?status=pending");
  };

  const handleDirectUploadJobCreated = (_job: IngestionJob) => {
    toast.success("Video qayta ishlash navbatiga qo'yildi");
    router.push("/admin/ingestion");
  };

  return (
    <div className="p-4 sm:p-8">
      <AdminPageHeader
        backHref="/admin/movies"
        backLabel="Kinolar"
        title="Yangi kino"
        subtitle="Majburiy maydonlar: nomi, yili, tavsif, poster va video. Qoralama avtomatik saqlanadi."
      />
      <MovieForm mode="create" onSubmit={handleSubmit} submitLabel="Kinoni qo'shish" token={token ?? undefined} onDirectUploadJobCreated={handleDirectUploadJobCreated} />
    </div>
  );
}
