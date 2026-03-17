import { useState, useEffect, useMemo } from 'react';
import { formatTimeAgo } from '../../lib/format.ts';

interface TimeAgoProps {
  date: string;
  className?: string;
}

export function TimeAgo({ date, className }: TimeAgoProps) {
  const [text, setText] = useState(() => formatTimeAgo(date));

  const absoluteDate = useMemo(() => {
    const d = new Date(date);
    if (isNaN(d.getTime())) return '';
    return d.toLocaleString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  }, [date]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing with external time source
    setText(formatTimeAgo(date));

    const interval = setInterval(() => {
      setText(formatTimeAgo(date));
    }, 60_000);

    return () => clearInterval(interval);
  }, [date]);

  return (
    <time dateTime={date} title={absoluteDate} className={className}>
      {text}
    </time>
  );
}
