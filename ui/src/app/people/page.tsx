'use client'

import { useEffect, useMemo, useState } from 'react'
import { PageHeader } from '@/components/shared/page-header'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Loader2, Plus, Save, Trash2, Users } from 'lucide-react'
import { getAuthToken, listApiKeys, type ApiClientKey } from '@/lib/api'

type Role = 'admin' | 'manager' | 'dev' | 'support'
type User = { id: string; email: string; name: string; role: Role; created_at: string }

async function apiJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json', Accept: 'application/json' }
  const tok = getAuthToken()
  if (tok) headers['Authorization'] = `Bearer ${tok}`
  const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'}${path}`, {
    headers,
    ...init,
  })
  if (!res.ok) throw new Error(await res.text())
  // Some admin endpoints return 204 No Content.
  if (res.status === 204) return undefined as T
  const text = await res.text()
  if (!text) return undefined as T
  return JSON.parse(text) as T
}

export default function PeoplePage() {
  const [users, setUsers] = useState<User[]>([])
  const [keys, setKeys] = useState<ApiClientKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [createEmail, setCreateEmail] = useState('')
  const [createName, setCreateName] = useState('')
  const [createPassword, setCreatePassword] = useState('')
  const [createRole, setCreateRole] = useState<Role>('dev')
  const [creating, setCreating] = useState(false)

  const [selectedUser, setSelectedUser] = useState<User | null>(null)
  const [assignments, setAssignments] = useState<Record<string, boolean>>({})
  const selectedAssignments = useMemo(() => Object.entries(assignments).filter(([, v]) => v).map(([k]) => k), [assignments])
  const [savingAssign, setSavingAssign] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const u = await apiJSON<User[]>('/v1/admin/users')
      setUsers(u ?? [])
      const k = await listApiKeys()
      setKeys(k ?? [])
    } catch (e) {
      console.error(e)
      setError('Failed to load people. You may need an admin JWT.')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const createUser = async () => {
    setCreating(true)
    setError(null)
    try {
      await apiJSON('/v1/admin/users', {
        method: 'POST',
        body: JSON.stringify({ email: createEmail, name: createName, password: createPassword, role: createRole }),
      })
      setCreateEmail('')
      setCreateName('')
      setCreatePassword('')
      setCreateRole('dev')
      await load()
    } catch (e) {
      console.error(e)
      setError('Failed to create user.')
    } finally {
      setCreating(false)
    }
  }

  const loadAssignments = async (u: User) => {
    setSelectedUser(u)
    setAssignments({})
    try {
      const out = await apiJSON<{ api_key_ids: string[] }>(`/v1/admin/users/${u.id}/clients`)
      const set = new Set(out.api_key_ids ?? [])
      const next: Record<string, boolean> = {}
      keys.forEach((k) => {
        next[k.id] = set.has(k.id)
      })
      setAssignments(next)
    } catch (e) {
      console.error(e)
      setError('Failed to load assignments.')
    }
  }

  const saveAssignments = async () => {
    if (!selectedUser) return
    setSavingAssign(true)
    setError(null)
    try {
      await apiJSON(`/v1/admin/users/${selectedUser.id}/clients`, {
        method: 'PUT',
        body: JSON.stringify({ api_key_ids: selectedAssignments }),
      })
    } catch (e) {
      console.error(e)
      setError('Failed to save assignments.')
    } finally {
      setSavingAssign(false)
    }
  }

  const deleteUser = async (u: User) => {
    const ok = window.confirm(`Delete user "${u.name}" (${u.email})? This cannot be undone.`)
    if (!ok) return
    setError(null)
    try {
      await apiJSON<void>(`/v1/admin/users/${u.id}`, { method: 'DELETE' })
      if (selectedUser?.id === u.id) {
        setSelectedUser(null)
        setAssignments({})
      }
      await load()
    } catch (e) {
      console.error(e)
      setError('Failed to delete user.')
    }
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500">
      <PageHeader title="People" description="Manage users, roles, and client (API key) assignments." />

      {error && (
        <p className="text-sm text-destructive rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3">
          {error}
        </p>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Users className="h-5 w-5 text-primary" aria-hidden />
              Users
            </CardTitle>
            <CardDescription>Create and view users.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {loading ? (
              <div className="flex items-center justify-center py-10 text-muted-foreground">
                <Loader2 className="h-6 w-6 animate-spin" />
              </div>
            ) : (
              <div className="space-y-2">
                {users.map((u) => (
                  <div
                    key={u.id}
                    className="w-full flex items-center justify-between gap-3 rounded-md border bg-card/40 px-3 py-2 text-left hover:bg-accent transition-colors"
                  >
                    <button onClick={() => loadAssignments(u)} className="flex-1 min-w-0 text-left">
                      <div className="min-w-0">
                        <div className="font-medium truncate">{u.name}</div>
                        <div className="text-xs text-muted-foreground truncate">{u.email}</div>
                      </div>
                    </button>
                    <div className="flex items-center gap-2">
                      <Badge variant={u.role === 'admin' ? 'default' : 'secondary'}>{u.role}</Badge>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => deleteUser(u)}
                        disabled={u.role === 'admin'}
                        aria-label={`Delete ${u.email}`}
                        className="text-destructive hover:text-destructive"
                      >
                        <Trash2 className="h-4 w-4" aria-hidden />
                      </Button>
                    </div>
                  </div>
                ))}
                {users.length === 0 && (
                  <p className="text-sm text-muted-foreground">No users yet.</p>
                )}
              </div>
            )}

            <div className="pt-4 border-t space-y-3">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div className="space-y-1">
                  <label className="text-xs font-semibold text-muted-foreground uppercase">Name</label>
                  <Input value={createName} onChange={(e) => setCreateName(e.target.value)} />
                </div>
                <div className="space-y-1">
                  <label className="text-xs font-semibold text-muted-foreground uppercase">Email</label>
                  <Input value={createEmail} onChange={(e) => setCreateEmail(e.target.value)} />
                </div>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div className="space-y-1">
                  <label className="text-xs font-semibold text-muted-foreground uppercase">Password</label>
                  <Input type="password" value={createPassword} onChange={(e) => setCreatePassword(e.target.value)} />
                </div>
                <div className="space-y-1">
                  <label className="text-xs font-semibold text-muted-foreground uppercase">Role</label>
                  <Select value={createRole} onValueChange={(v) => setCreateRole(v as any)}>
                    <SelectTrigger className="bg-card/50">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="admin">admin</SelectItem>
                      <SelectItem value="manager">manager</SelectItem>
                      <SelectItem value="dev">dev</SelectItem>
                      <SelectItem value="support">support</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <Button
                onClick={createUser}
                disabled={creating || !createEmail || !createName || !createPassword}
                className="w-full"
              >
                {creating ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4 mr-2" />}
                Create user
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Client assignments</CardTitle>
            <CardDescription>Assign API key clients to the selected user.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {!selectedUser ? (
              <p className="text-sm text-muted-foreground">Select a user to manage assignments.</p>
            ) : (
              <>
                <div className="flex items-center justify-between">
                  <div className="min-w-0">
                    <div className="font-medium truncate">{selectedUser.name}</div>
                    <div className="text-xs text-muted-foreground truncate">{selectedUser.email}</div>
                  </div>
                  <Badge variant={selectedUser.role === 'admin' ? 'default' : 'secondary'}>{selectedUser.role}</Badge>
                </div>

                <div className="space-y-2 rounded-lg border p-3">
                  {keys.map((k) => (
                    <label key={k.id} className="flex items-center justify-between gap-3 text-sm">
                      <div className="min-w-0">
                        <div className="font-medium truncate">{k.name}</div>
                        <div className="text-xs text-muted-foreground truncate">{k.prefix}</div>
                      </div>
                      <input
                        type="checkbox"
                        className="h-4 w-4"
                        checked={Boolean(assignments[k.id])}
                        onChange={(e) => setAssignments((p) => ({ ...p, [k.id]: e.target.checked }))}
                      />
                    </label>
                  ))}
                  {keys.length === 0 && <p className="text-sm text-muted-foreground">No API keys yet.</p>}
                </div>
              </>
            )}
          </CardContent>
          <CardFooter className="bg-muted/50 py-3 flex justify-end">
            <Button disabled={!selectedUser || savingAssign} onClick={saveAssignments}>
              {savingAssign ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4 mr-2" />}
              Save assignments
            </Button>
          </CardFooter>
        </Card>
      </div>
    </div>
  )
}

