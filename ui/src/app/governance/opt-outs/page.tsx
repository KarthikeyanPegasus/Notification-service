import { OptOutTable } from '@/components/governance/opt-out-table'

export default function OptOutsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Opt-out Management</h1>
        <p className="text-muted-foreground">
          View and manage user-level channel subscriptions. Users on this list will not receive common notifications on the specified channels.
        </p>
      </div>

      <OptOutTable />
    </div>
  )
}
