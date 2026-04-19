'use client'

import React, { useState, useEffect } from 'react'
import { PageHeader } from '@/components/shared/page-header'
import { Card, CardContent, CardDescription, CardHeader, CardTitle, CardFooter } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { VENDORS, CATEGORIES, Vendor, Category } from '@/lib/vendors'
import { getVendorConfigs, updateVendorConfig, VendorConfig } from '@/lib/api'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Loader2, Search, CheckCircle2, AlertCircle, ArrowRight, Settings, Upload, FileJson, Terminal, Info } from 'lucide-react'
import { cn } from '@/lib/utils'

export default function AppStorePage() {
  const [selectedCategory, setSelectedCategory] = useState<Category | 'all'>('all')
  const [searchQuery, setSearchQuery] = useState('')
  const [connectedVendors, setConnectedVendors] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)
  const [activeVendor, setActiveVendor] = useState<Vendor | null>(null)
  const [formData, setFormData] = useState<Record<string, any>>({})
  const [isAdvancedMode, setIsAdvancedMode] = useState(false)
  const [configJson, setConfigJson] = useState('{\n  \n}')
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error', text: string } | null>(null)

  useEffect(() => {
    loadConnectedVendors()
  }, [])

  const loadConnectedVendors = async () => {
    try {
      setLoading(true)
      const data = await getVendorConfigs()
      const connected = new Set(data.map(v => v.vendor_type))
      setConnectedVendors(connected)
    } catch (err) {
      console.error('Failed to load configs', err)
    } finally {
      setLoading(false)
    }
  }

  const handleConnect = async () => {
    if (!activeVendor) return
    try {
      setSaving(true)
      let payload: any
      
      if (isAdvancedMode) {
        payload = JSON.parse(configJson)
      } else {
        payload = { ...formData }
      }

      await updateVendorConfig(activeVendor.id, payload)
      setMessage({ type: 'success', text: `Successfully connected to ${activeVendor.name}!` })
      setConnectedVendors(prev => new Set(prev).add(activeVendor.id))
      setTimeout(() => {
        setActiveVendor(null)
        setMessage(null)
        setFormData({})
      }, 1500)
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message || 'Failed to connect. Please check your inputs.' })
    } finally {
      setSaving(false)
    }
  }

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return

    const reader = new FileReader()
    reader.onload = (event) => {
      try {
        const json = JSON.parse(event.target?.result as string)
        if (activeVendor?.id === 'fcm') {
          setFormData(prev => ({ ...prev, service_account: json }))
        } else {
          setFormData(json)
        }
        setMessage({ type: 'success', text: `${file.name} loaded successfully!` })
        setTimeout(() => setMessage(null), 2000)
      } catch (err) {
        setMessage({ type: 'error', text: 'Invalid JSON file.' })
      }
    }
    reader.readAsText(file)
  }

  const updateField = (key: string, value: any) => {
    setFormData(prev => ({ ...prev, [key]: value }))
  }

  const renderVendorForm = () => {
    if (!activeVendor) return null

    if (isAdvancedMode) {
      return (
        <div className="space-y-2">
          <label className="text-xs font-bold text-muted-foreground uppercase flex items-center justify-between">
            JSON Configuration
            <Badge variant="outline" className="font-mono text-[9px]">application/json</Badge>
          </label>
          <textarea
            className="flex w-full rounded-md border border-input bg-background/50 px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 min-h-[200px] font-mono leading-relaxed"
            value={configJson}
            onChange={(e) => setConfigJson(e.target.value)}
            placeholder='{ "key": "value" }'
          />
        </div>
      )
    }

    switch (activeVendor.id) {
      case 'ses':
        return (
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <label className="text-sm font-medium">AWS Region</label>
                <Input 
                  placeholder="us-east-1"
                  value={formData.region || ''}
                  onChange={e => updateField('region', e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <label className="text-sm font-medium">IAM Username</label>
                <Input 
                  placeholder="ses-user"
                  value={formData.iam_username || ''}
                  onChange={e => updateField('iam_username', e.target.value)}
                />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <label className="text-sm font-medium">SMTP Username</label>
                <Input 
                  placeholder="AKIA..."
                  value={formData.smtp_username || ''}
                  onChange={e => updateField('smtp_username', e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <label className="text-sm font-medium">SMTP Password</label>
                <Input 
                  type="password"
                  placeholder="Enter SMTP password"
                  value={formData.smtp_password || ''}
                  onChange={e => updateField('smtp_password', e.target.value)}
                />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <label className="text-sm font-medium">From Email</label>
                <Input 
                  placeholder="sender@domain.com"
                  value={formData.from_address || ''}
                  onChange={e => updateField('from_address', e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <label className="text-sm font-medium">From Name (Optional)</label>
                <Input 
                  placeholder="Notification Service"
                  value={formData.from_name || ''}
                  onChange={e => updateField('from_name', e.target.value)}
                />
              </div>
            </div>
          </div>
        )
      case 'twilio':
        return (
          <div className="space-y-4">
            <div className="grid gap-2">
              <label className="text-sm font-medium">Account SID</label>
              <Input 
                placeholder="ACXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
                value={formData.account_sid || ''}
                onChange={e => updateField('account_sid', e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <label className="text-sm font-medium">Auth Token</label>
              <Input 
                type="password"
                placeholder="Your Twilio auth token"
                value={formData.auth_token || ''}
                onChange={e => updateField('auth_token', e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <label className="text-sm font-medium">From Number</label>
              <Input 
                placeholder="+1234567890" 
                value={formData.from_number || ''}
                onChange={e => updateField('from_number', e.target.value)}
              />
            </div>
          </div>
        )
      case 'sendgrid':
        return (
          <div className="space-y-4">
            <div className="grid gap-2">
              <label className="text-sm font-medium">API Key</label>
              <Input 
                type="password"
                placeholder="SG.xxxxxxxxxxxxxxxx"
                value={formData.api_key || ''}
                onChange={e => updateField('api_key', e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <label className="text-sm font-medium">From Email</label>
              <Input 
                placeholder="notifications@yourdomain.com"
                value={formData.from_email || ''}
                onChange={e => updateField('from_email', e.target.value)}
              />
            </div>
          </div>
        )
      case 'fcm':
        return (
          <div className="space-y-4">
            <div className="grid gap-2">
              <label className="text-sm font-medium">Server Key (Legacy)</label>
              <Input 
                type="password"
                placeholder="AAAA..."
                value={formData.server_key || ''}
                onChange={e => updateField('server_key', e.target.value)}
              />
            </div>
            <div className="relative py-4">
              <div className="absolute inset-0 flex items-center">
                <span className="w-full border-t" />
              </div>
              <div className="relative flex justify-center text-xs uppercase">
                <span className="bg-background px-2 text-muted-foreground">Or Upload Service Account</span>
              </div>
            </div>
            <div className={cn(
              "border-2 border-dashed rounded-lg p-6 flex flex-col items-center justify-center transition-colors text-center",
              formData.service_account ? "border-green-500/50 bg-green-50/50" : "border-muted-foreground/20 hover:border-primary/50 hover:bg-primary/5"
            )}>
              <div className="mb-2 h-10 w-10 rounded-full bg-muted flex items-center justify-center">
                {formData.service_account ? <FileJson className="h-5 w-5 text-green-600" /> : <Upload className="h-5 w-5 text-muted-foreground" />}
              </div>
              <p className="text-sm font-medium">
                {formData.service_account ? "Service account loaded" : "Upload service_account.json"}
              </p>
              <p className="text-xs text-muted-foreground mt-1">Recommended for Firebase HTTP v1 API</p>
              <input 
                type="file" 
                accept=".json"
                className="absolute inset-0 opacity-0 cursor-pointer"
                onChange={handleFileUpload}
              />
            </div>
          </div>
        )
      case 'slack':
      case 'discord':
      case 'webhooks':
        return (
          <div className="space-y-4">
            <div className="grid gap-2">
              <label className="text-sm font-medium">Webhook URL</label>
              <Input 
                placeholder="https://hooks.slack.com/services/..."
                value={formData.webhook_url || ''}
                onChange={e => updateField('webhook_url', e.target.value)}
              />
            </div>
          </div>
        )
      default:
        return (
          <div className="space-y-4">
            <div className="flex items-center gap-2 p-3 rounded-lg bg-blue-50 text-blue-700 text-xs border border-blue-100">
              <Info className="h-4 w-4 shrink-0" />
              This provider uses standard configuration fields. For custom setups, use Advanced Mode.
            </div>
            <div className="grid gap-2">
              <label className="text-sm font-medium">API Key / Token</label>
              <Input 
                type="password"
                placeholder="Enter credential"
                value={formData.api_key || ''}
                onChange={e => updateField('api_key', e.target.value)}
              />
            </div>
          </div>
        )
    }
  }

  const filteredVendors = VENDORS.filter(v => {
    const matchesCategory = selectedCategory === 'all' || v.category === selectedCategory
    const matchesSearch = v.name.toLowerCase().includes(searchQuery.toLowerCase()) || 
                         v.description.toLowerCase().includes(searchQuery.toLowerCase())
    return matchesCategory && matchesSearch
  })

  return (
    <div className="flex flex-col gap-8 p-6 max-w-6xl mx-auto pb-20">
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <PageHeader 
          title="App Store" 
          description="Discover and connect delivery providers for your notifications." 
        />
        <div className="relative w-full md:w-80">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <Input 
            placeholder="Search vendors..." 
            className="pl-10 bg-background/50 backdrop-blur-sm border-muted-foreground/20 focus:border-primary transition-all"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
      </div>

      {/* Categories */}
      <div className="flex flex-wrap gap-2 pb-2 overflow-x-auto no-scrollbar">
        <Button 
          variant={selectedCategory === 'all' ? 'default' : 'outline'}
          size="sm"
          className="rounded-full px-4"
          onClick={() => setSelectedCategory('all')}
        >
          All Vendors
        </Button>
        {CATEGORIES.map(cat => (
          <Button
            key={cat.id}
            variant={selectedCategory === cat.id ? 'default' : 'outline'}
            size="sm"
            className="rounded-full px-4 gap-2"
            onClick={() => setSelectedCategory(cat.id)}
          >
            <cat.icon className="h-3.5 w-3.5" />
            {cat.label}
          </Button>
        ))}
      </div>

      {/* Grid */}
      {loading ? (
        <div className="flex h-64 items-center justify-center">
          <Loader2 className="h-8 w-8 animate-spin text-primary/50" />
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {filteredVendors.map(vendor => {
            const isConnected = connectedVendors.has(vendor.id)
            return (
              <Card 
                key={vendor.id} 
                className="group relative overflow-hidden transition-all duration-300 hover:shadow-2xl hover:-translate-y-1 border-muted-foreground/10 bg-card/40 backdrop-blur-md"
              >
                <div className="absolute inset-0 bg-gradient-to-br from-transparent via-transparent to-primary/5 opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none" />
                <CardHeader className="pb-3">
                  <div className="flex justify-between items-start">
                    <div 
                      className="h-12 w-12 rounded-xl flex items-center justify-center shadow-inner"
                      style={{ backgroundColor: `${vendor.color}15`, color: vendor.color }}
                    >
                      <vendor.icon className="h-6 w-6" />
                    </div>
                    {isConnected && (
                      <Badge variant="secondary" className="bg-green-500/10 text-green-600 hover:bg-green-500/20 border-green-500/20 gap-1 px-2 py-0.5 animate-in fade-in zoom-in duration-300">
                        <CheckCircle2 className="h-3 w-3" />
                        Connected
                      </Badge>
                    )}
                  </div>
                  <CardTitle className="mt-4 text-xl group-hover:text-primary transition-colors">{vendor.name}</CardTitle>
                  <CardDescription className="line-clamp-2 min-h-[40px]">{vendor.description}</CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="flex items-center gap-2">
                    <Badge variant="outline" className="text-[10px] uppercase tracking-wider font-bold opacity-60">
                      {vendor.category}
                    </Badge>
                  </div>
                </CardContent>
                <CardFooter className="pt-2">
                  <Button 
                    variant={isConnected ? "outline" : "default"} 
                    className={cn(
                      "w-full group/btn transition-all",
                      !isConnected && "shadow-lg shadow-primary/20"
                    )}
                    onClick={() => {
                      setActiveVendor(vendor)
                      setIsAdvancedMode(false)
                      // Find existing config if any
                      const existing = Array.from(connectedVendors).includes(vendor.id)
                      if (existing) {
                        // In a real app we'd fetch the existing config here
                        // For now we'll just reset or keep current state
                      }
                      setFormData({})
                      setConfigJson('{\n  \n}')
                    }}
                  >
                    {isConnected ? (
                      <>
                        <Settings className="mr-2 h-4 w-4" />
                        Manage Configuration
                      </>
                    ) : (
                      <>
                        Connect Provider
                        <ArrowRight className="ml-2 h-4 w-4 transition-transform group-hover/btn:translate-x-1" />
                      </>
                    )}
                  </Button>
                </CardFooter>
              </Card>
            )
          })}
        </div>
      )}

      {/* Empty State */}
      {!loading && filteredVendors.length === 0 && (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <div className="h-20 w-20 bg-muted rounded-full flex items-center justify-center mb-4">
            <Search className="h-10 w-10 text-muted-foreground" />
          </div>
          <h3 className="text-xl font-semibold">No vendors found</h3>
          <p className="text-muted-foreground mt-2">Try searching for a different keyword or category.</p>
          <Button variant="link" onClick={() => {setSearchQuery(''); setSelectedCategory('all')}}>Clear filters</Button>
        </div>
      )}

      {/* Connect Modal */}
      <Dialog open={!!activeVendor} onOpenChange={(open) => !open && setActiveVendor(null)}>
        <DialogContent className="sm:max-w-[500px]">
          <DialogHeader>
            <div className="flex items-center gap-4">
              <div 
                className="h-10 w-10 rounded-lg flex items-center justify-center"
                style={{ backgroundColor: `${activeVendor?.color}15`, color: activeVendor?.color }}
              >
                {activeVendor && <activeVendor.icon className="h-5 w-5" />}
              </div>
              <div className="flex flex-col">
                <CardTitle>Connect to {activeVendor?.name}</CardTitle>
                <DialogDescription>
                  Enter your configuration settings for {activeVendor?.category.toUpperCase()} delivery.
                </DialogDescription>
              </div>
            </div>
          </DialogHeader>

          <div className="py-6 space-y-4">
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2 text-xs font-semibold text-muted-foreground uppercase">
                <Terminal className="h-3 w-3" />
                Connection Profile
              </div>
              <Button 
                variant="ghost" 
                size="sm" 
                className="h-7 text-[10px] uppercase font-bold"
                onClick={() => setIsAdvancedMode(!isAdvancedMode)}
              >
                {isAdvancedMode ? "Back to Form" : "Advanced JSON"}
              </Button>
            </div>

            <div className="animate-in fade-in duration-300">
              {renderVendorForm()}
            </div>

            {message && (
              <div className={cn(
                "flex items-start gap-2 p-3 rounded-lg border text-sm animate-in slide-in-from-top-1 duration-300",
                message.type === 'success' ? "bg-green-50 border-green-200 text-green-700" : "bg-destructive/5 border-destructive/10 text-destructive"
              )}>
                {message.type === 'success' ? <CheckCircle2 className="h-4 w-4 mt-0.5 shrink-0" /> : <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />}
                {message.text}
              </div>
            )}
          </div>

          <DialogFooter>
            <Button variant="ghost" onClick={() => setActiveVendor(null)} disabled={saving}>Cancel</Button>
            <Button 
              className="px-8 shadow-lg shadow-primary/20"
              disabled={saving} 
              onClick={handleConnect}
            >
              {saving ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
              {connectedVendors.has(activeVendor?.id || '') ? 'Update Connection' : 'Establish Connection'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
