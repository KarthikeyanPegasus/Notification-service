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
import { truncateId } from '@/lib/utils'

interface OptOut {
  id: string
  user_id: string
  channel: string
  reason: string
  source: string
  created_at: string
}

export function OptOutTable() {
  const [optOuts, setOptOuts] = useState<OptOut[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetchOptOuts()
  }, [])

  const fetchOptOuts = async () => {
    try {
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL}/v1/governance/opt-outs`)
      if (!res.ok) throw new Error('Failed to fetch')
      const data = await res.json()
      setOptOuts(data || [])
    } catch (err) {
      toast.error('Failed to load opt-outs')
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL}/v1/governance/opt-outs/${id}`, {
        method: 'DELETE',
      })
      if (!res.ok) throw new Error('Delete failed')
      setOptOuts(optOuts.filter(o => o.id !== id))
      toast.success('Opt-out removed')
    } catch (err) {
      toast.error('Failed to remove opt-out')
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h2 className="text-xl font-semibold">Active User Opt-outs</h2>
        <Button size="sm" onClick={() => toast.info('Manage opt-outs via API for now')}>
          <Plus className="h-4 w-4 mr-2" /> Add Opt-out
        </Button>
      </div>

      <div className="rounded-md border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>User ID</TableHead>
              <TableHead>Channel</TableHead>
              <TableHead>Reason</TableHead>
              <TableHead>Source</TableHead>
              <TableHead>Added</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center py-8">Loading...</TableCell>
              </TableRow>
            ) : optOuts.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                  No opt-outs found
                </TableCell>
              </TableRow>
            ) : (
              optOuts.map((o) => (
                <TableRow key={o.id}>
                  <TableCell className="font-mono text-xs">{truncateId(o.user_id)}</TableCell>
                  <TableCell>
                    <Badge variant="outline">{o.channel.toUpperCase()}</Badge>
                  </TableCell>
                  <TableCell>{o.reason || 'Not specified'}</TableCell>
                  <TableCell>
                    <Badge variant="secondary" className="capitalize">{o.source}</Badge>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatDistanceToNow(new Date(o.created_at))} ago
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleDelete(o.id)}
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
