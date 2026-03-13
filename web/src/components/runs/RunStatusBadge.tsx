const STATUS_COLORS: Record<string, string> = {
  pending: 'bg-gray-500/20 text-gray-400',
  running: 'bg-blue-500/20 text-blue-400',
  completed: 'bg-green-500/20 text-green-400',
  failed: 'bg-red-500/20 text-red-400',
  cancelled: 'bg-yellow-500/20 text-yellow-400',
};

const SIZE_CLASSES: Record<string, string> = {
  sm: 'px-1.5 py-0.5 text-[10px]',
  md: 'px-2 py-0.5 text-xs',
};

interface RunStatusBadgeProps {
  status: string;
  size?: 'sm' | 'md';
}

export function RunStatusBadge({ status, size = 'md' }: RunStatusBadgeProps) {
  const colorClass = STATUS_COLORS[status] ?? STATUS_COLORS.pending;
  const sizeClass = SIZE_CLASSES[size];

  return (
    <span className={`rounded-full ${sizeClass} ${colorClass}`}>
      {status}
    </span>
  );
}
