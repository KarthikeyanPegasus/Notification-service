'use client'

import React from 'react'
import { APIDocs } from '@/components/docs/api-docs'

export default function DocsPage() {
  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-2">
        <h1 className="text-3xl font-bold tracking-tight">API Documentation</h1>
        <p className="text-muted-foreground">
          Explore and integrate with the NotifyHub API. 
          Use your service tokens for authentication.
        </p>
      </div>

      <APIDocs />
    </div>
  )
}
