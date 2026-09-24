/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./src/**/*.{js,jsx,ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: [
          "-apple-system",
          "BlinkMacSystemFont",
          "SF Pro Text",
          "Segoe UI",
          "system-ui",
          "sans-serif",
        ],
        mono: [
          "ui-monospace",
          "SF Mono",
          "SFMono-Regular",
          "Menlo",
          "Consolas",
          "monospace",
        ],
      },
      colors: {
        // Warm paper neutrals — low-chroma warm hue, never pure white/black.
        paper: {
          DEFAULT: "#fbfaf8",
          subtle: "#f6f4f0",
          muted: "#efede7",
          sunk: "#e8e5de",
        },
        // Neutral ramp chosen so every text-bearing step clears WCAG AA (4.5:1)
        // on the paper surfaces; ink-350/300 are reserved for disabled text,
        // placeholder text, and decorative glyphs only.
        // WARNING: bg-ink-*/<alpha> utilities do not compile under Tailwind
        // 3.1 (verified: the class silently vanishes from the bundle while
        // text-ink-* and other bg-*/<alpha> work). Scrims and translucent
        // ink fills must use inline rgba(28, 27, 25, a) instead.
        ink: {
          950: "#1c1b19",
          900: "#2b2a27",
          800: "#3d3c37",
          700: "#514f49",
          600: "#67655d",
          500: "#6f6d64",
          400: "#767469",
          350: "#8a887e",
          300: "#9d9a90",
          200: "#c9c6bd",
          100: "#e4e1db",
        },
        rule: {
          DEFAULT: "#e6e3dc",
          strong: "#d5d1c8",
        },
        // Deep, deliberate teal — the single action accent.
        accent: {
          50: "#eef7f6",
          100: "#d5ecea",
          200: "#aed9d6",
          300: "#7fbfbc",
          400: "#529e9c",
          500: "#3c8280",
          600: "#2e6866",
          700: "#295453",
          800: "#244443",
          900: "#1f3a39",
        },
        // Emerald is reserved for the profit/success semantic.
        success: {
          50: "#eef6f1",
          100: "#d6e9df",
          200: "#aed3bf",
          300: "#7fb798",
          400: "#529873",
          500: "#387d59",
          600: "#2a6346",
          700: "#244f39",
          800: "#1f4030",
          900: "#1a3528",
        },
        // Amber marks "internal only" classification, not a warning state.
        internal: {
          50: "#fbf5e9",
          100: "#f4e8cd",
          200: "#e6d0a1",
          300: "#d3b071",
          400: "#b98f4b",
          500: "#9c7337",
          600: "#7d5b2d",
          700: "#644926",
          800: "#523c21",
          900: "#45321d",
        },
        danger: {
          50: "#fbf0ef",
          100: "#f6dad7",
          200: "#ecb8b3",
          300: "#dd8b84",
          400: "#c95f56",
          500: "#ad453d",
          600: "#8d3730",
          700: "#722e29",
          800: "#5e2926",
          900: "#4e2422",
        },
      },
      fontSize: {
        // 5-step scale, >=1.25 ratio between display steps.
        micro: ["10.5px", { lineHeight: "14px", letterSpacing: "0.08em" }],
        caption: ["11.5px", { lineHeight: "16px" }],
        small: ["12.5px", { lineHeight: "18px" }],
        body: ["13.5px", { lineHeight: "21px" }],
        lead: ["15px", { lineHeight: "23px" }],
        title: ["19px", { lineHeight: "26px", letterSpacing: "-0.02em" }],
        display: ["26px", { lineHeight: "32px", letterSpacing: "-0.03em" }],
      },
      // Tailwind 3.1 ships no numeric min-height scale, so `min-h-10` and
      // friends silently produced nothing. Define them here; the default
      // full/screen/min/max/fit values are still merged in.
      minHeight: {
        6: "24px",
        7: "28px",
        8: "32px",
        9: "36px",
        10: "40px",
        11: "44px",
        12: "48px",
        14: "56px",
      },
      boxShadow: {
        card: "0 1px 2px rgba(60, 52, 34, 0.04), 0 0 0 1px rgba(230, 227, 220, 0.9)",
        pop: "0 12px 32px rgba(50, 44, 30, 0.12), 0 2px 8px rgba(50, 44, 30, 0.06)",
        rail: "0 -1px 0 rgba(230, 227, 220, 0.9)",
      },
      transitionTimingFunction: {
        "out-quart": "cubic-bezier(0.25, 1, 0.5, 1)",
        "out-quint": "cubic-bezier(0.22, 1, 0.36, 1)",
        "out-expo": "cubic-bezier(0.16, 1, 0.3, 1)",
      },
      keyframes: {
        "item-in": {
          from: { opacity: "0", transform: "translateY(4px)" },
          to: { opacity: "1", transform: "translateY(0)" },
        },
        "status-in": {
          from: { opacity: "0", transform: "translateY(-4px)" },
          to: { opacity: "1", transform: "translateY(0)" },
        },
        "toast-in": {
          from: { opacity: "0", transform: "translateY(-6px)" },
          to: { opacity: "1", transform: "translateY(0)" },
        },
      },
      animation: {
        "item-in": "item-in 200ms cubic-bezier(0.25, 1, 0.5, 1) both",
        "status-in": "status-in 180ms cubic-bezier(0.25, 1, 0.5, 1) both",
        "toast-in": "toast-in 180ms cubic-bezier(0.25, 1, 0.5, 1) both",
      },
    },
  },
  plugins: [],
};
