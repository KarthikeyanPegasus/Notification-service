'use client'

import React, { use } from 'react'
import Link from 'next/link'
import { PageHeader } from '@/components/shared/page-header'
import { Button } from '@/components/ui/button'
import { ArrowLeft } from 'lucide-react'
import { IngressSettings } from '@/components/clients/ingress-settings'
import { WorkflowOrchestrationSettings } from '@/components/clients/workflow-orchestration-settings'
import { WorkersPerChannelSettings } from '@/components/clients/workers-per-channel-settings'

interface Props {
  params: Promise<{ id: string }>
}

export default function ClientSettingsPage({ params }: Props) {
  const { id } = use(params)

  return (
    <div className="space-y-6">
      <PageHeader
        title="Client settings"
        description="Manage configuration scoped to this client (API key)."
        breadcrumbs={[
          { label: 'Dashboard', href: '/dashboard' },
          { label: 'Client management', href: '/clients' },
          { label: id },
        ]}
        actions={
          <Button variant="outline" asChild>
            <Link href="/clients">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back
            </Link>
          </Button>
        }
      />

      <WorkflowOrchestrationSettings apiKeyId={id} />
      <WorkersPerChannelSettings apiKeyId={id} />
      <IngressSettings apiKeyId={id} />
    </div>
  )
}

