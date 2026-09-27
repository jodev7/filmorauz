"use client";

import Link from "next/link";
import { ChevronLeft } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import NotificationSettings from "@/components/NotificationSettings";
import { useAuth } from "@/lib/auth-context";

export default function NotificationSettingsPage() {
  const { isAuthenticated, isLoading } = useAuth();
  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-24">
        <div className="mx-auto max-w-3xl px-4 pb-16">
          <Link href="/notifications" className="mb-4 inline-flex items-center gap-1 text-sm text-gray-400 hover:text-white">
            <ChevronLeft size={16} /> Bildirishnomalar
          </Link>
          <h1 className="mb-6 font-display text-3xl tracking-wide text-white sm:text-4xl">BILDIRISHNOMA SOZLAMALARI</h1>
          {!isLoading && !isAuthenticated ? (
            <p className="text-gray-400">Sozlamalar uchun tizimga kiring.</p>
          ) : (
            <NotificationSettings />
          )}
        </div>
      </main>
      <Footer />
    </>
  );
}
