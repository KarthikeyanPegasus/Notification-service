'use client'

import { 
  Mail, 
  MessageSquare, 
  Smartphone, 
  Globe, 
  MoreVertical, 
  Edit2, 
  Trash2,
  Copy
} from 'lucide-react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { deleteTemplate } from '@/lib/api'

const ChannelIcon = ({ channel }: { channel: string }) => {
  switch (channel) {
    case 'email': return <Mail className="h-4 w-4 text-blue-500" />
    case 'sms': return <MessageSquare className="h-4 w-4 text-green-500" />
    case 'push': return <Smartphone className="h-4 w-4 text-purple-500" />
    case 'webhook': return <Globe className="h-4 w-4 text-orange-500" />
    default: return <Mail className="h-4 w-4" />
  }
}

export function TemplateList({ templates, loading, onEdit, onRefresh }: any) {
  if (loading) {
    return (
      <div className="p-4 space-y-4">
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-12 w-full rounded-lg" />
        ))}
      </div>
    )
  }

  if (templates.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center p-12 text-center">
        <div className="rounded-full bg-muted p-4 mb-4">
          <Mail className="h-8 w-8 text-muted-foreground" />
        </div>
        <h3 className="text-lg font-semibold">No templates found</h3>
        <p className="text-sm text-muted-foreground max-w-sm">
          Get started by creating your first notification template.
        </p>
      </div>
    )
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this template?')) return
    try {
      await deleteTemplate(id)
      onRefresh()
    } catch (error) {
      console.error('Failed to delete template:', error)
    }
  }

  return (
    <Table>
      <TableHeader className="bg-muted/50">
        <TableRow>
          <TableHead className="w-[200px]">Name</TableHead>
          <TableHead className="w-[150px]">Template ID</TableHead>
          <TableHead>Channel</TableHead>
          <TableHead>Version</TableHead>
          <TableHead>Status</TableHead>
          <TableHead className="text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {templates.map((template: any) => (
          <TableRow key={template.id} className="group hover:bg-muted/30 transition-colors">
            <TableCell>
              <span className="font-semibold text-foreground">{template.name}</span>
            </TableCell>
            <TableCell>
              <div className="flex items-center gap-2 group/id">
                <code className="text-xs text-muted-foreground font-mono bg-muted/50 px-1.5 py-0.5 rounded">
                  {template.id}
                </code>
                <Button 
                  variant="ghost" 
                  size="icon" 
                  className="h-6 w-6 opacity-0 group-hover/id:opacity-100 transition-opacity"
                  onClick={() => {
                    navigator.clipboard.writeText(template.id)
                  }}
                >
                  <Copy className="h-3 w-3" />
                </Button>
              </div>
            </TableCell>
            <TableCell>
              <div className="flex items-center gap-2">
                <ChannelIcon channel={template.channel} />
                <span className="capitalize text-sm">{template.channel}</span>
              </div>
            </TableCell>
            <TableCell>
              <Badge variant="outline" className="font-mono">v{template.version}</Badge>
            </TableCell>
            <TableCell>
              {template.is_active ? (
                <Badge variant="secondary" className="bg-green-500/10 text-green-600 border-none">Active</Badge>
              ) : (
                <Badge variant="outline">Inactive</Badge>
              )}
            </TableCell>
            <TableCell className="text-right">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon" className="h-8 w-8 opacity-0 group-hover:opacity-100 transition-opacity">
                    <MoreVertical className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-40 border-none shadow-xl ring-1 ring-black/5">
                  <DropdownMenuItem onClick={() => onEdit(template)} className="gap-2">
                    <Edit2 className="h-4 w-4" /> Edit
                  </DropdownMenuItem>
                  <DropdownMenuItem className="gap-2">
                    <Copy className="h-4 w-4" /> Duplicate
                  </DropdownMenuItem>
                  <DropdownMenuItem 
                    onClick={() => handleDelete(template.id)} 
                    className="gap-2 text-destructive focus:text-destructive"
                  >
                    <Trash2 className="h-4 w-4" /> Delete
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
