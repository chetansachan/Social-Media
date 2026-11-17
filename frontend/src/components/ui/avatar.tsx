"use client";

import * as React from "react";
import * as AvatarPrimitive from "@radix-ui/react-avatar";
import { cn } from "@/lib/utils";
import { handleHue, initials } from "@/lib/format";

const sizeMap = {
  xs: "size-6 text-2xs",
  sm: "size-8 text-[11px]",
  md: "size-10 text-xs",
  lg: "size-12 text-sm",
  xl: "size-20 text-xl",
  "2xl": "size-28 text-2xl",
} as const;

export type AvatarSize = keyof typeof sizeMap;

export function Avatar({
  name,
  handle,
  src,
  size = "md",
  className,
}: {
  name: string;
  handle: string;
  src?: string;
  size?: AvatarSize;
  className?: string;
}) {
  const hue = handleHue(handle);
  const palette = [212, 262, 190, 330, 280, 168];
  const baseHue = palette[hue % palette.length]!;
  return (
    <AvatarPrimitive.Root
      className={cn(
        "relative inline-flex shrink-0 select-none items-center justify-center overflow-hidden rounded-full bg-inset",
        sizeMap[size],
        className,
      )}
    >
      {src ? (
        <AvatarPrimitive.Image
          src={src}
          alt=""
          className="size-full object-cover"
          loading="lazy"
          decoding="async"
        />
      ) : null}
      <AvatarPrimitive.Fallback
        delayMs={src ? 200 : 0}
        className="flex size-full items-center justify-center font-mono font-medium uppercase tracking-mono text-white/95"
        style={{
          backgroundImage: `linear-gradient(140deg, hsl(${baseHue} 58% 34%), hsl(${(baseHue + 34) % 360} 62% 19%))`,
        }}
      >
        {initials(name)}
      </AvatarPrimitive.Fallback>
    </AvatarPrimitive.Root>
  );
}