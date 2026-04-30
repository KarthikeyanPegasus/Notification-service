import React from 'react'
import { Icon } from '@iconify/react'
import type { IconifyIcon } from '@iconify/react'
import {
  Bell,
  Globe,
  Mail,
  MessageCircle,
  MessageSquare,
  Send,
  Smartphone,
  Webhook,
  Hash,
  Zap,
} from 'lucide-react'

export type Category = 'email' | 'sms' | 'push' | 'social' | 'webhooks'

export interface Vendor {
  id: string
  name: string
  description: string
  category: Category
  icon: React.ComponentType<{ className?: string }>
  color: string
  /** Official vendor console / dashboard URL */
  dashboardUrl?: string
}

function iconify(icon: IconifyIcon): React.FC<{ className?: string }> {
  return function IconifyIcon({ className }) {
    // Keep this file as `.ts` (no JSX). Wrapper ensures Tailwind `h-* w-*` works reliably.
    return React.createElement(
      'span',
      { className, 'aria-hidden': true },
      React.createElement(Icon, { icon, width: '100%', height: '100%' }),
    )
  }
}

// Brand icons (bundled at build time; no runtime fetch).
import twilioIcon from '@iconify-icons/simple-icons/twilio'
import vonageIcon from '@iconify-icons/simple-icons/vonage'
import awsIcon from '@iconify-icons/simple-icons/amazonaws'
import mailgunIcon from '@iconify-icons/simple-icons/mailgun'
import firebaseIcon from '@iconify-icons/simple-icons/firebase'
import pusherIcon from '@iconify-icons/simple-icons/pusher'
import slackIcon from '@iconify-icons/simple-icons/slack'
import discordIcon from '@iconify-icons/simple-icons/discord'
import teamsIcon from '@iconify-icons/simple-icons/microsoftteams'
import telegramIcon from '@iconify-icons/simple-icons/telegram'

import sendgridLogo from '@iconify-icons/logos/sendgrid'
import onesignalLogo from '@iconify-icons/logos/onesignal'

export const VENDORS: Vendor[] = [
  // SMS
  // Dashboard: https://console.twilio.com/ (SMS logs + delivery stats)
  { id: 'twilio', name: 'Twilio', description: 'Global SMS, Voice, and WhatsApp delivery.', category: 'sms', icon: iconify(twilioIcon), color: '#F22F46', dashboardUrl: 'https://console.twilio.com/us1/monitor/logs/sms' },
  // Dashboard: https://console.plivo.com/ (SMS logs)
  { id: 'plivo', name: 'Plivo', description: 'SMS and Voice platform for businesses.', category: 'sms', icon: Hash, color: '#4BA1E6', dashboardUrl: 'https://console.plivo.com/sms/logs/' },
  // Dashboard: https://dashboard.nexmo.com/ (message search & analytics)
  { id: 'vonage', name: 'Vonage', description: 'Unified communications and SMS APIs.', category: 'sms', icon: iconify(vonageIcon), color: '#000000', dashboardUrl: 'https://dashboard.nexmo.com/messages/sms' },
  // Dashboard: https://dashboard.messagebird.com/ (statistics)
  { id: 'messagebird', name: 'MessageBird', description: 'Omnichannel communication platform.', category: 'sms', icon: Smartphone, color: '#3247C4', dashboardUrl: 'https://dashboard.messagebird.com/en/statistics' },

  // Email
  // Reputation dashboard shows bounce/complaint rates: https://console.aws.amazon.com/ses/home#/reputation
  { id: 'ses', name: 'Amazon SES', description: 'High-scale outbound and inbound email service.', category: 'email', icon: iconify(awsIcon), color: '#FF9900', dashboardUrl: 'https://console.aws.amazon.com/ses/home#/reputation' },
  // Suppression lists & bounce reports: https://app.sendgrid.com/suppressions/bounces
  { id: 'sendgrid', name: 'SendGrid', description: 'Email delivery and marketing campaigns.', category: 'email', icon: iconify(sendgridLogo), color: '#00B3FF', dashboardUrl: 'https://app.sendgrid.com/suppressions/bounces' },
  // Analytics overview with bounce/complaint stats: https://app.mailgun.com/app/analytics/overview
  { id: 'mailgun', name: 'Mailgun', description: 'Email for developers and transactional power.', category: 'email', icon: iconify(mailgunIcon), color: '#BB1C2F', dashboardUrl: 'https://app.mailgun.com/app/analytics/overview' },
  // Bounce/spam tracking per server: https://account.postmarkapp.com/servers
  { id: 'postmark', name: 'Postmark', description: 'The email service that delivers on time.', category: 'email', icon: Mail, color: '#F7CC42', dashboardUrl: 'https://account.postmarkapp.com/servers' },

  // Push
  { id: 'fcm', name: 'Firebase', description: 'Google cloud messaging for App push.', category: 'push', icon: iconify(firebaseIcon), color: '#FFCA28', dashboardUrl: 'https://console.firebase.google.com/' },
  { id: 'onesignal', name: 'OneSignal', description: 'Omnichannel customer engagement platform.', category: 'push', icon: iconify(onesignalLogo), color: '#E44B4D', dashboardUrl: 'https://app.onesignal.com/' },
  { id: 'pusher', name: 'Pusher', description: 'Real-time infrastructure for modern apps.', category: 'push', icon: iconify(pusherIcon), color: '#3247C4', dashboardUrl: 'https://dashboard.pusher.com/' },

  // Social/App
  { id: 'slack', name: 'Slack', description: 'Connect directly to your team workspace.', category: 'social', icon: iconify(slackIcon), color: '#4A154B', dashboardUrl: 'https://api.slack.com/apps' },
  { id: 'discord', name: 'Discord', description: 'Webhooks and bot support for your server.', category: 'social', icon: iconify(discordIcon), color: '#5865F2', dashboardUrl: 'https://discord.com/developers/applications' },
  { id: 'teams', name: 'MS Teams', description: 'Enterprise messaging for collaborators.', category: 'social', icon: iconify(teamsIcon), color: '#6264A7', dashboardUrl: 'https://admin.teams.microsoft.com/' },
  { id: 'telegram', name: 'Telegram', description: 'Deliver notifications via Telegram bots.', category: 'social', icon: iconify(telegramIcon), color: '#26A5E4', dashboardUrl: 'https://t.me/BotFather' },

  // Webhooks
  { id: 'webhooks', name: 'Custom Webhooks', description: 'Forward notifications to any endpoint.', category: 'webhooks', icon: Webhook, color: '#34D399' },
]

export const CATEGORIES: { id: Category; label: string; icon: any }[] = [
  { id: 'email', label: 'Email', icon: Mail },
  { id: 'sms', label: 'SMS', icon: MessageSquare },
  { id: 'push', label: 'Push Buff', icon: Bell },
  { id: 'social', label: 'Social & Apps', icon: Send },
  { id: 'webhooks', label: 'Webhooks', icon: Webhook },
]
