'use client'

import { useEffect, useState } from 'react'
import { PageHeader } from '@/components/shared/page-header'
import { NotificationsTable } from '@/components/notifications/notifications-table'
import { ClientScopeSelect } from '@/components/shared/client-scope-select'
import { listMyClients, type ApiClientKey } from '@/lib/api'

export function NotificationsClient() {
  const [apiKeyId, setApiKeyId] = useState<string | undefined>(undefined)
  const [clients, setClients] = useState<ApiClientKey[]>([])

  useEffect(() => {
    ;(async () => {
      try {
        const ks = await listMyClients()
        setClients(ks ?? [])
      } catch {
        setClients([])
      }
    })()
  }, [])

  return (
    <div>
      <PageHeader
        title="Notifications"
        description="Explore, filter, and inspect all notifications"
        breadcrumbs={[{ label: 'Dashboard', href: '/dashboard' }, { label: 'Notifications' }]}
        actions={<ClientScopeSelect className="w-72" onScopeChange={setApiKeyId} />}
      />
      <NotificationsTable apiKeyId={apiKeyId} clients={clients} />
    </div>
  )
}

