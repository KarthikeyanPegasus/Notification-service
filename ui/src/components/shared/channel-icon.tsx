import { Mail, MessageSquare, Bell, Globe, Webhook, type LucideIcon } from 'lucide-react'
import type { Channel } from '@/types'
import { cn } from '@/lib/utils'

interface ChannelIconProps {
  channel: Channel
  className?: string
  size?: number
}

const channelIcons: Record<Channel, LucideIcon> = {
  email:     Mail,
  sms:       MessageSquare,
  push:      Bell,
  websocket: Globe,
  webhook:   Webhook,
}

const channelColors: Record<Channel, string> = {
  email:     'text-blue-500',
  sms:       'text-green-500',
  push:      'text-orange-500',
  websocket: 'text-cyan-500',
  webhook:   'text-pink-500',
}

export function ChannelIcon({ channel, className, size = 16 }: ChannelIconProps) {
  if (!channel) return <Globe size={size} className={className} />
  const Icon = channelIcons[channel] ?? Globe
  return <Icon size={size} className={cn(channelColors[channel], className)} />
}

export function channelLabel(channel: Channel): string {
  if (!channel || typeof channel !== 'string') return 'Unknown'
  return channel.charAt(0).toUpperCase() + channel.slice(1)
}
