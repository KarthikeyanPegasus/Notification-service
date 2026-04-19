'use client'

import { useState, useEffect } from 'react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Trash2, Plus } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import { toast } from 'sonner'

interface Suppression {
  id: string
  type: 'email' | 'sms'
  value: string
  reason: string
  created_at: string
}

export function SuppressionTable() {
  const [suppressions, setSuppressions] = useState<Suppression[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetchSuppressions()
  }, [])

  const fetchSuppressions = async () => {
    try {
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL}/v1/governance/suppressions`)
      if (!res.ok) throw new Error('Failed to fetch')
      const data = await res.json()
      setSuppressions(data || [])
    } catch (err) {
      toast.error('Failed to load suppressions')
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL}/v1/governance/suppressions/${id}`, {
        method: 'DELETE',
      })
      if (!res.ok) throw new Error('Delete failed')
      setSuppressions(suppressions.filter(s => s.id !== id))
      toast.success('Suppression removed')
    } catch (err) {
      toast.error('Failed to remove suppression')
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h2 className="text-xl font-semibold">Active Suppressions</h2>
        <Button size="sm" onClick={() => toast.info('Add suppression via API for now')}>
          <Plus className="h-4 w-4 mr-2" /> Add Selection
        </Button>
      </div>

      <div className="rounded-md border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Type</TableHead>
              <TableHead>Value</TableHead>
              <TableHead>Reason</TableHead>
              <TableHead>Added</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell colSpan={5} className="text-center py-8">Loading...</TableCell>
              </TableRow>
            ) : suppressions.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="text-center py-8 text-muted-foreground">
                  No suppressions found
                </TableCell>
              </TableRow>
            ) : (
              suppressions.map((s) => (
                <TableRow key={s.id}>
                  <TableCell>
                    <Badge variant={s.type === 'email' ? 'default' : 'secondary'}>
                      {s.type.toUpperCase()}
                    </Badge>
                  </TableCell>
                  <TableCell className="font-mono text-xs">{s.value}</TableCell>
                  <TableCell>{s.reason || '--'}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatDistanceToNow(new Date(s.created_at))} ago
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleDelete(s.id)}
                      className="text-destructive hover:text-destructive hover:bg-destructive/10"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}
