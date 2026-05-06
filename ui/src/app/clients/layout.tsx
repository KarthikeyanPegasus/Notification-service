import { RequireRole } from '@/components/auth/require-role'

export default function ClientsLayout({ children }: { children: React.ReactNode }) {
  return <RequireRole roles={['admin', 'manager', 'dev']}>{children}</RequireRole>
}
