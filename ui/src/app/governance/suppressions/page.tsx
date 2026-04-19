import { SuppressionTable } from '@/components/governance/suppression-table'

export default function SuppressionsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Suppression List</h1>
        <p className="text-muted-foreground">
          Manage blocked email addresses and phone numbers. Notifications to these recipients will be automatically rejected.
        </p>
      </div>

      <SuppressionTable />
    </div>
  )
}
