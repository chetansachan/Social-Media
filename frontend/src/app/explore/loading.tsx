export default function ExploreLoading() {
  return (
    <div className="w-full">
      <div className="border-b border-line px-4 py-2" aria-hidden="true">
        <div className="sf-skeleton h-11 w-full rounded-full" />
      </div>
      <div className="flex border-b border-line px-4 py-4" aria-hidden="true">
        <div className="flex-1">
          <div className="mx-auto h-4 w-16 rounded bg-[var(--surface-inset)]" />
        </div>
        <div className="flex-1">
          <div className="mx-auto h-4 w-16 rounded bg-[var(--surface-inset)]" />
        </div>
      </div>
      <div className="space-y-0" aria-hidden="true">
        {[0, 1, 2, 3].map((row) => (
          <div key={row} className="flex items-center gap-3 border-b border-line px-4 py-4">
            <div className="sf-skeleton size-9 shrink-0 rounded-full" />
            <div className="flex-1 space-y-2">
              <div className="sf-skeleton h-4 w-1/3 rounded" />
              <div className="sf-skeleton h-3 w-1/2 rounded" />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
