import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { DollarSign, AlertCircle, CheckCircle2, Wallet } from 'lucide-react'
import type { VendorBilling } from '@/types'

interface VendorBillingCardProps {
  billing: VendorBilling
}

function fmt(usd: number, currency = 'USD'): string {
  if (usd === 0) return currency === 'USD' ? '$0.00' : `0.00 ${currency}`
  const prefix = currency === 'USD' ? '$' : ''
  const suffix = currency === 'USD' ? '' : ` ${currency}`
  if (usd < 0.01) return `${prefix}${usd.toFixed(5)}${suffix}`
  if (usd >= 1000) return `${prefix}${usd.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}${suffix}`
  return `${prefix}${usd.toFixed(2)}${suffix}`
}

export function VendorBillingCard({ billing }: VendorBillingCardProps) {
  const hasError = !!billing.error
  const isFree = billing.cost_per_msg === 0 && billing.total_cost === 0 && !billing.balance

  return (
    <Card className={cn(
      'border shadow-sm transition-colors',
      hasError ? 'border-destructive/30 bg-destructive/5' : 'bg-muted/30 hover:bg-muted/50'
    )}>
      <CardHeader className="pb-2 pt-4 px-4 flex flex-row items-center justify-between space-y-0">
        <div className="flex items-center gap-2">
          <div className={cn(
            'p-1.5 rounded-full',
            isFree ? 'bg-emerald-500/10 text-emerald-500'
              : hasError ? 'bg-red-500/10 text-red-500'
              : 'bg-amber-500/10 text-amber-500'
          )}>
            <DollarSign size={14} />
          </div>
          <CardTitle className="text-xs font-bold uppercase tracking-wider text-muted-foreground">
            {billing.provider}
          </CardTitle>
        </div>
        <Badge
          variant={billing.is_actual ? 'success' : 'secondary'}
          className="text-[9px] h-4 px-1.5 leading-none"
        >
          {billing.is_actual ? 'actual' : 'estimated'}
        </Badge>
      </CardHeader>

      <CardContent className="px-4 pb-4 space-y-3">
        {hasError ? (
          <div className="flex items-start gap-2 text-destructive">
            <AlertCircle size={14} className="mt-0.5 shrink-0" />
            <p className="text-xs leading-tight">{billing.error}</p>
          </div>
        ) : (
          <>
            {/* Primary figure */}
            <div className="space-y-1">
              {billing.balance !== undefined ? (
                <>
                  <p className="text-[10px] font-bold uppercase text-muted-foreground flex items-center gap-1">
                    <Wallet size={10} /> Remaining Balance
                  </p>
                  <p className={cn(
                    'text-2xl font-black tabular-nums tracking-tight',
                    billing.balance > 5 ? 'text-emerald-500'
                      : billing.balance > 1 ? 'text-yellow-500'
                      : 'text-red-500'
                  )}>
                    {fmt(billing.balance, billing.currency)}
                  </p>
                </>
              ) : isFree ? (
                <>
                  <p className="text-[10px] font-bold uppercase text-muted-foreground">Total Cost</p>
                  <p className="text-2xl font-black tabular-nums tracking-tight text-emerald-500">
                    Free
                  </p>
                </>
              ) : (
                <>
                  <p className="text-[10px] font-bold uppercase text-muted-foreground">
                    Total Cost ({billing.period === 'all_time' ? 'All Time' : billing.period === 'this_month' ? 'This Month' : billing.period})
                  </p>
                  <p className="text-2xl font-black tabular-nums tracking-tight text-amber-500">
                    {fmt(billing.total_cost, billing.currency)}
                  </p>
                </>
              )}
            </div>

            {/* Stats row */}
            <div className="flex items-center gap-3 text-[10px] text-muted-foreground border-t pt-2">
              {billing.total_count > 0 && (
                <span className="flex items-center gap-1">
                  <CheckCircle2 size={10} />
                  {billing.total_count.toLocaleString()} msgs
                </span>
              )}
              {billing.cost_per_msg > 0 && (
                <span className="font-mono">
                  ${billing.cost_per_msg.toFixed(4)}/msg
                </span>
              )}
            </div>

            {/* Source label */}
            <p className="text-[9px] text-muted-foreground/60 italic truncate">
              {billing.source}
            </p>
          </>
        )}
      </CardContent>
    </Card>
  )
}
