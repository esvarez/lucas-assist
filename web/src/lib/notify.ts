import { toast } from 'sonner'

// Thin wrapper over sonner so forms/dialogs depend on this, not the
// toast primitive directly — swapping the underlying library later
// touches one file instead of every call site.
export const notify = {
  success: (message: string) => toast.success(message),
  error: (message: string, options?: { id?: string | number; retry?: () => void }) =>
    toast.error(message, {
      id: options?.id,
      action: options?.retry ? { label: 'Retry', onClick: options.retry } : undefined,
    }),
  // loading/updateLoading/action all take or return sonner's toast id so a
  // caller can carry one toast through a background job's progress ->
  // completion transition (#166) instead of stacking new toasts.
  loading: (message: string) => toast.loading(message),
  updateLoading: (id: string | number, message: string) => toast.loading(message, { id }),
  // duration: Infinity — a "ready to review" or "needs an answer" toast
  // shouldn't disappear before the user notices it.
  action: (id: string | number | null, message: string, action: { label: string; onClick: () => void }) =>
    toast(message, { id: id ?? undefined, action, duration: Infinity }),
  dismiss: (id: string | number) => toast.dismiss(id),
}
