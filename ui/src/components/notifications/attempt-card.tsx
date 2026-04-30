import type { Attempt } from '@/types'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { formatDate, formatLatency } from '@/lib/utils'
import { Clock, CheckCircle2, XCircle, Hash, Zap } from 'lucide-react'
import { cn } from '@/lib/utils'

interface AttemptCardProps {
  attempt: Attempt
  index: number
}

export function AttemptCard({ attempt, index }: AttemptCardProps) {
  const isSuccess = ['DELIVERED', 'SENT', 'SUCCESS'].includes(attempt.status.toUpperCase())
  const isFailed = ['FAILED', 'ERROR'].includes(attempt.status.toUpperCase())
  const providerMsgID = attempt.provider_message_id ?? attempt.provider_msg_id ?? undefined
  const errText = attempt.error ?? attempt.error_message ?? undefined
  const errCode = attempt.error_code ?? undefined

  return (
    <Card className={cn(
      'border-l-4',
      isSuccess ? 'border-l-green-500' : isFailed ? 'border-l-red-500' : 'border-l-yellow-500',
    )}>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-sm font-semibold">
            Attempt #{index + 1} — {attempt.provider}
          </CardTitle>
          <Badge
            variant={isSuccess ? 'success' : isFailed ? 'destructive' : 'warning'}
            className="capitalize"
          >
            {attempt.status}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        <div className="grid grid-cols-2 gap-3">
          <div className="flex items-center gap-2 text-muted-foreground">
            <Clock className="h-3.5 w-3.5" />
            <span>{formatDate(attempt.created_at)}</span>
          </div>
          <div className="flex items-center gap-2 text-muted-foreground">
            <Zap className="h-3.5 w-3.5" />
            <span>Latency: <strong className="text-foreground">{formatLatency(attempt.latency_ms)}</strong></span>
          </div>
        </div>

        {providerMsgID && (
          <div className="flex items-center gap-2 text-muted-foreground">
            <Hash className="h-3.5 w-3.5" />
            <span>
              Provider ID:{' '}
              <code className="text-xs bg-muted rounded px-1 text-foreground">
                {providerMsgID}
              </code>
            </span>
          </div>
        )}

        {errText && (
          <div className="flex items-start gap-2 text-red-600 bg-red-50 dark:bg-red-950/30 rounded-md p-3">
            <XCircle className="h-4 w-4 shrink-0 mt-0.5" />
            <div className="text-xs font-mono">
              {errCode && <div className="opacity-80">code: {errCode}</div>}
              <div>{errText}</div>
            </div>
          </div>
        )}

        {isSuccess && !errText && (
          <div className="flex items-center gap-2 text-green-600">
            <CheckCircle2 className="h-4 w-4" />
            <span className="text-xs">Delivered successfully</span>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
