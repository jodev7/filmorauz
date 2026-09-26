import Navbar from "@/components/Navbar";
import { DetailPageSkeleton } from "@/components/skeletons";

export default function Loading() {
  return (
    <>
      <Navbar />
      <DetailPageSkeleton />
    </>
  );
}
