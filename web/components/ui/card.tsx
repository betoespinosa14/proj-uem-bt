import * as React from "react";
import { cn } from "@/lib/utils";

/** Card surfaces. "mint" / "sky" / "sand" are pastel panels for grouping content inside a page or card. */
export type CardTone = "default" | "mint" | "sky" | "sand";

const tones: Record<CardTone, string> = {
  default: "bg-card shadow-card border border-edge/70 dark:border-edge dark:shadow-none",
  mint: "bg-tint-mint border border-edge/70 dark:border-edge",
  sky: "bg-tint-sky border border-edge/70 dark:border-edge",
  sand: "bg-tint-sand border border-edge/70 dark:border-edge",
};

export function Card({ className, tone = "default", ...props }: React.HTMLAttributes<HTMLDivElement> & { tone?: CardTone }) {
  return <div className={cn("rounded-2xl text-card-foreground", tones[tone], className)} {...props} />;
}
export function CardHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("flex flex-col gap-1 px-5 pt-5 pb-3", className)} {...props} />;
}
export function CardTitle({ className, ...props }: React.HTMLAttributes<HTMLHeadingElement>) {
  return <h3 className={cn("text-[15px] font-semibold leading-tight tracking-tight", className)} {...props} />;
}
export function CardDescription({ className, ...props }: React.HTMLAttributes<HTMLParagraphElement>) {
  return <p className={cn("text-xs text-muted-foreground", className)} {...props} />;
}
export function CardContent({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("px-5 pb-5", className)} {...props} />;
}
export function CardFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("flex items-center border-t border-border/70 px-5 py-3", className)} {...props} />;
}
