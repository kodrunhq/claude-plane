import { useEffect, useState } from 'react';
import { formatDuration } from '../../lib/format.ts';

interface DurationDisplayProps {
  startedAt?: string;
  completedAt?: string;
  prefix?: string;
}

export function DurationDisplay({ startedAt, completedAt, prefix }: DurationDisplayProps) {
  const [elapsed, setElapsed] = useState<number | null>(null);

  useEffect(() => {
    if (!startedAt) {
      setElapsed(null);
      return;
    }

    const start = new Date(startedAt).getTime();

    if (completedAt) {
      setElapsed(Math.floor((new Date(completedAt).getTime() - start) / 1000));
      return;
    }

    const update = () => setElapsed(Math.floor((Date.now() - start) / 1000));
    update();
    const interval = setInterval(update, 1000);
    return () => clearInterval(interval);
  }, [startedAt, completedAt]);

  if (elapsed === null) {
    return <span>&mdash;</span>;
  }

  const formatted = formatDuration(elapsed);

  return (
    <span>
      {prefix ? `${prefix} ${formatted}` : formatted}
    </span>
  );
}
