/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./src/**/*.{js,jsx,ts,tsx}",
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ['SF Pro Display', 'SF Pro', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'sans-serif'],
      },
      transitionTimingFunction: {
        'out-expo': 'cubic-bezier(0.16, 1, 0.3, 1)',
        'out-quint': 'cubic-bezier(0.22, 1, 0.36, 1)',
        'in-out': 'cubic-bezier(0.77, 0, 0.175, 1)',
      },
    },
  },
  plugins: [],
}
