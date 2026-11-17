"use client";

import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  [
    "inline-flex items-center justify-center gap-2 whitespace-nowrap",
    "cursor-pointer font-sans font-semibold select-none",
    "transition-[background-color,color,border-color,box-shadow,opacity,transform] duration-200 ease-standard",
    "disabled:pointer-events-none disabled:opacity-50",
    "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
    "[&_svg]:pointer-events-none [&_svg]:shrink-0",
  ].join(" "),
  {
    variants: {
      variant: {
        primary:
          "bg-accent-solid text-on-accent hover:bg-accent-solid-hover active:opacity-90",
        secondary:
          "border border-line-interactive bg-transparent text-primary hover:bg-hover active:bg-hover",
        outline:
          "border border-line-interactive text-primary hover:bg-hover active:bg-hover",
        ghost: "text-secondary hover:bg-hover hover:text-primary active:bg-hover",
        subtle: "bg-inset text-primary hover:bg-hover active:bg-hover",
        danger: "bg-danger text-on-accent hover:opacity-90 active:opacity-80",
        link: "h-auto p-0 font-medium text-accent underline-offset-4 hover:underline",
      },
      size: {
        sm: "h-8 px-3.5 text-xs rounded-full",
        md: "h-9 px-4 text-sm rounded-full",
        lg: "h-11 px-6 text-base rounded-full",
        icon: "size-11 rounded-full p-0",
        "icon-sm": "size-9 rounded-full p-0",
        "icon-lg": "size-12 rounded-full p-0",
      },
    },
    defaultVariants: { variant: "primary", size: "md" },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
  loading?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, loading, children, disabled, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return (
      <Comp
        ref={ref}
        className={cn(buttonVariants({ variant, size }), className)}
        aria-busy={loading || undefined}
        disabled={disabled || loading}
        {...props}
      >
        {loading ? (
          <>
            <span
              aria-hidden="true"
              className="size-icon-xs animate-spin rounded-full border-2 border-current border-t-transparent"
            />
            <span className="sr-only">Loading</span>
            {children}
          </>
        ) : (
          children
        )}
      </Comp>
    );
  },
);
Button.displayName = "Button";

export { Button, buttonVariants };