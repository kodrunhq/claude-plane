import { CheckCircle2, Clock, Loader2, XCircle } from 'lucide-react';
import {
  getStatusDotClass,
  getStatusIcon,
  getStatusPulse,
  getStatusTextClass,
  type StatusIconType,
} from '../../lib/statusColors';

export interface StatusBadgeProps {
  status: string;
  size?: 'sm' | 'md' | 'lg';
}

const SIZE_DOT: Record<NonNullable<StatusBadgeProps['size']>, string> = {
  sm: 'w-1.5 h-1.5',
  md: 'w-2 h-2',
  lg: 'w-2.5 h-2.5',
};

const SIZE_TEXT: Record<NonNullable<StatusBadgeProps['size']>, string> = {
  sm: 'text-xs',
  md: 'text-sm',
  lg: 'text-base',
};

const SIZE_ICON: Record<NonNullable<StatusBadgeProps['size']>, number> = {
  sm: 12,
  md: 14,
  lg: 16,
};

function StatusIcon({
  iconType,
  size,
  dotColorClass,
  textColorClass,
}: {
  iconType: StatusIconType;
  size: NonNullable<StatusBadgeProps['size']>;
  dotColorClass: string;
  textColorClass: string;
}) {
  const px = SIZE_ICON[size];

  if (iconType === 'check') {
    return <CheckCircle2 size={px} className={`${textColorClass} shrink-0`} />;
  }

  if (iconType === 'x') {
    return <XCircle size={px} className={`${textColorClass} shrink-0`} />;
  }

  if (iconType === 'spinner') {
    return <Loader2 size={px} className={`${textColorClass} animate-spin shrink-0`} />;
  }

  if (iconType === 'clock') {
    return <Clock size={px} className={`${textColorClass} shrink-0`} />;
  }

  // Fallback: plain dot indicator
  return <span className={`${SIZE_DOT[size]} rounded-full ${dotColorClass} shrink-0`} />;
}

export function StatusBadge({ status, size = 'md' }: StatusBadgeProps) {
  const dotColorClass = getStatusDotClass(status);
  const textColorClass = getStatusTextClass(status);
  const iconType = getStatusIcon(status);
  const pulse = getStatusPulse(status);
  const textSize = SIZE_TEXT[size];

  const showDot = iconType === 'none';

  return (
    <span className={`inline-flex items-center gap-1.5 ${textSize}`}>
      {showDot ? (
        <span
          className={`${SIZE_DOT[size]} rounded-full ${dotColorClass} shrink-0 ${pulse ? 'animate-pulse' : ''}`}
        />
      ) : (
        <StatusIcon iconType={iconType} size={size} dotColorClass={dotColorClass} textColorClass={textColorClass} />
      )}
      <span className="capitalize text-text-secondary">{status}</span>
    </span>
  );
}
