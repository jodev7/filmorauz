"use client";

import { useAuth } from "@/lib/auth-context";
import CollectionEditor from "@/components/admin/CollectionEditor";

export default function NewCollectionPage() {
  const { token } = useAuth();
  return <CollectionEditor token={token} collectionId={null} />;
}
