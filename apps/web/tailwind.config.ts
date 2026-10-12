import type { Config } from "tailwindcss";
import defaultTheme from "tailwindcss/defaultTheme";

/**
 * Tailwind CSS v3 configuration (kept on v3 deliberately for a simple config).
 *
 * Brand tokens come from the sign-in prototype: a warm milk-white page, the
 * SafariStack ink navy sampled from the splash artwork, and the Atlas action
 * blue.
 */
const config: Config = {
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        milk: {
          DEFAULT: "#FBF8F1",
          50: "#FFFEFB",
          100: "#FDFBF6",
          200: "#F4EFE3",
        },
        /** Exactly the splash navy, so the halo behind forms is seamless. */
        ink: {
          DEFAULT: "#001F4D",
          field: "rgba(2, 14, 42, 0.55)",
          muted: "#C9D6F0",
          hint: "#B9C8E6",
        },
        atlas: {
          DEFAULT: "#1A6CF5",
          line: "#2F74EA",
          focus: "#79B8FF",
          link: "#5FB2FF",
          "link-hover": "#A6D4FF",
          danger: "#FF8F7A",
          "danger-text": "#FFB3A5",
        },
      },
      fontFamily: {
        sans: ["var(--font-outfit)", ...defaultTheme.fontFamily.sans],
      },
    },
  },
  plugins: [],
};

export default config;
