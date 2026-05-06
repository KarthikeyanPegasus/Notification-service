'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { KeyRound, Plus, Copy, Trash2, Loader2, Settings, FlaskConical } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  type ApiClientKey,
  createApiKey,
  createMyClient,
  listApiKeys,
  listMyClients,
  revokeApiKey,
} from '@/lib/api'
import { useUserRole } from '@/hooks/use-user-role'

export default function ClientsPage() {
  const role = useUserRole()
  const isSandbox = role === 'dev'

  const [keys, setKeys] = useState<ApiClientKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [createName, setCreateName] = useState('')
  const [creating, setCreating] = useState(false)
  const [secretOpen, setSecretOpen] = useState(false)
  const [newSecret, setNewSecret] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [revokingId, setRevokingId] = useState<string | null>(null)

  const needsBearer = !isSandbox && (typeof process.env.NEXT_PUBLIC_API_BEARER === 'undefined' || !process.env.NEXT_PUBLIC_API_BEARER)

  const activeCount = keys.filter((k) => !k.revoked_at).length

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = isSandbox ? await listMyClients() : await listApiKeys()
      setKeys(data ?? [])
    } catch (e) {
      console.error(e)
      setError(
        e instanceof Error
          ? e.message
          : 'Failed to load API clients. If the API runs in release mode, set NEXT_PUBLIC_API_BEARER to a valid JWT.',
      )
    } finally {
      setLoading(false)
    }
  }, [isSandbox])

  useEffect(() => {
    load()
  }, [load])

  const handleCreate = async () => {
    const name = createName.trim()
    if (name.length < 2) return
    setCreating(true)
    setError(null)
    try {
      const out = isSandbox ? await createMyClient(name) : await createApiKey(name)
      setNewSecret(out.api_key)
      setSecretOpen(true)
      setCreateOpen(false)
      setCreateName('')
      await load()
    } catch (e) {
      console.error(e)
      setError(e instanceof Error ? e.message : 'Failed to create API key')
    } finally {
      setCreating(false)
    }
  }

  const handleRevoke = async (id: string) => {
    if (!confirm('Revoke this API key? It will stop working immediately.')) return
    setRevokingId(id)
    setError(null)
    try {
      await revokeApiKey(id)
      await load()
    } catch (e) {
      console.error(e)
      setError(e instanceof Error ? e.message : 'Failed to revoke key')
    } finally {
      setRevokingId(null)
    }
  }

  const copySecret = async () => {
    if (!newSecret) return
    try {
      await navigator.clipboard.writeText(newSecret)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      /* ignore */
    }
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500">
      <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight flex items-center gap-2">
            <KeyRound className="h-8 w-8 text-primary" aria-hidden />
            Client management
            {isSandbox && <Badge variant="secondary" className="ml-1 text-xs font-normal gap-1"><FlaskConical className="h-3 w-3" />Sandbox</Badge>}
          </h1>
          <p className="text-muted-foreground mt-1 max-w-2xl">
            {isSandbox
              ? 'Your isolated sandbox client. Create 1 API key to test integrations — it will not affect production.'
              : <>Create and revoke API keys for server-to-server access (for example{' '}
                  <code className="text-xs rounded bg-muted px-1 py-0.5">X-API-Key</code> on{' '}
                  <code className="text-xs rounded bg-muted px-1 py-0.5">POST /v1/notifications</code>
                  ). The secret is shown only once when a key is created.</>}
          </p>
        </div>
        <Button
          onClick={() => {
            setCreateOpen(true)
            setCreateName('')
          }}
          disabled={isSandbox && activeCount >= 1}
          title={isSandbox && activeCount >= 1 ? 'Sandbox accounts are limited to 1 active client' : undefined}
          className="w-full md:w-auto shadow-lg shadow-primary/20"
        >
          <Plus className="mr-2 h-4 w-4" />
          New API key
        </Button>
      </div>

      {isSandbox && (
        <p className="text-sm text-blue-700 dark:text-blue-400 rounded-lg border border-blue-500/30 bg-blue-500/10 px-4 py-3">
          <strong>Sandbox mode</strong> — your client is isolated from production. Sandbox accounts are limited to 1 active API key.
        </p>
      )}

      {needsBearer && (
        <p className="text-sm text-amber-600 dark:text-amber-500 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3">
          For production APIs that require JWT, set{' '}
          <code className="text-xs">NEXT_PUBLIC_API_BEARER</code> in the UI environment so admin
          requests can authenticate. Local debug mode often allows unauthenticated admin routes.
        </p>
      )}

      {error && (
        <p className="text-sm text-destructive rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3">
          {error}
        </p>
      )}

      <div className="rounded-xl border bg-card shadow-sm overflow-hidden">
        {loading ? (
          <div className="flex items-center justify-center py-24 text-muted-foreground">
            <Loader2 className="h-8 w-8 animate-spin" aria-label="Loading" />
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Prefix</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Status</TableHead>
                  <TableHead className="w-[140px] text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {keys.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-muted-foreground py-12">
                    No API keys yet. Create one to call the API without browser JWT flows.
                  </TableCell>
                </TableRow>
              ) : (
                keys.map((k) => {
                  const revoked = Boolean(k.revoked_at)
                  return (
                    <TableRow key={k.id} className={revoked ? 'opacity-60' : ''}>
                      <TableCell className="font-medium">{k.name}</TableCell>
                      <TableCell>
                        <code className="text-xs rounded bg-muted px-1.5 py-0.5">{k.prefix}</code>
                      </TableCell>
                      <TableCell className="text-muted-foreground text-sm">
                        {new Date(k.created_at).toLocaleString()}
                      </TableCell>
                      <TableCell>
                        {revoked ? (
                          <Badge variant="secondary">Revoked</Badge>
                        ) : (
                          <Badge>Active</Badge>
                        )}
                      </TableCell>
                      <TableCell className="text-right">
                        <Button
                          variant="ghost"
                          size="icon"
                          asChild
                          aria-label={`Client settings for ${k.name}`}
                        >
                          <Link href={`/clients/${k.id}/settings`}>
                            <Settings className="h-4 w-4" />
                          </Link>
                        </Button>
                        {!revoked && !isSandbox && (
                          <Button
                            variant="ghost"
                            size="icon"
                            className="text-destructive hover:text-destructive"
                            disabled={revokingId === k.id}
                            onClick={() => handleRevoke(k.id)}
                            aria-label={`Revoke ${k.name}`}
                          >
                            {revokingId === k.id ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <Trash2 className="h-4 w-4" />
                            )}
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
        )}
      </div>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>New API key</DialogTitle>
            <DialogDescription>
              Choose a label for this client. You will receive the secret once; store it securely.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2 py-2">
            <label htmlFor="client-name" className="text-sm font-medium">
              Name
            </label>
            <Input
              id="client-name"
              placeholder="e.g. billing-service, mobile-backend"
              value={createName}
              onChange={(e) => setCreateName(e.target.value)}
              className="bg-card/50"
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleCreate}
              disabled={creating || createName.trim().length < 2}
            >
              {creating ? <Loader2 className="h-4 w-4 animate-spin" /> : 'Create'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={secretOpen}
        onOpenChange={(open) => {
          setSecretOpen(open)
          if (!open) setNewSecret(null)
        }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Save your API key</DialogTitle>
            <DialogDescription>
              This secret is not shown again. Copy it now and store it in a secrets manager.
            </DialogDescription>
          </DialogHeader>
          {newSecret && (
            <div className="space-y-3">
              <div className="rounded-md border bg-muted/50 p-3 font-mono text-xs break-all">
                {newSecret}
              </div>
              <Button type="button" variant="secondary" className="w-full" onClick={copySecret}>
                <Copy className="mr-2 h-4 w-4" />
                {copied ? 'Copied' : 'Copy to clipboard'}
              </Button>
            </div>
          )}
          <DialogFooter>
            <Button onClick={() => setSecretOpen(false)}>Done</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
