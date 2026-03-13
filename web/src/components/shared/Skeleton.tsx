interface SkeletonProps {
  width?: string;
  height?: string;
  rounded?: 'sm' | 'md' | 'lg' | 'full';
  className?: string;
}

const roundedClasses: Record<NonNullable<SkeletonProps['rounded']>, string> = {
  sm: 'rounded-sm',
  md: 'rounded',
  lg: 'rounded-lg',
  full: 'rounded-full',
};

export function Skeleton({ width, height, rounded = 'md', className = '' }: SkeletonProps) {
  return (
    <div
      className={`bg-bg-tertiary animate-pulse ${roundedClasses[rounded]} ${className}`}
      style={{ width, height }}
    />
  );
}

interface SkeletonTextProps {
  lines?: number;
}

const LINE_WIDTHS = ['w-full', 'w-4/5', 'w-3/4', 'w-2/3', 'w-1/2', 'w-3/5'];

export function SkeletonText({ lines = 3 }: SkeletonTextProps) {
  return (
    <div className="space-y-2">
      {Array.from({ length: lines }, (_, i) => (
        <div
          key={i}
          className={`h-3 bg-bg-tertiary animate-pulse rounded ${LINE_WIDTHS[i % LINE_WIDTHS.length]}`}
        />
      ))}
    </div>
  );
}
