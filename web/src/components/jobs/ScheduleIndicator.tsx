import { useMemo } from 'react';
import { Clock } from 'lucide-react';
import { useSchedules } from '../../hooks/useSchedules.ts';

interface ScheduleIndicatorProps {
  jobId: string;
}

export function ScheduleIndicator({ jobId }: ScheduleIndicatorProps) {
  const { data: schedules } = useSchedules(jobId);

  const enabledSchedule = useMemo(() => {
    const enabled = schedules?.filter((s) => s.enabled) ?? [];
    if (enabled.length === 0) return null;
    // Pick the schedule with the earliest non-null next_run_at.
    const withNext = enabled.filter((s) => s.next_run_at);
    if (withNext.length === 0) return enabled[0];
    return withNext.reduce((a, b) =>
      new Date(a.next_run_at!).getTime() < new Date(b.next_run_at!).getTime() ? a : b,
    );
  }, [schedules]);

  if (!enabledSchedule) return null;

  const nextRunLabel = enabledSchedule.next_run_at
    ? new Date(enabledSchedule.next_run_at).toLocaleString(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
    : 'Scheduled';

  return (
    <span className="flex items-center gap-1 text-blue-400">
      <Clock size={11} />
      {nextRunLabel}
    </span>
  );
}
