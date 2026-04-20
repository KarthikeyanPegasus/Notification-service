'use client'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { TemplateForm } from './template-form'

interface TemplateDialogProps {
  isOpen: boolean
  onClose: () => void
  template?: any
  onSuccess: () => void
}

export function TemplateDialog({
  isOpen,
  onClose,
  template,
  onSuccess,
}: TemplateDialogProps) {
  const handleSuccess = () => {
    onSuccess()
    onClose()
  }

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="max-w-2xl border-none shadow-2xl ring-1 ring-black/5">
        <DialogHeader>
          <DialogTitle>{template ? 'Edit Template' : 'Create New Template'}</DialogTitle>
          <DialogDescription>
            {template 
              ? 'Modify your template content and channel settings.' 
              : 'Define a new reusable template for your notifications.'}
          </DialogDescription>
        </DialogHeader>
        <TemplateForm 
          template={template} 
          onSuccess={handleSuccess} 
          onCancel={onClose} 
        />
      </DialogContent>
    </Dialog>
  )
}
