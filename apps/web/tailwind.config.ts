import type { Config } from "tailwindcss";

/**
 * Tailwind CSS v3 configuration (kept on v3 deliberately for a simple config).
 */
const config: Config = {
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
};

export default config;
