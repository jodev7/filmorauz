import type { Metadata } from "next";
import { cookies } from "next/headers";
import { notFound } from "next/navigation";
import { ListVideo, Lock, User as UserIcon } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import ListItemsGrid from "@/components/ListItemsGrid";
import ListShareButton from "@/components/ListShareButton";
import { getListBySlug } from "@/lib/api";
import { SITE_URL } from "@/lib/content-routes";

export const dynamic = "force-dynamic";

interface Props {
  params: { slug: string };
}

async function load(slug: string) {
  const token = cookies().get("auth_token")?.value;
  try {
    return await getListBySlug(slug, token);
  } catch {
    return null;
  }
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const list = await load(params.slug);
  if (!list) return { title: "Ro'yxat topilmadi", robots: { index: false } };
  const title = `${list.title} — ${list.owner_name}ning ro'yxati`;
  const description = list.description || `${list.owner_name} tuzgan ${list.count} ta kino va serialdan iborat ro'yxat. FilmoraUz'da tomosha qiling.`;
  const url = `${SITE_URL}/lists/${list.share_slug}`;
  return {
    title,
    description,
    alternates: { canonical: url },
    robots: list.is_public ? undefined : { index: false },
    openGraph: { title, description, url, type: "website", siteName: "FILMORAUZ", locale: "uz_UZ", images: list.covers[0] ? [list.covers[0]] : undefined },
  };
}

export default async function ListPage({ params }: Props) {
  const list = await load(params.slug);
  if (!list) notFound();

  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-24">
        <div className="mx-auto max-w-7xl px-4 pb-16">
          <header className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
            <div className="min-w-0">
              <p className="mb-2 flex items-center gap-1.5 text-sm text-orange-400">
                <ListVideo size={16} /> Ro&apos;yxat
                {!list.is_public && (
                  <span className="ml-2 inline-flex items-center gap-1 rounded-full bg-white/5 px-2 py-0.5 text-xs text-gray-400">
                    <Lock size={11} /> yopiq — faqat sizga ko&apos;rinadi
                  </span>
                )}
              </p>
              <h1 className="font-display text-4xl leading-none tracking-wide text-white sm:text-5xl">{list.title}</h1>
              {list.description && <p className="mt-3 max-w-2xl text-gray-300">{list.description}</p>}
              <p className="mt-3 flex items-center gap-1.5 text-sm text-gray-500">
                <UserIcon size={14} /> {list.owner_name} · {list.count} ta
              </p>
            </div>
            {list.is_public && <ListShareButton title={list.title} slug={list.share_slug} />}
          </header>
          <ListItemsGrid list={list} />
        </div>
      </main>
      <Footer />
    </>
  );
}
