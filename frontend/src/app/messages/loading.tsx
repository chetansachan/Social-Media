export default function MessagesLoading() {
  return (
    <div className="flex h-[calc(100dvh_-_var(--layout-bottom-bar)_-_env(safe-area-inset-bottom,0px))] min-h-0 md:h-dvh">
      <div className="w-full border-r border-line md:w-[360px]" aria-hidden="true">
        <div className="flex h-[60px] items-center border-b border-line px-4">
          <div className="sf-skeleton h-5 w-24 rounded" />
        </div>
        <div className="space-y-0">
          {[0, 1, 2, 3].map((row) => (
            <div key={row} className="flex gap-3 border-b border-line px-4 py-3">
              <div className="sf-skeleton size-10 shrink-0 rounded-full" />
              <div className="flex-1 space-y-2">
                <div className="sf-skeleton h-3 w-1/3 rounded" />
                <div className="sf-skeleton h-3 w-2/3 rounded" />
              </div>
            </div>
          ))}
        </div>
      </div>
      <div className="hidden flex-1 p-4 md:block" aria-hidden="true">
        <div className="sf-skeleton h-12 w-full rounded-lg" />
      </div>
    </div>
  );
}
