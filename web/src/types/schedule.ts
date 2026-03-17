export interface CronSchedule {
  schedule_id: string;
  job_id: string;
  cron_expr: string;
  timezone: string;
  enabled: boolean;
  next_run_at: string | null;
  last_triggered_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface CronScheduleWithJob extends CronSchedule {
  job_name: string;
}

export interface CreateScheduleParams {
  cron_expr: string;
  timezone?: string;
}

export interface UpdateScheduleParams {
  cron_expr?: string;
  timezone?: string;
}
