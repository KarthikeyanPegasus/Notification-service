'use client'

import React, { useState, useEffect, useMemo } from 'react'
import { Search, ChevronRight, Hash, Code, List } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { OperationView } from './operation-view'

export function APIDocs() {
  const [spec, setSpec] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  useEffect(() => {
    fetch('http://localhost:8080/v1/openapi.json')
      .then(res => res.json())
      .then(data => {
        setSpec(data)
        setLoading(false)
        // Auto-select first endpoint
        const firstPath = Object.keys(data.paths || {})[0]
        if (firstPath) {
          const firstMethod = Object.keys(data.paths[firstPath])[0]
          setSelectedId(`${firstMethod}-${firstPath}`)
        }
      })
      .catch(err => {
        console.error(err)
        setError('Failed to load API specification')
        setLoading(false)
      })
  }, [])

  const operations = useMemo(() => {
    if (!spec) return []
    const ops: any[] = []
    Object.entries(spec.paths || {}).forEach(([path, methods]: [string, any]) => {
      Object.entries(methods).forEach(([method, detail]: [string, any]) => {
        if (['get', 'post', 'put', 'patch', 'delete'].includes(method.toLowerCase())) {
          ops.push({
            id: `${method}-${path}`,
            path,
            method: method.toUpperCase(),
            ...detail,
          })
        }
      })
    })
    return ops
  }, [spec])

  const filteredOperations = useMemo(() => {
    return operations.filter(op => 
      op.path.toLowerCase().includes(search.toLowerCase()) || 
      op.summary?.toLowerCase().includes(search.toLowerCase())
    )
  }, [operations, search])

  const selectedOp = useMemo(() => 
    operations.find(op => op.id === selectedId),
  [operations, selectedId])

  if (loading) return <div className="p-8 text-center">Loading documentation...</div>
  if (error) return <div className="p-8 text-center text-destructive">{error}</div>

  return (
    <div className="flex h-[calc(100vh-8rem)] rounded-xl border bg-card shadow-sm overflow-hidden">
      {/* Sidebar Navigation */}
      <div className="w-80 border-r bg-muted/30 flex flex-col">
        <div className="p-4 border-b bg-background/50 backdrop-blur-sm sticky top-0 z-10">
          <div className="relative">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search endpoints..."
              className="pl-9 h-9"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
        </div>
        
        <div className="flex-1 overflow-auto p-2">
          {filteredOperations.length === 0 ? (
            <div className="p-4 text-center text-xs text-muted-foreground">No matches found</div>
          ) : (
            <div className="space-y-1">
              {filteredOperations.map((op) => (
                <button
                  key={op.id}
                  onClick={() => setSelectedId(op.id)}
                  className={cn(
                    "w-full flex items-center gap-3 px-3 py-2.5 rounded-md text-left text-sm transition-all group",
                    selectedId === op.id 
                      ? "bg-primary text-primary-foreground shadow-sm" 
                      : "hover:bg-accent text-muted-foreground hover:text-foreground"
                  )}
                >
                  <span className={cn(
                    "text-[10px] font-bold uppercase w-12 text-center",
                    selectedId === op.id ? "text-primary-foreground/90" : getMethodColor(op.method)
                  )}>
                    {op.method}
                  </span>
                  <div className="flex-1 truncate">
                    <div className="font-medium truncate">{op.summary || op.path}</div>
                    <div className={cn(
                      "text-[10px] truncate opacity-70",
                      selectedId === op.id ? "text-primary-foreground/70" : "text-muted-foreground"
                    )}>
                      {op.path}
                    </div>
                  </div>
                  <ChevronRight className={cn(
                    "h-3 w-3 shrink-0 transition-transform",
                    selectedId === op.id ? "rotate-90" : "opacity-0 group-hover:opacity-100"
                  )} />
                </button>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Main Content */}
      <div className="flex-1 overflow-auto bg-background/30 custom-scrollbar">
        {selectedOp ? (
          <OperationView operation={selectedOp} components={spec.components} />
        ) : (
          <div className="h-full flex items-center justify-center text-muted-foreground italic">
            Select an endpoint to view details
          </div>
        )}
      </div>
    </div>
  )
}

function getMethodColor(method: string) {
  switch (method.toUpperCase()) {
    case 'GET': return 'text-blue-500'
    case 'POST': return 'text-green-500'
    case 'PUT': return 'text-orange-500'
    case 'PATCH': return 'text-amber-500'
    case 'DELETE': return 'text-red-500'
    default: return 'text-muted-foreground'
  }
}
