"use client";

import * as React from "react";
import * as SwitchPrimitive from "@radix-ui/react-switch";
import { cn } from "@/lib/utils";

export const Switch = React.forwardRef<
  React.ElementRef<typeof SwitchPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof SwitchPrimitive.Root>
>(({ className, ...props }, ref) => (
  <SwitchPrimitive.Root
    ref={ref}
    className={cn(
      "relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full",
      "bg-inset ring-1 ring-inset ring-line-strong transition-colors duration-200",
      "data-[state=checked]:bg-accent-solid data-[state=checked]:ring-accent-solid",
      "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
      "disabled:cursor-not-allowed disabled:opacity-50",
      className,
    )}
    {...props}
  >
    <SwitchPrimitive.Thumb
      className={cn(
        "pointer-events-none block size-5 rounded-full bg-white shadow-1",
        "transition-transform duration-200 ease-standard",
        "translate-x-0.5 data-[state=checked]:translate-x-[22px]",
      )}
    />
  </SwitchPrimitive.Root>
));
Switch.displayName = "Switch";