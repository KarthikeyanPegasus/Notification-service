import React from 'react'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ShieldAlert, Globe } from 'lucide-react'
import { BreakdownRow, addSuppression } from '@/lib/api'

interface SmsCountryWidgetProps {
  data: BreakdownRow[]
  isLoading?: boolean
}

export function SmsCountryWidget({ data, isLoading }: SmsCountryWidgetProps) {
  const handleBlock = async (prefix: string) => {
    // Extract prefix from label like "+91 (India)" -> "+91"
    const val = prefix.split(' ')[0]
    
    if (!window.confirm(`Are you sure you want to block all SMS to prefix ${val}?`)) {
      return
    }

    try {
      await addSuppression('sms', val, 'Manual block from dashboard')
      alert(`Country prefix ${val} has been blocked.`)
    } catch (error: any) {
      alert(`Error: ${error.message || 'Failed to block country'}`)
    }
  }

  return (
    <Card className="overflow-hidden border-none shadow-xl bg-gradient-to-br from-background to-muted/20">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <div className="flex items-center gap-2">
          <Globe className="text-blue-500" size={18} />
          <CardTitle className="text-sm font-medium">SMS by Country (24h)</CardTitle>
        </div>
        <Badge variant="outline" className="text-[10px]">Top 10</Badge>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2 py-4">
            <div className="h-4 bg-muted animate-pulse rounded w-full" />
            <div className="h-4 bg-muted animate-pulse rounded w-3/4" />
            <div className="h-4 bg-muted animate-pulse rounded w-1/2" />
          </div>
        ) : data.length === 0 ? (
          <div className="py-8 text-center text-xs text-muted-foreground italic">
            No SMS data available for the selected period.
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent border-muted/50">
                <TableHead className="text-[10px] uppercase font-bold py-2">Country/Prefix</TableHead>
                <TableHead className="text-[10px] uppercase font-bold py-2 text-right">Count</TableHead>
                <TableHead className="text-[10px] uppercase font-bold py-2 text-right">Action</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.map((item) => (
                <TableRow key={item.key} className="group border-muted/30">
                  <TableCell className="py-2 text-sm font-medium">{item.key}</TableCell>
                  <TableCell className="py-2 text-sm text-right font-mono">{item.count}</TableCell>
                  <TableCell className="py-2 text-right">
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                      onClick={() => handleBlock(item.key)}
                      title="Block Country"
                    >
                      <ShieldAlert size={14} />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}
