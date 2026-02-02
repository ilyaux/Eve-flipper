import type { Config } from "tailwindcss";

const config: Config = {
  darkMode: "class",
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        eve: {
          dark: "#0d0d0d",
          panel: "#1a1a1a",
          "panel-hover": "#222222",
          input: "#232323",
          accent: "#e69500",
          "accent-hover": "#f0a500",
          "accent-dim": "#b37400",
          text: "#c0c0c0",
          dim: "#8c8c8c",
          success: "#00b450",
          error: "#dc3c3c",
          border: "#2a2a2a",
          "border-light": "#3a3a3a",
          glow: "rgba(230, 149, 0, 0.15)",
        },
      },
      fontFamily: {
        eve: ['"Exo 2"', "Consolas", "Monaco", "monospace"],
        mono: ["Consolas", "Monaco", "Courier New", "monospace"],
      },
      boxShadow: {
        "eve-glow": "0 0 8px rgba(230, 149, 0, 0.15)",
        "eve-glow-strong": "0 0 16px rgba(230, 149, 0, 0.3)",
        "eve-inset": "inset 0 1px 3px rgba(0, 0, 0, 0.5)",
      },
    },
  },
  plugins: [],
};
export default config;
