'use client'

import { useState, useEffect } from 'react'
import { FileText, Plus, Search, Filter } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { TemplateList } from '@/components/templates/template-list'
import { TemplateDialog } from '@/components/templates/template-dialog'
import { getTemplates } from '@/lib/api'
import type { Template } from '@/types'

export default function TemplatesPage() {
  const [templates, setTemplates] = useState<Template[]>([])
  const [loading, setLoading] = useState(true)
  const [searchQuery, setSearchQuery] = useState('')
  const [isDialogOpen, setIsDialogOpen] = useState(false)
  const [editingTemplate, setEditingTemplate] = useState<Template | null>(null)

  const fetchTemplates = async () => {
    setLoading(true)
    try {
      const data = await getTemplates()
      setTemplates(data || [])
    } catch (error) {
      console.error('Failed to fetch templates:', error)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchTemplates()
  }, [])

  const filteredTemplates = (templates || []).filter((t: any) => 
    t.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    t.channel.toLowerCase().includes(searchQuery.toLowerCase())
  )

  const handleEdit = (template: any) => {
    setEditingTemplate(template)
    setIsDialogOpen(true)
  }

  const handleCreate = () => {
    setEditingTemplate(null)
    setIsDialogOpen(true)
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500">
      {/* Header */}
      <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Templates</h1>
          <p className="text-muted-foreground">
            Manage your multi-channel notification templates.
          </p>
        </div>
        <Button onClick={handleCreate} className="w-full md:w-auto shadow-lg shadow-primary/20">
          <Plus className="mr-2 h-4 w-4" /> Add Template
        </Button>
      </div>

      {/* Filters & Search */}
      <div className="flex flex-col gap-4 md:flex-row md:items-center">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search by name or channel..."
            className="pl-9 bg-card border-none ring-1 ring-border/50 focus-visible:ring-primary shadow-sm"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
        <Button variant="outline" size="icon" className="shrink-0 bg-card border-none ring-1 ring-border/50">
          <Filter className="h-4 w-4" />
        </Button>
      </div>

      {/* List */}
      <div className="rounded-xl border bg-card shadow-sm overflow-hidden">
        <TemplateList 
          templates={filteredTemplates} 
          loading={loading} 
          onEdit={handleEdit}
          onRefresh={fetchTemplates}
        />
      </div>

      <TemplateDialog
        isOpen={isDialogOpen}
        onClose={() => setIsDialogOpen(false)}
        template={editingTemplate}
        onSuccess={fetchTemplates}
      />
    </div>
  )
}
