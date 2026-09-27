"use client";

import { useParams } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import CollectionEditor from "@/components/admin/CollectionEditor";

export default function EditCollectionPage() {
  const { token } = useAuth();
  const params = useParams();
  const id = params.id as string;
  return <CollectionEditor token={token} collectionId={id === "new" ? null : id} />;
}
