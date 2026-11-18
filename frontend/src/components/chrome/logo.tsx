import Link from "next/link";
import { Feather } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * The brand mark. One circular solid-accent disc with a white feather — the
 * same shape at every size, so the rail, the mobile bar and the auth screen
 * stay recognisably one product.
 */
export function LogoMark({
  size = 40,
  className,
}: {
  size?: number;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "grid shrink-0 place-items-center rounded-full bg-accent-solid text-on-accent shadow-glow",
        className,
      )}
      style={{ width: size, height: size }}
    >
      <Feather
        strokeWidth={2.2}
        style={{ width: Math.round(size * 0.5), height: Math.round(size * 0.5) }}
      />
    </span>
  );
}

export function LogoLink({ compact = false }: { compact?: boolean }) {
  return (
    <Link
      href="/"
      className="flex items-center gap-2 rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
      aria-label="SocialFlow home"
    >
      <LogoMark size={compact ? 40 : 42} />
      {compact ? null : (
        <span className="sf-wordmark text-2xl text-primary">SocialFlow</span>
      )}
    </Link>
  );
}