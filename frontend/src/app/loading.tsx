/**
 * Content fallback while a route segment loads. The shell lives above this in
 * the root layout, so navigation keeps the rails in place and only the column
 * shows a skeleton.
 */
export default function Loading() {
  return (
    <div className="w-full">
      <div className="border-b border-line px-4 py-3">
        <div className="flex gap-3">
          <div className="sf-skeleton size-10 shrink-0 rounded-full" aria-hidden="true" />
          <div className="flex-1 space-y-2" aria-hidden="true">
            <div className="sf-skeleton h-4 w-1/3 rounded" />
            <div className="sf-skeleton h-4 w-3/4 rounded" />
          </div>
        </div>
      </div>
      <div aria-hidden="true">
        {[0, 1, 2].map((row) => (
          <div key={row} className="flex gap-3 border-b border-line px-4 py-4">
            <div className="sf-skeleton size-10 shrink-0 rounded-full" />
            <div className="flex-1 space-y-2">
              <div className="sf-skeleton h-3 w-40 rounded" />
              <div className="sf-skeleton h-4 w-[90%] rounded" />
              <div className="sf-skeleton h-4 w-[70%] rounded" />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
