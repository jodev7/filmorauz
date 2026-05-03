import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./src/pages/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/components/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      colors: {
        brand: {
          red: "#F97316",
          dark: "#0A0A0F",
          card: "#12121A",
          border: "#1E1E2E",
          muted: "#6B7280",
        },
      },
      fontFamily: {
        display: ["var(--font-display)"],
        body: ["var(--font-body)"],
      },
      // Bump max-w-7xl from Tailwind's 80rem (1280px) to 1440px so the global
      // page container is comfortable on 1440p displays without scaling.
      // Ultra-wide viewports (1920+/2560+/4K/8K) still cap here and center.
      maxWidth: {
        "7xl": "1440px",
      },
    },
  },
  plugins: [],
};

export default config;
