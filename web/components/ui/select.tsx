import * as React from "react";
import { ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

export type SelectOption = { value: string; label: React.ReactNode; disabled?: boolean };

type Props = Omit<React.SelectHTMLAttributes<HTMLSelectElement>, "onChange"> & {
  options?: SelectOption[];
  placeholder?: string;
  onValueChange?: (v: string) => void;
  onChange?: React.ChangeEventHandler<HTMLSelectElement>;
};

/** Styled native select: accessible and works on every device. */
export const Select = React.forwardRef<HTMLSelectElement, Props>(function Select(
  { className, options, placeholder, children, onValueChange, onChange, ...props },
  ref,
) {
  return (
    <div className={cn("relative", className)}>
      <select
        ref={ref}
        className="flex h-9 w-full cursor-pointer appearance-none rounded-md border border-input bg-card py-1 pl-3 pr-8 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/40 focus-visible:border-ring disabled:cursor-not-allowed disabled:opacity-50"
        onChange={(e) => {
          onChange?.(e);
          onValueChange?.(e.target.value);
        }}
        {...props}
      >
        {placeholder !== undefined && <option value="">{placeholder}</option>}
        {options?.map((o) => (
          <option key={o.value} value={o.value} disabled={o.disabled}>
            {typeof o.label === "string" ? o.label : o.value}
          </option>
        ))}
        {children}
      </select>
      <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
    </div>
  );
});
