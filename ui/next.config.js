/** @type {import('next').NextConfig} */
const path = require('path')

const isDev = process.env.NODE_ENV === 'development'

const securityHeaders = [
  {
    key: 'Strict-Transport-Security',
    value: 'max-age=63072000; includeSubDomains; preload',
  },
  {
    key: 'X-Content-Type-Options',
    value: 'nosniff',
  },
  {
    key: 'X-Frame-Options',
    value: 'SAMEORIGIN',
  },
  {
    key: 'X-XSS-Protection',
    value: '1; mode=block',
  },
  {
    key: 'Referrer-Policy',
    value: 'strict-origin-when-cross-origin',
  },
  {
    key: 'Permissions-Policy',
    value: 'camera=(), microphone=(), geolocation=()',
  },
  {
    // CSP: allow Clerk (accounts.dev, clerk.com), same-origin scripts/styles, and inline
    // theme-detection script (unsafe-inline is narrowly required for the blocking theme script).
    key: 'Content-Security-Policy',
    value: [
      "default-src 'self'",
      `script-src 'self' 'unsafe-inline'${isDev ? " 'unsafe-eval'" : ''} https://*.clerk.accounts.dev https://clerk.com https://*.clerk.com`,
      "style-src 'self' 'unsafe-inline'",
      "img-src 'self' data: https:",
      "font-src 'self'",
      "connect-src 'self' https://*.clerk.accounts.dev https://clerk.com https://*.clerk.com",
      "frame-src 'self' https://*.clerk.accounts.dev https://clerk.com",
      "object-src 'none'",
      "base-uri 'self'",
      "form-action 'self'",
    ].join('; '),
  },
]

const nextConfig = {
  reactStrictMode: true,
  output: 'standalone',
  typedRoutes: false,
  // Prevent monorepo root inference issues when multiple lockfiles exist.
  outputFileTracingRoot: path.join(__dirname),
  async headers() {
    return [
      {
        source: '/(.*)',
        headers: securityHeaders,
      },
    ]
  },
  async rewrites() {
    // Proxy API calls through Next.js to avoid browser -> localhost issues
    // (IPv6/IPv4 resolution, CORS, mixed-content, containerised dev).
    const apiBase = process.env.NEXT_PUBLIC_API_URL || 'http://127.0.0.1:8080'
    return [
      {
        source: '/v1/:path*',
        destination: `${apiBase}/v1/:path*`,
      },
    ]
  },
}

module.exports = nextConfig
