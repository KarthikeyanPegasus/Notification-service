'use client'

import React from 'react'
import { PageHeader } from '@/components/shared/page-header'
import { ApiDocsSettings } from '@/components/settings/api-docs-settings'
import { useUserRole } from '@/hooks/use-user-role'

export default function ApiSettingsPage() {
  const role = useUserRole()
  
  if (role !== 'admin') {
    return (
      <div className="p-8 text-center">
        <h1 className="text-2xl font-bold text-destructive">Unauthorized</h1>
        <p className="text-muted-foreground mt-2">Only administrators can access this page.</p>
      </div>
    )
  }

  return (
    <div className="max-w-6xl mx-auto space-y-10 py-10 px-4 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <PageHeader 
        title="API Documentation Settings" 
        description="Manage visibility and security settings for the public API Reference." 
      />
      
      <div className="grid gap-10">
        <ApiDocsSettings />
      </div>
    </div>
  )
}
