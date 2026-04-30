'use client'

import { PageHeader } from '@/components/shared/page-header'
import { Button } from '@/components/ui/button'

export default function IngressPage() {
  return (
    <div className="space-y-6 animate-in fade-in duration-500">
      <PageHeader
        title="Ingress settings moved"
        description="Ingress settings are now configured per-client. Open Client management and click the settings icon on a client."
      />
      <Button asChild>
        <a href="/clients">Go to Client management</a>
      </Button>
    </div>
  )
}

