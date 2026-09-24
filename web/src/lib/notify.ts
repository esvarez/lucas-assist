import { toast } from 'sonner'

// Thin wrapper over sonner so forms/dialogs depend on this, not the
// toast primitive directly — swapping the underlying library later
// touches one file instead of every call site.
export const notify = {
  success: (message: string) => toast.success(message),
  error: (message: string, options?: { retry?: () => void }) =>
    toast.error(message, {
      action: options?.retry ? { label: 'Retry', onClick: options.retry } : undefined,
    }),
}
