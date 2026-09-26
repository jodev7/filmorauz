import Navbar from "@/components/Navbar";
import { ListPageSkeleton } from "@/components/skeletons";

export default function Loading() {
  return (
    <>
      <Navbar />
      <ListPageSkeleton />
    </>
  );
}
