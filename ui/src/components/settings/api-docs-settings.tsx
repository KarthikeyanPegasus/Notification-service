'use client'

import React, { useState, useEffect, useMemo } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Loader2, Search, Eye, EyeOff } from 'lucide-react'
import { getApiDocsVisibility, setApiDocsVisibility } from '@/lib/api'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'

export function ApiDocsSettings() {
  const [spec, setSpec] = useState<any>(null)
  const [hiddenEndpoints, setHiddenEndpoints] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [search, setSearch] = useState('')

  useEffect(() => {
    const baseUrl = typeof window === 'undefined' ? process.env.NEXT_PUBLIC_API_URL ?? '' : ''
    
    Promise.all([
      fetch(`${baseUrl}/v1/openapi.json`).then(res => res.json()),
      getApiDocsVisibility()
    ])
      .then(([data, visibilityData]) => {
        setSpec(data)
        setHiddenEndpoints(visibilityData.hidden_endpoints || [])
        setLoading(false)
      })
      .catch(err => {
        console.error(err)
        toast.error('Failed to load API specification')
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

  const toggleEndpointVisibility = async (id: string) => {
    setSaving(true)
    const isHidden = hiddenEndpoints.includes(id)
    const newHidden = isHidden 
      ? hiddenEndpoints.filter(x => x !== id) 
      : [...hiddenEndpoints, id]
      
    try {
      await setApiDocsVisibility(newHidden)
      setHiddenEndpoints(newHidden)
      toast.success(`Endpoint is now ${isHidden ? 'public' : 'hidden'}`)
    } catch (err) {
      toast.error('Failed to update visibility')
    } finally {
      setSaving(false)
    }
  }

  const getMethodColor = (method: string) => {
    switch (method.toUpperCase()) {
      case 'GET': return 'text-blue-500 bg-blue-500/10'
      case 'POST': return 'text-green-500 bg-green-500/10'
      case 'PUT': return 'text-orange-500 bg-orange-500/10'
      case 'PATCH': return 'text-amber-500 bg-amber-500/10'
      case 'DELETE': return 'text-red-500 bg-red-500/10'
      default: return 'text-muted-foreground bg-muted'
    }
  }

  if (loading) {
    return (
      <Card>
        <CardContent className="flex h-64 items-center justify-center">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
        </CardContent>
      </Card>
    )
  }

  return (
    <Card className="shadow-sm">
      <CardHeader className="pb-6 pt-8 px-8">
        <CardTitle className="text-2xl flex items-center gap-2">API Documentation Settings</CardTitle>
        <CardDescription className="text-base">Configure which endpoints are visible in the public API reference.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-8 px-8 pb-8">
        <div className="relative max-w-md">
          <Search className="absolute left-3 top-3 h-5 w-5 text-muted-foreground" />
          <Input
            placeholder="Search endpoints..."
            className="pl-10 h-11 text-base"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        
        <div className="rounded-xl border shadow-sm overflow-hidden bg-background">
          <div className="max-h-[600px] overflow-y-auto custom-scrollbar">
            {filteredOperations.length === 0 ? (
              <div className="p-16 text-center text-base text-muted-foreground">No endpoints found matching your search.</div>
            ) : (
              <div className="divide-y divide-border/50">
                {filteredOperations.map((op) => {
                  const isHidden = hiddenEndpoints.includes(op.id)
                  return (
                    <div key={op.id} className={cn(
                      "flex items-center justify-between py-5 px-6 hover:bg-muted/30 transition-colors",
                      isHidden && "bg-muted/10"
                    )}>
                      <div className="flex items-center gap-6 overflow-hidden">
                        <Badge variant="outline" className={cn("w-20 h-7 justify-center text-xs font-bold border-transparent tracking-wider", getMethodColor(op.method))}>
                          {op.method}
                        </Badge>
                        <div className="flex flex-col gap-1 truncate">
                          <span className="text-base font-semibold tracking-tight truncate">{op.summary || op.path}</span>
                          <span className={cn("text-sm font-mono truncate px-2 py-0.5 rounded bg-muted/50 w-fit", isHidden ? "text-muted-foreground/60" : "text-muted-foreground")}>
                            {op.path}
                          </span>
                        </div>
                      </div>
                      
                      <div className="flex items-center gap-5 shrink-0 ml-8">
                        {isHidden ? (
                          <Badge variant="secondary" className="px-3 py-1 text-xs font-medium uppercase tracking-wide">Hidden</Badge>
                        ) : (
                          <Badge variant="default" className="bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/20 border-transparent px-3 py-1 text-xs font-medium uppercase tracking-wide">Public</Badge>
                        )}
                        <Button
                          variant={isHidden ? "default" : "outline"}
                          size="default"
                          disabled={saving}
                          onClick={() => toggleEndpointVisibility(op.id)}
                          className="w-28 font-medium shadow-sm h-10"
                        >
                          {isHidden ? (
                            <><Eye className="h-4 w-4 mr-2" /> Show</>
                          ) : (
                            <><EyeOff className="h-4 w-4 mr-2" /> Hide</>
                          )}
                        </Button>
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
