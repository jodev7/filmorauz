"use client";

import { useRouter } from "next/navigation";
import Link from "next/link";
import { ChevronLeft } from "lucide-react";
import MovieForm from "@/components/MovieForm";
import { useAuth } from "@/lib/auth-context";
import { adminCreateMovie, MovieInput } from "@/lib/api";

export default function NewMoviePage() {
  const { token } = useAuth();
  const router = useRouter();

  const handleSubmit = async (data: MovieInput) => {
    if (!token) throw new Error("Not authenticated");
    await adminCreateMovie(token, data);
    router.push("/admin/movies");
  };

  return (
    <div className="p-8">
      {/* Header */}
      <div className="mb-8">
        <Link
          href="/admin/movies"
          className="flex items-center gap-1 text-sm text-gray-500 hover:text-white mb-4 transition-colors"
        >
          <ChevronLeft size={16} />
          Back to Movies
        </Link>
        <h1 className="text-2xl font-bold text-white">Add New Movie</h1>
        <p className="text-gray-500 text-sm mt-1">
          Fill in the details below to add a new movie.
        </p>
      </div>

      <MovieForm onSubmit={handleSubmit} submitLabel="Create Movie" />
    </div>
  );
}
