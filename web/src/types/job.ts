// Job system types -- matches server REST responses

export interface Job {
  job_id: string;
  name: string;
  description: string;
  created_at: string;
  updated_at: string;
  step_count?: number;
  last_run_status?: string;
  trigger_type?: string;    // 'manual' | 'cron' | 'event' | 'mixed'
  machine_ids?: string;     // comma-separated machine IDs from steps
  parameters?: Record<string, string>;
  timeout_seconds?: number;
  max_concurrent_runs?: number;
}

export interface Task {
  step_id: string;
  job_id: string;
  name: string;
  prompt: string;
  machine_id: string;
  working_dir: string;
  command: string;
  args: string;
  timeout_seconds?: number;
  sort_order?: number;
  on_failure?: string;
  skip_permissions?: number | null;
  model?: string;
  delay_seconds?: number;
  task_type?: string;
  session_key?: string;
  run_if?: string;
  max_retries?: number;
  retry_delay_seconds?: number;
  parameters?: Record<string, string>;
  target_job_id?: string;
  job_params?: Record<string, string>;
}

export interface TaskDependency {
  step_id: string;
  depends_on: string;
}

export interface JobDetail {
  job: Job;
  steps: Task[];
  dependencies: TaskDependency[];
}

export interface Run {
  run_id: string;
  job_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  trigger_type?: string;
  trigger_detail?: string;
  job_name?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
  machine_ids?: string;     // comma-separated machine IDs from run steps
  parameters?: Record<string, string>;
}

export interface ListRunsParams {
  job_id?: string;
  status?: string;
  trigger_type?: string;
  limit?: number;
  offset?: number;
}

export interface RunTask {
  run_step_id: string;
  run_id: string;
  step_id: string;
  session_id?: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'cancelled';
  started_at?: string;
  completed_at?: string;
  error?: string;
  task_type_snapshot?: string;
  attempt?: number;
}

export interface RunDetail {
  run: Run;
  run_steps: RunTask[];
}

export interface CreateJobParams {
  name: string;
  description?: string;
  parameters?: Record<string, string>;
  timeout_seconds?: number;
  max_concurrent_runs?: number;
}

export interface CreateTaskParams {
  name: string;
  prompt?: string;
  machine_id: string;
  working_dir?: string;
  command?: string;
  args?: string;
  skip_permissions?: number | null;
  model?: string;
  delay_seconds?: number;
  task_type?: string;
  session_key?: string;
  run_if?: string;
  max_retries?: number;
  retry_delay_seconds?: number;
  on_failure?: string;
  target_job_id?: string;
  job_params?: Record<string, string>;
}

export interface UpdateTaskParams {
  name?: string;
  prompt?: string;
  machine_id?: string;
  working_dir?: string;
  command?: string;
  args?: string;
  skip_permissions?: number | null;
  model?: string;
  delay_seconds?: number;
  task_type?: string;
  session_key?: string;
  run_if?: string;
  max_retries?: number;
  retry_delay_seconds?: number;
  on_failure?: string;
  target_job_id?: string;
  job_params?: Record<string, string>;
}

export interface TriggerRunParams {
  trigger_type?: string;
  trigger_detail?: string;
  parameters?: Record<string, string>;
}
