interface StatusBadgeProps {
  status: string;
  size?: 'sm' | 'md';
}

const statusColors: Record<string, string> = {
  online: 'bg-status-success',
  running: 'bg-status-success',
  offline: 'bg-status-error',
  terminated: 'bg-status-error',
  failed: 'bg-status-error',
  completed: 'bg-status-running',
  created: 'bg-status-pending',
  pending: 'bg-status-warning',
};

export function StatusBadge({ status, size = 'md' }: StatusBadgeProps) {
  const dotColor = statusColors[status] ?? 'bg-status-pending';
  const dotSize = size === 'sm' ? 'w-1.5 h-1.5' : 'w-2 h-2';
  const textSize = size === 'sm' ? 'text-xs' : 'text-sm';

  return (
    <span className={`inline-flex items-center gap-1.5 ${textSize}`}>
      <span className={`${dotSize} rounded-full ${dotColor}`} />
      <span className="capitalize text-text-secondary">{status}</span>
    </span>
  );
}
