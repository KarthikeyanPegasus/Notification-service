import { PageHeader } from '@/components/shared/page-header'
import { NotificationsClient } from './notifications-client'

export const metadata = {
  title: 'Notifications — NotifyHub',
}

export default function NotificationsPage() {
  return <NotificationsClient />
}
