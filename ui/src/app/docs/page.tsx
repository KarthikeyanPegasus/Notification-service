'use client'

import React from 'react'
import { APIDocs } from '@/components/docs/api-docs'

export default function DocsPage() {
  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-2">
        <h1 className="text-3xl font-bold tracking-tight">API Documentation</h1>
        <p className="text-muted-foreground">
          Explore and integrate with the NotifyHub API. The spec is loaded from your API base URL
          (<code className="text-xs">NEXT_PUBLIC_API_URL</code>). Use JWT or service tokens as required
          per route. Channels include <code className="text-xs">slack</code> (webhook URL in{' '}
          <code className="text-xs">recipient</code>, message in <code className="text-xs">body</code> or a template).
        </p>
      </div>

      <APIDocs />
    </div>
  )
}
