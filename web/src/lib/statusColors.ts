/**
 * Centralized status-to-color and status-to-icon mapping.
 * All status display logic should use these helpers for consistency.
 */

export type StatusVariant = 'success' | 'error' | 'running' | 'warning' | 'pending';

export type StatusIconType = 'check' | 'x' | 'spinner' | 'clock' | 'none';

interface StatusMeta {
  variant: StatusVariant;
  iconType: StatusIconType;
  pulse: boolean;
}

const STATUS_META: Record<string, StatusMeta> = {
  connected: { variant: 'success', iconType: 'check', pulse: false },
  online: { variant: 'success', iconType: 'check', pulse: false },
  completed: { variant: 'success', iconType: 'check', pulse: false },
  success: { variant: 'success', iconType: 'check', pulse: false },

  disconnected: { variant: 'error', iconType: 'x', pulse: false },
  offline: { variant: 'error', iconType: 'x', pulse: false },
  terminated: { variant: 'error', iconType: 'x', pulse: false },
  failed: { variant: 'error', iconType: 'x', pulse: false },
  error: { variant: 'error', iconType: 'x', pulse: false },
  cancelled: { variant: 'error', iconType: 'x', pulse: false },

  running: { variant: 'running', iconType: 'spinner', pulse: true },

  waiting_for_input: { variant: 'warning', iconType: 'clock', pulse: false },

  pending: { variant: 'warning', iconType: 'spinner', pulse: false },

  created: { variant: 'pending', iconType: 'clock', pulse: false },
};

const DEFAULT_META: StatusMeta = {
  variant: 'pending',
  iconType: 'clock',
  pulse: false,
};

const VARIANT_DOT_CLASS: Record<StatusVariant, string> = {
  success: 'bg-status-success',
  error: 'bg-status-error',
  running: 'bg-status-running',
  warning: 'bg-status-warning',
  pending: 'bg-status-pending',
};

const VARIANT_TEXT_CLASS: Record<StatusVariant, string> = {
  success: 'text-status-success',
  error: 'text-status-error',
  running: 'text-status-running',
  warning: 'text-status-warning',
  pending: 'text-status-pending',
};

function getMeta(status: string): StatusMeta {
  return STATUS_META[status.toLowerCase()] ?? DEFAULT_META;
}

/** Returns the Tailwind bg class for the status dot indicator. */
export function getStatusDotClass(status: string): string {
  const meta = getMeta(status);
  return VARIANT_DOT_CLASS[meta.variant];
}

/** Returns the Tailwind text color class for the status label. */
export function getStatusTextClass(status: string): string {
  const meta = getMeta(status);
  return VARIANT_TEXT_CLASS[meta.variant];
}

/** Returns the icon type to render for the given status. */
export function getStatusIcon(status: string): StatusIconType {
  return getMeta(status).iconType;
}

/** Returns true if the status dot should pulse (e.g. active/running states). */
export function getStatusPulse(status: string): boolean {
  return getMeta(status).pulse;
}

/** Returns the color variant category for the given status. */
export function getStatusVariant(status: string): StatusVariant {
  return getMeta(status).variant;
}
