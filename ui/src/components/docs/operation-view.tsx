'use client'

import React from 'react'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Table, TableHeader, TableRow, TableHead, TableBody, TableCell } from '@/components/ui/table'
import { SchemaView } from './schema-view'
import { cn } from '@/lib/utils'

interface OperationViewProps {
  operation: any
  components: any
}

export function OperationView({ operation, components }: OperationViewProps) {
  const responses = Object.entries(operation.responses || {})

  return (
    <div className="p-8 max-w-4xl mx-auto space-y-10 animate-in fade-in slide-in-from-bottom-2 duration-300">
      {/* Header Section */}
      <section className="space-y-4">
        <div className="flex items-center gap-4">
          <Badge className={cn("text-xs font-bold px-2 py-0.5", getMethodBg(operation.method))}>
            {operation.method}
          </Badge>
          <code className="text-sm font-mono bg-muted px-2 py-0.5 rounded text-foreground/80">
            {operation.path}
          </code>
        </div>
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">{operation.summary}</h1>
          <p className="text-muted-foreground leading-relaxed">
            {operation.description || "No description provided."}
          </p>
        </div>
      </section>

      <Separator />

      {/* Parameters */}
      {operation.parameters && operation.parameters.length > 0 && (
        <section className="space-y-4">
          <h2 className="text-xl font-semibold flex items-center gap-2">
            Parameters
          </h2>
          <div className="rounded-md border overflow-hidden">
            <Table>
              <TableHeader className="bg-muted/50">
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>In</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Required</TableHead>
                  <TableHead className="w-1/2">Description</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {operation.parameters.map((p: any, idx: number) => (
                  <TableRow key={idx}>
                    <TableCell className="font-mono text-xs font-medium">{p.name}</TableCell>
                    <TableCell className="text-xs text-muted-foreground italic">{p.in}</TableCell>
                    <TableCell className="text-xs">{p.schema?.type || 'string'}</TableCell>
                    <TableCell>
                      {p.required ? (
                        <Badge variant="outline" className="text-[10px] text-destructive border-destructive/20 bg-destructive/5 capitalize">Required</Badge>
                      ) : (
                        <span className="text-[10px] text-muted-foreground uppercase tracking-widest font-semibold">Optional</span>
                      )}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{p.description || '-'}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </section>
      )}

      {/* Request Body */}
      {operation.requestBody && (
        <section className="space-y-4">
          <h2 className="text-xl font-semibold">Request Body</h2>
          <div className="bg-muted/30 rounded-lg p-5 border border-dashed">
            {Object.entries(operation.requestBody.content || {}).map(([contentType, content]: [string, any], idx: number) => (
              <div key={idx} className="space-y-4">
                <div className="flex items-center gap-2">
                   <Badge variant="secondary" className="text-[10px] font-mono">{contentType}</Badge>
                </div>
                <SchemaView schema={content.schema} components={components} />
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Responses */}
      <section className="space-y-4">
        <h2 className="text-xl font-semibold">Responses</h2>
        <div className="space-y-6">
          {responses.map(([code, res]: [string, any]) => (
            <div key={code} className="border rounded-lg overflow-hidden">
              <div className={cn("px-4 py-2 flex items-center justify-between border-b bg-muted/50")}>
                <div className="flex items-center gap-3">
                  <Badge className={cn("text-[10px] font-bold", getStatusColor(code))}>
                    {code}
                  </Badge>
                  <span className="text-sm font-medium">{res.description}</span>
                </div>
              </div>
              {res.content && (
                <div className="p-4 bg-muted/10">
                  {Object.entries(res.content).map(([contentType, content]: [string, any], idx: number) => (
                    <div key={idx} className="space-y-3">
                       <span className="text-[10px] font-mono text-muted-foreground uppercase tracking-wider">{contentType}</span>
                       <SchemaView schema={content.schema} components={components} />
                    </div>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}

function getMethodBg(method: string) {
  switch (method.toUpperCase()) {
    case 'GET': return 'bg-blue-500 text-white hover:bg-blue-600'
    case 'POST': return 'bg-green-500 text-white hover:bg-green-600'
    case 'PUT': return 'bg-orange-500 text-white hover:bg-orange-600'
    case 'PATCH': return 'bg-amber-500 text-white hover:bg-amber-600'
    case 'DELETE': return 'bg-red-500 text-white hover:bg-red-600'
    default: return 'bg-muted text-muted-foreground'
  }
}

function getStatusColor(code: string) {
  const c = parseInt(code)
  if (c >= 200 && c < 300) return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
  if (c >= 400 && c < 500) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  return 'bg-muted text-muted-foreground'
}
