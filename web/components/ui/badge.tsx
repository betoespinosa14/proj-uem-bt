import * as React from "react";
import { cn } from "@/lib/utils";

const variants = {
  default: "bg-muted text-foreground border-border",
  primary: "bg-primary/10 text-primary border-primary/20",
  success: "bg-success/10 text-success border-success/25",
  warning: "bg-warning/10 text-warning border-warning/25",
  danger: "bg-destructive/10 text-destructive border-destructive/25",
  neutral: "bg-muted text-muted-foreground border-border",
  outline: "bg-transparent text-foreground border-border",
} as const;

export type BadgeVariant = keyof typeof variants;

export function Badge({
  className,
  variant = "default",
  dot,
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & { variant?: BadgeVariant; dot?: boolean }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border px-2 py-0.5 text-xs font-medium",
        variants[variant],
        className,
      )}
      {...props}
    >
      {dot && <span className="h-1.5 w-1.5 rounded-full bg-current" />}
      {props.children}
    </span>
  );
}
