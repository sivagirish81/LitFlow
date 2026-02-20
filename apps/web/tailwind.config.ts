import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        sand: "#f5f1e8",
        ink: "#111318",
        brass: "#b88746",
        teal: "#2d6a6a"
      }
    }
  },
  plugins: []
};

export default config;
