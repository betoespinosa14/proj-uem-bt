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
      },
      borderRadius: { lg: "0.625rem", md: "0.5rem", sm: "0.375rem" },
      fontFamily: {
        sans: ["ui-sans-serif", "system-ui", "-apple-system", "Segoe UI", "Roboto", "sans-serif"],
        mono: ["ui-monospace", "SFMono-Regular", "Menlo", "Consolas", "Liberation Mono", "monospace"],
      },
      keyframes: {
        "fade-in": { from: { opacity: "0" }, to: { opacity: "1" } },
        "zoom-in": { from: { opacity: "0", transform: "scale(.97)" }, to: { opacity: "1", transform: "scale(1)" } },
        "slide-in": { from: { opacity: "0", transform: "translateY(8px)" }, to: { opacity: "1", transform: "translateY(0)" } },
      },
      animation: {
        "fade-in": "fade-in .15s ease-out",
        "zoom-in": "zoom-in .15s ease-out",
        "slide-in": "slide-in .2s ease-out",
      },
    },
  },
  plugins: [],
};
export default config;
