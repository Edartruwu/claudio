/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./internal/web/templates/**/*.templ'],
  darkMode: ['class', '[data-theme="dark"]'],
  safelist: ['z-[9999]', 'z-[100]', 'min-h-[44px]'],
  theme: { extend: {} },
  plugins: [require('daisyui')],
  daisyui: { themes: ['light', 'dark'], logs: false },
}
