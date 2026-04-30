/** @type {import('next').NextConfig} */
const path = require('path')

const nextConfig = {
  reactStrictMode: true,
  output: 'standalone',
  typedRoutes: false,
  // Prevent monorepo root inference issues when multiple lockfiles exist.
  outputFileTracingRoot: path.join(__dirname),
  async rewrites() {
    // Proxy API calls through Next.js to avoid browser -> localhost issues
    // (IPv6/IPv4 resolution, CORS, mixed-content, containerized dev).
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
