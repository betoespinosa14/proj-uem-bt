import { BrandMark } from "@/components/common";
import { APP_DESCRIPTION, APP_NAME } from "@/lib/config";

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="bg-brand-glow flex min-h-screen flex-col items-center justify-center px-4 py-12">
      <div className="mb-8 flex flex-col items-center text-center motion-safe:animate-fade-in">
        <span className="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-white/5 ring-1 ring-white/10 backdrop-blur">
          <BrandMark className="h-10" />
        </span>
        <h1 className="text-3xl font-bold tracking-[0.22em] text-white">{APP_NAME}</h1>
        <p className="mt-2 max-w-xs text-sm text-white/60">{APP_DESCRIPTION}</p>
      </div>
      <div className="w-full max-w-sm motion-safe:animate-slide-in">{children}</div>
    </div>
  );
}
