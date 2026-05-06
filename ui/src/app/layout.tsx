import type { Metadata } from 'next'
import './globals.css'
import { Sidebar } from '@/components/shared/sidebar'
import { Providers } from './providers'
import { AuthProvider } from '@/components/auth/auth-provider'
import { AuthGuard } from '@/components/auth/auth-guard'
import { Toaster } from 'sonner'

export const metadata: Metadata = {
  title: 'NotifyHub — Notification Service',
  description: 'Production notification service dashboard',
  icons: {
    icon: '/assets/logo_small.png',
    apple: '/assets/logo_large.png',
  },
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        {/* Theme: only 'dark'/'light' are acted on — no arbitrary value can affect the DOM. */}
        <script
          dangerouslySetInnerHTML={{
            __html: `(()=>{try{const v=localStorage.getItem('notifyhub-theme');if(v==='dark')document.documentElement.classList.add('dark');else if(v==='light')document.documentElement.classList.remove('dark');}catch{}})();`,
          }}
        />
      </head>
      <body className="font-sans antialiased">
        <AuthProvider>
          <Providers>
            <AuthGuard>
              <div className="flex min-h-screen bg-background">
                <Sidebar />
                <main className="flex-1 overflow-auto">
                  <div className="container mx-auto max-w-7xl px-4 py-8 lg:px-8">
                    {children}
                  </div>
                </main>
              </div>
            </AuthGuard>
            <Toaster position="top-right" richColors />
          </Providers>
        </AuthProvider>
      </body>
    </html>
  )
}
