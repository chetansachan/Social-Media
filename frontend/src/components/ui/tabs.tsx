"use client";

import * as React from "react";
import * as TabsPrimitive from "@radix-ui/react-tabs";
import { cn } from "@/lib/utils";

export const Tabs = TabsPrimitive.Root;

export const TabsList = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.List>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.List>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.List
    ref={ref}
    className={cn("flex w-full items-stretch border-b border-line", className)}
    {...props}
  />
));
TabsList.displayName = "TabsList";

export const TabsTrigger = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.Trigger>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.Trigger>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.Trigger
    ref={ref}
    className={cn(
      "group relative flex flex-1 items-center justify-center gap-1.5 px-4 py-4",
      "text-base font-medium text-secondary transition-colors duration-200",
      "hover:bg-hover hover:text-primary",
      "focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring",
      "data-[state=active]:font-semibold data-[state=active]:text-primary",
      // The indicator is a real element, so it animates with a transform.
      "after:absolute after:inset-x-0 after:bottom-0 after:h-1 after:rounded-full",
      "after:bg-accent after:opacity-0 after:transition-opacity after:duration-200",
      "data-[state=active]:after:opacity-100",
      className,
    )}
    {...props}
  />
));
TabsTrigger.displayName = "TabsTrigger";

export const TabsContent = TabsPrimitive.Content;