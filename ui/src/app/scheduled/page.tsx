import { PageHeader } from '@/components/shared/page-header'
import { ScheduledTable } from '@/components/scheduled/scheduled-table'

export const metadata = {
  title: 'Scheduled — NotifyHub',
}

export default function ScheduledPage() {
  return (
    <div>
      <PageHeader
        title="Scheduled Notifications"
        description="Manage pending scheduled notifications — cancel or reschedule delivery"
        breadcrumbs={[
          { label: 'Dashboard', href: '/dashboard' },
          { label: 'Scheduled' },
        ]}
      />
      <ScheduledTable />
    </div>
  )
}
