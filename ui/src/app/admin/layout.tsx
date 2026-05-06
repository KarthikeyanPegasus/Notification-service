import { RequireRole } from '@/components/auth/require-role'

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return <RequireRole roles={['admin', 'manager', 'dev', 'support']}>{children}</RequireRole>
}
