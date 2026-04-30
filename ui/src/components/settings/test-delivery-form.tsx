'use client'

import React, { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { Send, CheckCircle2, AlertCircle, Loader2 } from 'lucide-react'
import { sendVendorTest } from '@/lib/api'

interface TestDeliveryFormProps {
  vendorType: string
  channel: 'sms' | 'email' | 'slack'
  orchestrationMode?: string
  apiKeyId?: string
  onSuccess?: () => void
}

export function TestDeliveryForm({ vendorType, channel, orchestrationMode = 'temporal', apiKeyId, onSuccess }: TestDeliveryFormProps) {
  const [recipient, setRecipient] = useState('')
  const [body, setBody] = useState(`NotifyHub test via ${vendorType}`)
  const [isLoading, setIsLoading] = useState(false)
  const [status, setStatus] = useState<'idle' | 'success' | 'error'>('idle')
  const [error, setError] = useState('')
  const [notifId, setNotifId] = useState('')

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!recipient.trim()) return

    setIsLoading(true)
    setStatus('idle')
    setError('')

    try {
      const res = await sendVendorTest(vendorType, {
        channel,
        recipient,
        body,
        subject: channel === 'email' ? 'NotifyHub Test Message' : undefined,
      }, apiKeyId)

      setStatus('success')
      setNotifId(String((res as any)?.notification_id ?? (res as any)?.id ?? ''))
      setRecipient('')
      onSuccess?.()
      // Reset success message after 5 seconds
      setTimeout(() => setStatus('idle'), 5000)
    } catch (err: any) {
      console.error('Test delivery failed:', err)
      setStatus('error')
      setError(err.message || 'An unexpected error occurred')
    } finally {
      setIsLoading(false)
    }
  }

  const getPlaceholder = () => {
    switch (channel) {
      case 'email': return 'user@example.com'
      case 'sms': return '+1234567890'
      case 'slack': return '#channel-name or webhook url'
      default: return 'Recipient'
    }
  }

  return (
    <div className="mt-4 p-4 rounded-2xl bg-white/5 border border-white/10 overflow-hidden">
      <div className="flex items-center justify-between mb-4">
        <h4 className="text-sm font-medium text-white/50 flex items-center gap-2">
          <Send className="w-3.5 h-3.5" />
          Send Test {channel.toUpperCase()}
        </h4>
        <div className="flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-blue-500/10 border border-blue-500/20">
          <div className={`w-1.5 h-1.5 rounded-full ${orchestrationMode === 'standalone' ? 'bg-amber-400' : 'bg-blue-400'} animate-pulse`} />
          <span className="text-[10px] font-semibold uppercase tracking-wider text-blue-300/80">
            {orchestrationMode} Mode
          </span>
        </div>
      </div>

      <form onSubmit={handleSend} className="space-y-4">
        <div className="space-y-1.5">
          <label className="text-xs text-white/40 ml-1">Recipient</label>
          <input
            type="text"
            value={recipient}
            onChange={(e) => setRecipient(e.target.value)}
            placeholder={getPlaceholder()}
            required
            className="w-full bg-black/40 border border-white/10 rounded-xl px-4 py-2.5 text-sm text-white placeholder:text-white/20 focus:outline-none focus:ring-2 focus:ring-blue-500/50 transition-all font-mono"
          />
        </div>

        <div className="space-y-1.5">
          <label className="text-xs text-white/40 ml-1">Message Body</label>
          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={2}
            className="w-full bg-black/40 border border-white/10 rounded-xl px-4 py-2.5 text-sm text-white placeholder:text-white/20 focus:outline-none focus:ring-2 focus:ring-blue-500/50 transition-all resize-none"
          />
        </div>

        <button
          type="submit"
          disabled={isLoading || !recipient.trim()}
          className={`
            w-full py-2.5 rounded-xl text-sm font-medium transition-all flex items-center justify-center gap-2
            ${isLoading 
              ? 'bg-blue-500/20 text-blue-300 cursor-not-allowed' 
              : 'bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-500/20 active:scale-[0.98]'}
          `}
        >
          {isLoading ? (
            <Loader2 className="w-4 h-4 animate-spin" />
          ) : (
            <Send className="w-4 h-4" />
          )}
          {isLoading ? 'Triggering Workflow...' : 'Deliver Test'}
        </button>
      </form>

      <AnimatePresence>
        {status === 'success' && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            className="mt-4 p-3 rounded-xl bg-green-500/10 border border-green-500/20 flex items-start gap-3"
          >
            <CheckCircle2 className="w-5 h-5 text-green-500 shrink-0 mt-0.5" />
            <div className="text-xs text-green-200/80 leading-relaxed">
              <p className="font-semibold text-green-400 mb-1">Workflow Triggered!</p>
              Your test message has been queued. 
              {notifId && (
                <div className="mt-1 font-mono text-[10px] bg-black/40 p-1 px-2 rounded">
                  ID: {notifId}
                </div>
              )}
            </div>
          </motion.div>
        )}

        {status === 'error' && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            className="mt-4 p-3 rounded-xl bg-red-500/10 border border-red-500/20 flex items-start gap-3"
          >
            <AlertCircle className="w-5 h-5 text-red-500 shrink-0 mt-0.5" />
            <div className="text-xs text-red-200/80 leading-relaxed">
              <p className="font-semibold text-red-400 mb-1">Send Failed</p>
              {error}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}
