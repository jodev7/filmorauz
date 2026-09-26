// Loading placeholders shaped like the real pages, so navigation shows the
// layout immediately instead of a blank screen or spinner.

const pulse = "animate-pulse rounded-xl bg-white/[0.06]";

export function PosterCardSkeleton() {
  return (
    <div>
      <div className={`${pulse} aspect-[2/3]`} />
      <div className={`${pulse} mt-2 h-4 w-3/4 rounded-md`} />
      <div className={`${pulse} mt-1.5 h-3 w-1/3 rounded-md`} />
    </div>
  );
}

export function PosterGridSkeleton({ count = 12 }: { count?: number }) {
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4 md:grid-cols-4 lg:grid-cols-6" aria-hidden="true">
      {Array.from({ length: count }, (_, i) => (
        <PosterCardSkeleton key={i} />
      ))}
    </div>
  );
}

export function RowSkeleton() {
  return (
    <div aria-hidden="true">
      <div className={`${pulse} mb-4 h-7 w-48 rounded-md`} />
      <div className="flex gap-3 overflow-hidden">
        {Array.from({ length: 7 }, (_, i) => (
          <div key={i} className="w-[140px] shrink-0 sm:w-[180px]">
            <PosterCardSkeleton />
          </div>
        ))}
      </div>
    </div>
  );
}

export function ListPageSkeleton({ withFilters = true }: { withFilters?: boolean }) {
  return (
    <div className="mx-auto max-w-7xl px-4 pt-24" role="status" aria-label="Yuklanmoqda">
      <div className={`${pulse} h-10 w-64 rounded-lg`} />
      <div className={`${pulse} mt-3 h-4 w-40 rounded-md`} />
      {withFilters && (
        <div className="mt-6 flex gap-2 overflow-hidden">
          {Array.from({ length: 8 }, (_, i) => (
            <div key={i} className={`${pulse} h-9 w-24 shrink-0 rounded-full`} />
          ))}
        </div>
      )}
      <div className="mt-6">
        <PosterGridSkeleton />
      </div>
    </div>
  );
}

export function DetailPageSkeleton() {
  return (
    <div role="status" aria-label="Yuklanmoqda">
      <div className="h-[36vh] min-h-[240px] animate-pulse bg-white/[0.04] sm:h-[55vh] sm:min-h-[380px]" />
      <div className="relative mx-auto -mt-28 max-w-7xl px-4 sm:-mt-36">
        <div className="flex gap-4 sm:gap-6 md:gap-8">
          <div className={`${pulse} aspect-[2/3] w-28 shrink-0 sm:w-40 md:w-48 lg:w-56`} />
          <div className="flex-1 pt-10 sm:pt-16 md:pt-24">
            <div className={`${pulse} h-9 w-3/4 rounded-lg sm:h-12`} />
            <div className={`${pulse} mt-3 h-4 w-1/2 rounded-md`} />
            <div className="mt-3 flex gap-2">
              <div className={`${pulse} h-7 w-20 rounded-full`} />
              <div className={`${pulse} h-7 w-20 rounded-full`} />
            </div>
          </div>
        </div>
        <div className="mt-5 md:ml-[14rem] lg:ml-[16rem]">
          <div className="flex gap-3">
            <div className={`${pulse} h-12 w-full sm:w-56`} />
          </div>
          <div className={`${pulse} mt-6 h-4 w-full max-w-3xl rounded-md`} />
          <div className={`${pulse} mt-2 h-4 w-11/12 max-w-3xl rounded-md`} />
          <div className={`${pulse} mt-2 h-4 w-2/3 max-w-3xl rounded-md`} />
        </div>
        <div className="mt-12">
          <RowSkeleton />
        </div>
      </div>
    </div>
  );
}
