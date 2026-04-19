import type { Metadata } from 'next'
import './globals.css'
import { Sidebar } from '@/components/shared/sidebar'
import { Providers } from './providers'

export const metadata: Metadata = {
  title: 'NotifyHub — Notification Service',
  description: 'Production notification service dashboard',
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="font-sans antialiased">
        <Providers>
          <div className="flex min-h-screen bg-background">
            <Sidebar />
            <main className="flex-1 overflow-auto">
              <div className="container mx-auto max-w-7xl px-4 py-8 lg:px-8">
                {children}
              </div>
            </main>
          </div>
        </Providers>
      </body>
    </html>
  )
}
