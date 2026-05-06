'use client'

import { useEffect, useMemo, useState } from 'react'
import { PageHeader } from '@/components/shared/page-header'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Loader2, Mail, Save, Trash2, Users } from 'lucide-react'
import { fetchJSON, listApiKeys, type ApiClientKey } from '@/lib/api'
import { useMaybeAuth } from '@/components/auth/maybe-clerk'
import { useUserRole } from '@/hooks/use-user-role'

type Role = 'admin' | 'manager' | 'dev' | 'support'
type User = { id: string; email: string; name: string; role: Role; created_at: string }

export default function PeoplePage() {
  const { isLoaded, isSignedIn } = useMaybeAuth()
  const callerRole = useUserRole()
  const isSandbox = callerRole === 'dev'

  const [users, setUsers] = useState<User[]>([])
  const [keys, setKeys] = useState<ApiClientKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState<Role>('dev')
  const [inviting, setInviting] = useState(false)

  const [selectedUser, setSelectedUser] = useState<User | null>(null)
  const [assignments, setAssignments] = useState<Record<string, boolean>>({})
  const selectedAssignments = useMemo(() => Object.entries(assignments).filter(([, v]) => v).map(([k]) => k), [assignments])
  // Sandbox limit: count non-admin users. Dev themselves + 1 invited = 2 max.
  const nonAdminCount = useMemo(() => users.filter((u) => u.role !== 'admin').length, [users])
  const sandboxInviteLimitReached = isSandbox && nonAdminCount >= 2
  const [savingAssign, setSavingAssign] = useState(false)
  const [selectedRole, setSelectedRole] = useState<Role>('dev')
  const [savingRole, setSavingRole] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const u = await fetchJSON<User[]>('/v1/admin/users')
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
    if (isLoaded && isSignedIn) load()
  }, [isLoaded, isSignedIn])

  const inviteUser = async () => {
    setInviting(true)
    setError(null)
    try {
      const res = await fetchJSON<{ id: string; email: string; role: string; status: string; message: string }>('/v1/admin/invitations', {
        method: 'POST',
        body: JSON.stringify({ email: inviteEmail, role: inviteRole }),
      })
      setInviteEmail('')
      setInviteRole('dev')
      await load()
    } catch (e) {
      console.error(e)
      setError('Failed to send invitation.')
    } finally {
      setInviting(false)
    }
  }

  const loadAssignments = async (u: User) => {
    setSelectedUser(u)
    setSelectedRole(u.role)
    setAssignments({})
    try {
      const out = await fetchJSON<{ api_key_ids: string[] }>(`/v1/admin/users/${u.id}/clients`)
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

  const updateRole = async () => {
    if (!selectedUser) return
    setSavingRole(true)
    setError(null)
    try {
      await fetchJSON(`/v1/admin/users/${selectedUser.id}/role`, {
        method: 'PUT',
        body: JSON.stringify({ role: selectedRole }),
      })
      setSelectedUser((u) => u ? { ...u, role: selectedRole } : u)
      setUsers((prev) => prev.map((u) => u.id === selectedUser.id ? { ...u, role: selectedRole } : u))
    } catch (e) {
      console.error(e)
      setError('Failed to update role.')
    } finally {
      setSavingRole(false)
    }
  }

  const saveAssignments = async () => {
    if (!selectedUser) return
    setSavingAssign(true)
    setError(null)
    try {
      await fetchJSON(`/v1/admin/users/${selectedUser.id}/clients`, {
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
      await fetchJSON<void>(`/v1/admin/users/${u.id}`, { method: 'DELETE' })
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
              <p className="text-xs text-muted-foreground">
                Send an invitation email via Clerk. The user will sign up and have their role assigned automatically.
              </p>
              <div className="space-y-1">
                <label className="text-xs font-semibold text-muted-foreground uppercase">Email</label>
                <Input
                  type="email"
                  placeholder="colleague@company.com"
                  value={inviteEmail}
                  onChange={(e) => setInviteEmail(e.target.value)}
                />
              </div>
              <div className="space-y-1">
                <label className="text-xs font-semibold text-muted-foreground uppercase">Role</label>
                <Select value={inviteRole} onValueChange={(v) => setInviteRole(v as any)}>
                  <SelectTrigger className="bg-card/50">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {!isSandbox && <SelectItem value="admin">admin</SelectItem>}
                    {!isSandbox && <SelectItem value="manager">manager</SelectItem>}
                    <SelectItem value="dev">dev</SelectItem>
                    <SelectItem value="support">support</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {sandboxInviteLimitReached && (
                <p className="text-xs text-amber-600 dark:text-amber-500">
                  Sandbox accounts are limited to 1 invited user.
                </p>
              )}
              <Button
                onClick={inviteUser}
                disabled={inviting || !inviteEmail || sandboxInviteLimitReached}
                className="w-full"
              >
                {inviting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Mail className="h-4 w-4 mr-2" />}
                Send invitation
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
                <div className="space-y-3 rounded-lg border p-3">
                  <div className="min-w-0">
                    <div className="font-medium truncate">{selectedUser.name}</div>
                    <div className="text-xs text-muted-foreground truncate">{selectedUser.email}</div>
                  </div>
                  <div className="flex items-center gap-2">
                    <label className="text-xs font-semibold text-muted-foreground uppercase shrink-0">Role</label>
                    <Select value={selectedRole} onValueChange={(v) => setSelectedRole(v as Role)}>
                      <SelectTrigger className="h-8 text-sm bg-card/50">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {!isSandbox && <SelectItem value="admin">admin</SelectItem>}
                        {!isSandbox && <SelectItem value="manager">manager</SelectItem>}
                        <SelectItem value="dev">dev</SelectItem>
                        <SelectItem value="support">support</SelectItem>
                      </SelectContent>
                    </Select>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={savingRole || selectedRole === selectedUser.role}
                      onClick={updateRole}
                      className="shrink-0"
                    >
                      {savingRole ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Update'}
                    </Button>
                  </div>
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

