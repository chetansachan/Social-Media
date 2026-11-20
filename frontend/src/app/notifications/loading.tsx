export default function NotificationsLoading() {
  return (
    <div className="w-full">
      <div className="flex border-b border-line px-4 py-4" aria-hidden="true">
        {[0, 1, 2, 3].map((tab) => (
          <div key={tab} className="flex-1">
            <div className="mx-auto h-4 w-14 rounded bg-[var(--surface-inset)]" />
          </div>
        ))}
      </div>
      <div aria-hidden="true">
        {[0, 1, 2, 3].map((row) => (
          <div key={row} className="flex gap-3 border-b border-line px-4 py-4">
            <div className="sf-skeleton size-10 shrink-0 rounded-full" />
            <div className="flex-1 space-y-2">
              <div className="sf-skeleton h-4 w-2/3 rounded" />
              <div className="sf-skeleton h-3 w-1/3 rounded" />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
