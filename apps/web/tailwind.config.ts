import type { Config } from "tailwindcss";
import defaultTheme from "tailwindcss/defaultTheme";

/**
 * Tailwind CSS v3 configuration (kept on v3 deliberately for a simple config).
 *
 * Brand tokens come from two prototypes:
 *   - sign-in: a warm milk-white page, the SafariStack ink navy sampled from the
 *     splash artwork, and the Atlas action blue (`milk`, `ink`, `atlas`);
 *   - the workspace screens (`images/Atlas Design – 0x`): a cooler canvas, a
 *     navy sidebar, paper cards, and a muted text scale (`canvas`, `navy`,
 *     `night`, `muted`, `line`, `sky`).
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
          ink: "#1A5FD6",
          hover: "#155DD8",
          line: "#2F74EA",
          focus: "#79B8FF",
          link: "#5FB2FF",
          "link-hover": "#A6D4FF",
          danger: "#FF8F7A",
          "danger-text": "#FFB3A5",
        },
        /** Workspace shell. */
        canvas: "#F8F7F3",
        navy: {
          DEFAULT: "#00204D",
          soft: "#9DB2D6",
          dim: "#7D95BD",
          text: "#D5E1F5",
          lead: "#C9D6EE",
        },
        sky: "#5FB2FF",
        /** Primary text on paper. */
        night: "#0B1B3A",
        muted: "#5B6B86",
        line: {
          DEFAULT: "#E6E8EE",
          strong: "#D9DEE8",
        },
      },
      fontFamily: {
        sans: ["var(--font-outfit)", ...defaultTheme.fontFamily.sans],
      },
      borderRadius: {
        card: "18px",
        control: "12px",
      },
      boxShadow: {
        card: "0 1px 2px rgba(0,32,77,.04), 0 10px 30px rgba(0,32,77,.05)",
        tile: "0 1px 2px rgba(0,32,77,.05), 0 6px 16px rgba(0,32,77,.05)",
        lift: "0 1px 2px rgba(0,32,77,.05), 0 10px 24px rgba(26,108,245,.12)",
        action: "0 6px 16px rgba(26,108,245,.24)",
        drawer: "-24px 0 60px rgba(0,32,77,.22)",
        pop: "0 14px 40px rgba(0,32,77,.3)",
      },
      keyframes: {
        "toast-in": {
          from: { opacity: "0", transform: "translate(-50%, 12px)" },
          to: { opacity: "1", transform: "translate(-50%, 0)" },
        },
        "drawer-in": {
          from: { transform: "translateX(24px)", opacity: "0" },
          to: { transform: "translateX(0)", opacity: "1" },
        },
        "fade-in": {
          from: { opacity: "0" },
          to: { opacity: "1" },
        },
      },
      animation: {
        "toast-in": "toast-in .18s ease-out",
        "drawer-in": "drawer-in .2s ease-out",
        "fade-in": "fade-in .15s ease-out",
      },
    },
  },
  plugins: [],
};

export default config;
