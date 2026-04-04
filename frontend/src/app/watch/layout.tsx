import { Metadata } from "next";

export const metadata: Metadata = {
  title: "Tomosha qilish — FILMORAUZ",
  description: "Film va seriallarni tomosha qiling",
};

export default function WatchLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <>{children}</>;
}
