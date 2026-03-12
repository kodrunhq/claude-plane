import { useState, useEffect } from 'react';
import { formatTimeAgo } from '../../lib/format.ts';

interface TimeAgoProps {
  date: string;
  className?: string;
}

export function TimeAgo({ date, className }: TimeAgoProps) {
  const [text, setText] = useState(() => formatTimeAgo(date));

  useEffect(() => {
    setText(formatTimeAgo(date));

    const interval = setInterval(() => {
      setText(formatTimeAgo(date));
    }, 60_000);

    return () => clearInterval(interval);
  }, [date]);

  return (
    <time dateTime={date} className={className}>
      {text}
    </time>
  );
}
