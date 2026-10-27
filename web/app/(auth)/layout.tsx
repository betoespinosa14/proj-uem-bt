import { ShieldCheck } from "lucide-react";
import { APP_DESCRIPTION, APP_NAME } from "@/lib/config";

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-muted/40 px-4 py-12">
      <div className="mb-6 flex flex-col items-center text-center">
        <span className="mb-3 flex h-10 w-10 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
          <ShieldCheck className="h-5 w-5" />
        </span>
        <h1 className="text-lg font-semibold tracking-tight">{APP_NAME}</h1>
        <p className="text-sm text-muted-foreground">{APP_DESCRIPTION}</p>
      </div>
      <div className="w-full max-w-sm">{children}</div>
    </div>
  );
}
