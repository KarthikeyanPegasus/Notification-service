import { PageHeader } from '@/components/shared/page-header'
import { NotificationsTable } from '@/components/notifications/notifications-table'

export const metadata = {
  title: 'Notifications — NotifyHub',
}

export default function NotificationsPage() {
  return (
    <div>
      <PageHeader
        title="Notifications"
        description="Explore, filter, and inspect all notifications"
        breadcrumbs={[{ label: 'Dashboard', href: '/dashboard' }, { label: 'Notifications' }]}
      />
      <NotificationsTable />
    </div>
  )
}
