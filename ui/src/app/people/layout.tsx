import { RequireRole } from '@/components/auth/require-role'

export default function PeopleLayout({ children }: { children: React.ReactNode }) {
  return <RequireRole roles={['admin', 'dev']}>{children}</RequireRole>
}
