import * as React from "react";
import { cn } from "@/lib/utils";

export const Textarea = React.forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(
  function Textarea({ className, ...props }, ref) {
    return (
      <textarea
        ref={ref}
        className={cn(
          "flex min-h-[80px] w-full rounded-md border border-input bg-card px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/40 focus-visible:border-ring disabled:cursor-not-allowed disabled:opacity-50",
          className,
        )}
        {...props}
      />
    );
  },
);

/** Monospace code editor: textarea with tab support. */
export const CodeEditor = React.forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(
  function CodeEditor({ className, onKeyDown, ...props }, ref) {
    return (
      <Textarea
        ref={ref}
        spellCheck={false}
        autoCapitalize="off"
        autoCorrect="off"
        className={cn("min-h-[260px] font-mono text-[13px] leading-relaxed bg-muted/40", className)}
        onKeyDown={(e) => {
          if (e.key === "Tab" && !e.shiftKey && !e.metaKey && !e.ctrlKey) {
            e.preventDefault();
            const ta = e.currentTarget;
            const { selectionStart: s, selectionEnd: end, value } = ta;
            const next = value.slice(0, s) + "  " + value.slice(end);
            // Use the native setter so React's onChange fires.
            const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
            setter?.call(ta, next);
            ta.dispatchEvent(new Event("input", { bubbles: true }));
            ta.selectionStart = ta.selectionEnd = s + 2;
          }
          onKeyDown?.(e);
        }}
        {...props}
      />
    );
  },
);
