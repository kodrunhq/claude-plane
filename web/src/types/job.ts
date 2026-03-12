// Job system types -- matches server REST responses

export interface Job {
  job_id: string;
  name: string;
  description: string;
  created_at: string;
  updated_at: string;
  step_count?: number;
  last_run_status?: string;
}

export interface Step {
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
}

export interface StepDependency {
  step_id: string;
  depends_on: string;
}

export interface JobDetail {
  job: Job;
  steps: Step[];
  dependencies: StepDependency[];
}

export interface Run {
  run_id: string;
  job_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  started_at?: string;
  completed_at?: string;
  created_at: string;
}

export interface RunStep {
  run_step_id: string;
  run_id: string;
  step_id: string;
  session_id?: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'cancelled';
  started_at?: string;
  completed_at?: string;
  error?: string;
}

export interface RunDetail {
  run: Run;
  run_steps: RunStep[];
}

export interface CreateJobParams {
  name: string;
  description?: string;
}

export interface CreateStepParams {
  name: string;
  prompt?: string;
  machine_id: string;
  working_dir?: string;
  command?: string;
  args?: string;
}

export interface UpdateStepParams {
  name?: string;
  prompt?: string;
  machine_id?: string;
  working_dir?: string;
  command?: string;
  args?: string;
}
