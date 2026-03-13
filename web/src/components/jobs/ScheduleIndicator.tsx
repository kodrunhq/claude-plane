import { useMemo } from 'react';
import { Clock } from 'lucide-react';
import { useSchedules } from '../../hooks/useSchedules.ts';

interface ScheduleIndicatorProps {
  jobId: string;
}

export function ScheduleIndicator({ jobId }: ScheduleIndicatorProps) {
  const { data: schedules } = useSchedules(jobId);

  const enabledSchedule = useMemo(
    () => schedules?.find((s) => s.enabled) ?? null,
    [schedules],
  );

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
