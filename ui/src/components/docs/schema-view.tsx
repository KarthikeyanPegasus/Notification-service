'use client'

import React from 'react'
import { Hash, List, Type, Box } from 'lucide-react'
import { cn } from '@/lib/utils'

interface SchemaViewProps {
  schema: any
  components: any
  depth?: number
}

export function SchemaView({ schema, components, depth = 0 }: SchemaViewProps) {
  if (!schema) return null

  // Resolve $ref
  let resolvedSchema = schema
  if (schema.$ref) {
    const parts = schema.$ref.split('/')
    const key = parts[parts.length - 1]
    resolvedSchema = components.schemas?.[key]
  }

  if (!resolvedSchema) {
    return <span className="text-xs text-muted-foreground italic">Reference not found</span>
  }

  // Handle Object
  if (resolvedSchema.type === 'object') {
    const properties = Object.entries(resolvedSchema.properties || {})
    return (
      <div className={cn("space-y-3", depth > 0 && "pl-4 border-l ml-1 mt-2")}>
        <div className="flex items-center gap-2 text-xs font-semibold text-foreground/70 tracking-wider uppercase">
          <Box className="h-3 w-3 text-purple-500" />
          object
        </div>
        <div className="space-y-4">
          {properties.map(([name, prop]: [string, any], idx: number) => (
            <div key={idx} className="space-y-1">
              <div className="flex items-baseline gap-2">
                <span className="text-sm font-mono font-medium text-primary/80">{name}</span>
                <span className="text-[10px] bg-muted/50 px-1.5 py-0.5 rounded text-muted-foreground font-mono">
                  {prop.type || (prop.$ref ? 'ref' : 'any')}
                </span>
                {resolvedSchema.required?.includes(name) && (
                  <span className="text-[10px] text-destructive/80 font-bold">*</span>
                )}
              </div>
              {prop.description && (
                <p className="text-xs text-muted-foreground leading-relaxed max-w-2xl">{prop.description}</p>
              )}
              {(prop.type === 'object' || prop.$ref || prop.type === 'array') && (
                <SchemaView schema={prop} components={components} depth={depth + 1} />
              )}
            </div>
          ))}
        </div>
      </div>
    )
  }

  // Handle Array
  if (resolvedSchema.type === 'array') {
    return (
      <div className={cn("space-y-2 mt-2", depth > 0 && "pl-4 border-l ml-1")}>
        <div className="flex items-center gap-2 text-xs font-semibold text-foreground/70 tracking-wider uppercase">
          <List className="h-3 w-3 text-blue-500" />
          array of:
        </div>
        <SchemaView schema={resolvedSchema.items} components={components} depth={depth + 1} />
      </div>
    )
  }

  // Handle Primitives or Enums
  return (
    <div className="space-y-1 text-xs">
       {resolvedSchema.enum && (
         <div className="flex flex-wrap gap-1 mt-1">
            {resolvedSchema.enum.map((e: string, i: number) => (
              <span key={i} className="bg-muted px-1.5 py-0.5 rounded text-[10px] font-mono border italic">{e}</span>
            ))}
         </div>
       )}
    </div>
  )
}
