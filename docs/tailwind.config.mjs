/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{astro,html,js,jsx,md,mdx,svelte,ts,tsx,vue}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Primary gradient colors
        primary: {
          DEFAULT: '#818cf8',
          50: '#eef2ff',
          100: '#e0e7ff',
          200: '#c7d2fe',
          300: '#a5b4fc',
          400: '#818cf8',
          500: '#6366f1',
          600: '#4f46e5',
          700: '#4338ca',
          800: '#3730a3',
          900: '#312e81',
          950: '#1e1b4b',
        },
        violet: {
          DEFAULT: '#a78bfa',
          glow: 'rgba(167, 139, 250, 0.4)',
        },
        purple: {
          DEFAULT: '#c084fc',
          glow: 'rgba(192, 132, 252, 0.4)',
        },
        // Surface colors (dark mode)
        surface: {
          DEFAULT: '#0f0f17',
          50: '#18181f',
          100: '#1e1e28',
          200: '#24242f',
          300: '#2a2a36',
          400: '#32323e',
          500: '#3a3a47',
        },
        // Terminal Catppuccin Mocha
        terminal: {
          bg: '#1e1e2e',
          header: '#313244',
          text: '#cdd6f4',
          green: '#a6e3a1',
          yellow: '#f9e2af',
          red: '#f38ba8',
          blue: '#89b4fa',
          purple: '#cba6f7',
          teal: '#94e2d5',
          pink: '#f5c2e7',
        },
        // Glass colors
        glass: {
          bg: 'rgba(255, 255, 255, 0.03)',
          border: 'rgba(255, 255, 255, 0.08)',
          hover: 'rgba(255, 255, 255, 0.06)',
        },
      },
      fontFamily: {
        sans: ['Inter Variable', 'Inter', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },
      backdropBlur: {
        xs: '2px',
      },
      boxShadow: {
        glow: '0 0 20px rgba(129, 140, 248, 0.3)',
        'glow-lg': '0 0 40px rgba(129, 140, 248, 0.4)',
        'glow-violet': '0 0 30px rgba(167, 139, 250, 0.4)',
        glass: '0 4px 30px rgba(0, 0, 0, 0.3)',
      },
      animation: {
        'gradient-flow': 'gradient-flow 4s ease infinite',
        'glow': 'glow 3s ease-in-out infinite',
        'float': 'float 6s ease-in-out infinite',
        'fade-in': 'fade-in 0.3s ease forwards',
        'slide-up': 'slide-up 0.8s ease-out',
        'blink': 'blink 1s step-end infinite',
      },
      typography: {
        DEFAULT: {
          css: {
            '--tw-prose-body': '#cdd6f4',
            '--tw-prose-headings': '#f5f5f5',
            '--tw-prose-lead': '#a6adc8',
            '--tw-prose-links': '#818cf8',
            '--tw-prose-bold': '#f5f5f5',
            '--tw-prose-counters': '#a6adc8',
            '--tw-prose-bullets': '#6c7086',
            '--tw-prose-hr': '#313244',
            '--tw-prose-quotes': '#cdd6f4',
            '--tw-prose-quote-borders': '#818cf8',
            '--tw-prose-captions': '#a6adc8',
            '--tw-prose-code': '#cba6f7',
            '--tw-prose-pre-code': '#cdd6f4',
            '--tw-prose-pre-bg': '#1e1e2e',
            '--tw-prose-th-borders': '#313244',
            '--tw-prose-td-borders': '#24242f',
            maxWidth: 'none',
            code: {
              backgroundColor: 'rgba(203, 166, 247, 0.1)',
              padding: '0.2em 0.4em',
              borderRadius: '0.25rem',
              fontWeight: '400',
            },
            'code::before': {
              content: '""',
            },
            'code::after': {
              content: '""',
            },
            a: {
              textDecoration: 'none',
              borderBottom: '1px solid rgba(129, 140, 248, 0.3)',
              transition: 'all 0.2s ease',
              '&:hover': {
                borderBottomColor: '#818cf8',
              },
            },
            pre: {
              backgroundColor: '#1e1e2e',
              border: '1px solid rgba(255, 255, 255, 0.08)',
              borderRadius: '0.75rem',
            },
            h1: {
              fontWeight: '800',
            },
            h2: {
              fontWeight: '700',
              marginTop: '2em',
            },
            h3: {
              fontWeight: '600',
            },
          },
        },
      },
    },
  },
  plugins: [
    require('@tailwindcss/typography'),
  ],
};
