'use client'

import { useState, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import { Calendar, ChevronDown, X } from 'lucide-react'
import { format, subMinutes, subHours, subDays, subMonths, subYears } from 'date-fns'

export interface DateRange {
  from: string   // ISO string
  to: string     // ISO string
  label: string  // human-readable label shown in the button
}

interface Preset {
  label: string
  shortLabel: string
  getRange: () => DateRange
}

const PRESETS: Preset[] = [
  { label: '5 minutes',  shortLabel: '5m',  getRange: () => makeRange(subMinutes(new Date(), 5),   '5m') },
  { label: '10 minutes', shortLabel: '10m', getRange: () => makeRange(subMinutes(new Date(), 10),  '10m') },
  { label: '15 minutes', shortLabel: '15m', getRange: () => makeRange(subMinutes(new Date(), 15),  '15m') },
  { label: '30 minutes', shortLabel: '30m', getRange: () => makeRange(subMinutes(new Date(), 30),  '30m') },
  { label: '1 hour',     shortLabel: '1h',  getRange: () => makeRange(subHours(new Date(), 1),     '1h') },
  { label: '2 hours',    shortLabel: '2h',  getRange: () => makeRange(subHours(new Date(), 2),     '2h') },
  { label: '6 hours',    shortLabel: '6h',  getRange: () => makeRange(subHours(new Date(), 6),     '6h') },
  { label: '12 hours',   shortLabel: '12h', getRange: () => makeRange(subHours(new Date(), 12),    '12h') },
  { label: '1 day',      shortLabel: '1d',  getRange: () => makeRange(subDays(new Date(), 1),      '1d') },
  { label: '2 days',     shortLabel: '2d',  getRange: () => makeRange(subDays(new Date(), 2),      '2d') },
  { label: '7 days',     shortLabel: '7d',  getRange: () => makeRange(subDays(new Date(), 7),      '7d') },
  { label: '14 days',    shortLabel: '14d', getRange: () => makeRange(subDays(new Date(), 14),     '14d') },
  { label: '1 month',    shortLabel: '1mo', getRange: () => makeRange(subMonths(new Date(), 1),    '1mo') },
  { label: '2 months',   shortLabel: '2mo', getRange: () => makeRange(subMonths(new Date(), 2),    '2mo') },
  { label: '3 months',   shortLabel: '3mo', getRange: () => makeRange(subMonths(new Date(), 3),    '3mo') },
  { label: '6 months',   shortLabel: '6mo', getRange: () => makeRange(subMonths(new Date(), 6),    '6mo') },
  { label: '1 year',     shortLabel: '1y',  getRange: () => makeRange(subYears(new Date(), 1),     '1y') },
]

function makeRange(from: Date, label: string): DateRange {
  return { from: from.toISOString(), to: new Date().toISOString(), label }
}

// Default: last 24 hours
export function defaultRange(): DateRange {
  return PRESETS[8].getRange() // '1d'
}

interface DateRangeSelectorProps {
  value: DateRange
  onChange: (range: DateRange) => void
  className?: string
}

export function DateRangeSelector({ value, onChange, className }: DateRangeSelectorProps) {
  const [open, setOpen] = useState(false)
  const [tab, setTab] = useState<'presets' | 'custom'>('presets')
  const [customFrom, setCustomFrom] = useState(() => toLocalInput(value.from))
  const [customTo, setCustomTo]     = useState(() => toLocalInput(value.to))

  const applyPreset = useCallback((p: Preset) => {
    const r = p.getRange()
    onChange(r)
    setCustomFrom(toLocalInput(r.from))
    setCustomTo(toLocalInput(r.to))
    setOpen(false)
  }, [onChange])

  const applyCustom = useCallback(() => {
    const from = new Date(customFrom)
    const to   = new Date(customTo)
    if (isNaN(from.getTime()) || isNaN(to.getTime()) || from >= to) return
    const label = `${format(from, 'MMM d, HH:mm')} – ${format(to, 'MMM d, HH:mm')}`
    onChange({ from: from.toISOString(), to: to.toISOString(), label })
    setOpen(false)
  }, [customFrom, customTo, onChange])

  return (
    <div className={cn('relative', className)}>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setOpen(o => !o)}
        className="flex items-center gap-2 font-mono text-xs h-9 px-3"
      >
        <Calendar size={13} className="text-muted-foreground shrink-0" />
        <span className="truncate max-w-[180px]">{value.label}</span>
        <ChevronDown size={12} className={cn('text-muted-foreground transition-transform shrink-0', open && 'rotate-180')} />
      </Button>

      {open && (
        <>
          {/* Backdrop */}
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />

          {/* Panel */}
          <div className="absolute right-0 top-10 z-50 w-72 rounded-lg border bg-popover shadow-lg overflow-hidden">
            {/* Tab bar */}
            <div className="flex border-b text-xs font-semibold">
              <button
                onClick={() => setTab('presets')}
                className={cn(
                  'flex-1 py-2.5 transition-colors',
                  tab === 'presets' ? 'bg-muted text-foreground' : 'text-muted-foreground hover:text-foreground'
                )}
              >
                Presets
              </button>
              <button
                onClick={() => setTab('custom')}
                className={cn(
                  'flex-1 py-2.5 transition-colors',
                  tab === 'custom' ? 'bg-muted text-foreground' : 'text-muted-foreground hover:text-foreground'
                )}
              >
                Custom range
              </button>
            </div>

            {tab === 'presets' ? (
              <div className="p-2 grid grid-cols-3 gap-1">
                {PRESETS.map((p) => {
                  const active = value.label === p.shortLabel
                  return (
                    <button
                      key={p.shortLabel}
                      onClick={() => applyPreset(p)}
                      className={cn(
                        'rounded-md px-2 py-1.5 text-xs font-mono text-left transition-colors',
                        active
                          ? 'bg-primary text-primary-foreground'
                          : 'hover:bg-muted text-muted-foreground hover:text-foreground'
                      )}
                    >
                      <span className="font-bold">{p.shortLabel}</span>
                      <span className="block text-[10px] opacity-70 font-sans">{p.label}</span>
                    </button>
                  )
                })}
              </div>
            ) : (
              <div className="p-3 space-y-3">
                <div className="space-y-1.5">
                  <Label className="text-xs">From</Label>
                  <Input
                    type="datetime-local"
                    value={customFrom}
                    onChange={e => setCustomFrom(e.target.value)}
                    className="text-xs h-8"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-xs">To</Label>
                  <Input
                    type="datetime-local"
                    value={customTo}
                    onChange={e => setCustomTo(e.target.value)}
                    className="text-xs h-8"
                  />
                </div>
                <Button size="sm" className="w-full text-xs h-8" onClick={applyCustom}>
                  Apply
                </Button>
              </div>
            )}

            {/* Active range footer */}
            <div className="border-t px-3 py-2 flex items-center justify-between gap-2 bg-muted/30">
              <span className="text-[10px] text-muted-foreground truncate font-mono">{value.label}</span>
              <button onClick={() => { applyPreset(PRESETS[8]); }} title="Reset to 1d">
                <X size={12} className="text-muted-foreground hover:text-foreground" />
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}

function toLocalInput(iso: string): string {
  try {
    const d = new Date(iso)
    // datetime-local needs "YYYY-MM-DDTHH:MM"
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
  } catch {
    return ''
  }
}
