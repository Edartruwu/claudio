/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./internal/web/templates/**/*.templ'],
  darkMode: ['class', '[data-theme="dark"]'],
  safelist: ['z-[9999]', 'z-[100]', 'min-h-[44px]', 'w-[280px]', 'border-l-2', 'border-[--color-session]', 'bg-base-100', 'bg-base-200', 'bg-base-300', 'border-base-300'],
  theme: { extend: {} },
  plugins: [require('daisyui')],
  daisyui: { themes: ['light', 'dark'], logs: false },
}
