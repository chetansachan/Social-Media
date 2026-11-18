"use client";

import { Toaster } from "sonner";
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Info,
  Loader2,
} from "lucide-react";
import { useTheme } from "@/lib/providers/theme-provider";

/**
 * Sonner, themed from the app tokens.
 *
 * One toaster for every screen: actions confirm here, failures explain here,
 * and nothing depends on a toast as the only copy of an error — inline states
 * still carry the details next to the control that failed.
 */
export function AppToaster() {
  const { theme } = useTheme();

  return (
    <Toaster
      theme={theme}
      position="top-center"
      offset={16}
      gap={10}
      closeButton
      visibleToasts={4}
      duration={3800}
      icons={{
        success: <CheckCircle2 className="size-icon-xs text-success" aria-hidden="true" />,
        error: <AlertCircle className="size-icon-xs text-danger" aria-hidden="true" />,
        info: <Info className="size-icon-xs text-accent" aria-hidden="true" />,
        warning: <AlertTriangle className="size-icon-xs text-accent-alt" aria-hidden="true" />,
        loading: <Loader2 className="size-icon-xs animate-spin text-accent" aria-hidden="true" />,
      }}
      toastOptions={{
        classNames: {
          toast:
            "!bg-[var(--surface-overlay)] !border !border-[var(--border)] !text-[var(--text-primary)] !rounded-[var(--radius-md)] !shadow-2 !font-sans !items-start !gap-2.5 !px-4 !py-3",
          title: "!text-sm !font-semibold !text-[var(--text-primary)]",
          description: "!text-xs !leading-snug !text-[var(--text-secondary)]",
          closeButton:
            "!bg-[var(--surface-raised)] !border-[var(--border)] !text-[var(--text-secondary)] hover:!text-[var(--text-primary)]",
          actionButton:
            "!bg-[var(--accent-solid)] !text-[var(--on-accent)] !rounded-full !text-xs !font-semibold",
          cancelButton:
            "!bg-[var(--surface-inset)] !text-[var(--text-primary)] !rounded-full !text-xs",
          success: "!border-[color-mix(in_srgb,var(--text-success)_45%,var(--border))]",
          error: "!border-[color-mix(in_srgb,var(--text-danger)_45%,var(--border))]",
          info: "!border-[color-mix(in_srgb,var(--accent-solid)_45%,var(--border))]",
          warning: "!border-[color-mix(in_srgb,var(--text-accent-alt)_45%,var(--border))]",
        },
      }}
    />
  );
}