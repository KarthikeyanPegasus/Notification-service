import { 
  Mail, 
  MessageSquare, 
  Bell, 
  MessageCircle, 
  Slack, 
  Github, 
  Globe, 
  Hash,
  Send,
  Smartphone,
  Webhook,
  Zap
} from 'lucide-react'

export type Category = 'email' | 'sms' | 'push' | 'social' | 'webhooks'

export interface Vendor {
  id: string
  name: string
  description: string
  category: Category
  icon: any
  color: string
}

export const VENDORS: Vendor[] = [
  // SMS
  { id: 'twilio', name: 'Twilio', description: 'Global SMS, Voice, and WhatsApp delivery.', category: 'sms', icon: MessageSquare, color: '#F22F46' },
  { id: 'plivo', name: 'Plivo', description: 'SMS and Voice platform for businesses.', category: 'sms', icon: Hash, color: '#4BA1E6' },
  { id: 'vonage', name: 'Vonage', description: 'Unified communications and SMS APIs.', category: 'sms', icon: Smartphone, color: '#000000' },
  { id: 'messagebird', name: 'MessageBird', description: 'Omnichannel communication platform.', category: 'sms', icon: Send, color: '#3247C4' },

  // Email
  { id: 'ses', name: 'Amazon SES', description: 'High-scale outbound and inbound email service.', category: 'email', icon: Mail, color: '#FF9900' },
  { id: 'sendgrid', name: 'SendGrid', description: 'Email delivery and marketing campaigns.', category: 'email', icon: Mail, color: '#00B3FF' },
  { id: 'mailgun', name: 'Mailgun', description: 'Email for developers and transactional power.', category: 'email', icon: Send, color: '#BB1C2F' },
  { id: 'postmark', name: 'Postmark', description: 'The email service that delivers on time.', category: 'email', icon: Github, color: '#F7CC42' },

  // Push
  { id: 'fcm', name: 'Firebase', description: 'Google cloud messaging for App push.', category: 'push', icon: Zap, color: '#FFCA28' },
  { id: 'onesignal', name: 'OneSignal', description: 'Omnichannel customer engagement platform.', category: 'push', icon: Bell, color: '#E44B4D' },
  { id: 'pusher', name: 'Pusher', description: 'Real-time infrastructure for modern apps.', category: 'push', icon: Zap, color: '#3247C4' },

  // Social/App
  { id: 'slack', name: 'Slack', description: 'Connect directly to your team workspace.', category: 'social', icon: Slack, color: '#4A154B' },
  { id: 'discord', name: 'Discord', description: 'Webhooks and bot support for your server.', category: 'social', icon: MessageCircle, color: '#5865F2' },
  { id: 'teams', name: 'MS Teams', description: 'Enterprise messaging for collaborators.', category: 'social', icon: Globe, color: '#6264A7' },
  { id: 'telegram', name: 'Telegram', description: 'Deliver notifications via Telegram bots.', category: 'social', icon: Send, color: '#26A5E4' },

  // Webhooks
  { id: 'webhooks', name: 'Custom Webhooks', description: 'Forward notifications to any endpoint.', category: 'webhooks', icon: Webhook, color: '#34D399' },
]

export const CATEGORIES: { id: Category; label: string; icon: any }[] = [
  { id: 'email', label: 'Email', icon: Mail },
  { id: 'sms', label: 'SMS', icon: MessageSquare },
  { id: 'push', label: 'Push Buff', icon: Bell },
  { id: 'social', label: 'Social & Apps', icon: Slack },
  { id: 'webhooks', label: 'Webhooks', icon: Webhook },
]
