'use client'

import { APIDocs } from '@/components/docs/api-docs'

export default function ApiDocsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">API Reference</h1>
        <p className="text-muted-foreground">
          Explore and integrate with the NotifyHub REST API. Authenticate with a JWT bearer token or an API key
          (<code className="text-xs">X-API-Key</code>). OTP endpoints require a service token
          (<code className="text-xs">X-Service-Token</code>).
        </p>
      </div>
      <APIDocs />
    </div>
  )
}
