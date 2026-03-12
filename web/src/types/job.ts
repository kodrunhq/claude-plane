// Job system types -- matches server REST responses

export interface Job {
  id: string;
  name: string;
  description: string;
  created_at: string;
  updated_at: string;
  step_count?: number;
  last_run_status?: string;
}

export interface Step {
  id: string;
  job_id: string;
  name: string;
  prompt: string;
  machine_id: string;
  working_dir: string;
  command: string;
  args: string[];
  position_x?: number;
  position_y?: number;
  created_at: string;
  updated_at: string;
}

export interface StepDependency {
  id: string;
  step_id: string;
  depends_on_step_id: string;
}

export interface JobDetail {
  job: Job;
  steps: Step[];
  dependencies: StepDependency[];
}

export interface Run {
  id: string;
  job_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  started_at: string;
  finished_at?: string;
  created_at: string;
}

export interface RunStep {
  id: string;
  run_id: string;
  step_id: string;
  session_id?: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped';
  started_at?: string;
  finished_at?: string;
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
  args?: string[];
}

export interface UpdateStepParams {
  name?: string;
  prompt?: string;
  machine_id?: string;
  working_dir?: string;
  command?: string;
  args?: string[];
}
