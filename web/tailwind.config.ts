import type { Config } from "tailwindcss";

const c = (v: string) => `hsl(var(--${v}) / <alpha-value>)`;

const config: Config = {
  darkMode: "class",
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}", "./lib/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        background: c("background"),
        foreground: c("foreground"),
        muted: { DEFAULT: c("muted"), foreground: c("muted-foreground") },
        card: { DEFAULT: c("card"), foreground: c("card-foreground") },
        popover: { DEFAULT: c("popover"), foreground: c("popover-foreground") },
        border: c("border"),
        input: c("input"),
        ring: c("ring"),
        accent: { DEFAULT: c("accent"), foreground: c("accent-foreground") },
        primary: { DEFAULT: c("primary"), foreground: c("primary-foreground") },
        secondary: { DEFAULT: c("secondary"), foreground: c("secondary-foreground") },
        destructive: { DEFAULT: c("destructive"), foreground: c("destructive-foreground") },
        success: c("success"),
        warning: c("warning"),
        sidebar: c("sidebar"),
        brand: { DEFAULT: c("brand"), deep: c("brand-deep") },
        neon: c("neon"),
        edge: c("edge"),
        rail: { DEFAULT: c("rail"), foreground: c("rail-foreground") },
        nav: { DEFAULT: c("nav"), foreground: c("nav-foreground"), accent: c("nav-accent") },
        tint: { mint: c("tint-mint"), sky: c("tint-sky"), sand: c("tint-sand") },
      },
      borderRadius: { "2xl": "1.25rem", xl: "1rem", lg: "0.75rem", md: "0.5rem", sm: "0.375rem" },
      boxShadow: {
        card: "0 1px 2px hsl(var(--shadow) / 0.04), 0 4px 16px -4px hsl(var(--shadow) / 0.08)",
        lift: "0 2px 4px hsl(var(--shadow) / 0.05), 0 12px 28px -8px hsl(var(--shadow) / 0.16)",
        frame: "0 8px 40px -12px hsl(0 0% 0% / 0.45)",
      },
      fontFamily: {
        sans: ["var(--font-sans)", "ui-sans-serif", "system-ui", "-apple-system", "Segoe UI", "Roboto", "sans-serif"],
        mono: ["ui-monospace", "SFMono-Regular", "Menlo", "Consolas", "Liberation Mono", "monospace"],
      },
      keyframes: {
        "fade-in": { from: { opacity: "0" }, to: { opacity: "1" } },
        "zoom-in": { from: { opacity: "0", transform: "scale(.97)" }, to: { opacity: "1", transform: "scale(1)" } },
        "slide-in": { from: { opacity: "0", transform: "translateY(8px)" }, to: { opacity: "1", transform: "translateY(0)" } },
        shimmer: { from: { backgroundPosition: "200% 0" }, to: { backgroundPosition: "-200% 0" } },
      },
      animation: {
        "fade-in": "fade-in .15s ease-out",
        "zoom-in": "zoom-in .15s ease-out",
        "slide-in": "slide-in .25s ease-out",
        shimmer: "shimmer 1.6s linear infinite",
      },
    },
  },
  plugins: [],
};
export default config;
