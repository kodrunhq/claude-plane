import { useRef, useCallback, useEffect, useState, useMemo } from 'react';
import { ChevronDown } from 'lucide-react';
import type { Task, UpdateTaskParams, Job } from '../../types/job.ts';
import type { Machine } from '../../lib/types.ts';
import type { SessionTemplate } from '../../types/template.ts';
import { useTemplates } from '../../hooks/useTemplates.ts';
import { useJobs } from '../../hooks/useJobs.ts';

interface TaskEditorProps {
  task: Task | null;
  machines: Machine[];
  onSave: (taskId: string, params: UpdateTaskParams) => void;
  onDelete: (taskId: string) => void;
  onDirtyChange?: (dirty: boolean) => void;
}

type TaskType = 'claude' | 'shell' | 'run_job';

function parseSkipPermissions(value: string): number | null | undefined {
  if (value === '1') return 1;
  if (value === '0') return 0;
  return undefined;
}

function collectJobParams(data: FormData): string | undefined {
  const jobParams: Record<string, string> = {};
  for (const [key, value] of data.entries()) {
    if (key.startsWith('job_param_') && typeof value === 'string' && value !== '') {
      jobParams[key.replace('job_param_', '')] = value;
    }
  }
  return Object.keys(jobParams).length > 0 ? JSON.stringify(jobParams) : undefined;
}

function getFormParams(form: HTMLFormElement, taskType: TaskType): UpdateTaskParams {
  const data = new FormData(form);
  const base: UpdateTaskParams = {
    name: data.get('name') as string,
    machine_id: taskType === 'run_job' ? '' : (data.get('machine_id') as string),
    working_dir: taskType === 'run_job' ? '' : (data.get('working_dir') as string),
    task_type: taskType === 'claude' ? 'claude_session' : taskType,
    delay_seconds: Number(data.get('delay_seconds')) || 0,
    run_if: (data.get('run_if') as string) || undefined,
    max_retries: Number(data.get('max_retries')) || 0,
    retry_delay_seconds: Number(data.get('retry_delay_seconds')) || 0,
    on_failure: (data.get('on_failure') as string) || undefined,
  };

  if (taskType === 'claude') {
    return {
      ...base,
      prompt: data.get('prompt') as string,
      command: (data.get('command') as string) || 'claude',
      args: data.get('args') as string,
      model: (data.get('model') as string) || undefined,
      skip_permissions: parseSkipPermissions(data.get('skip_permissions') as string),
      session_key: (data.get('session_key') as string) || undefined,
    };
  }

  if (taskType === 'run_job') {
    return {
      ...base,
      prompt: '',
      command: '',
      args: '',
      model: undefined,
      skip_permissions: undefined,
      session_key: undefined,
      target_job_id: (data.get('target_job_id') as string) || undefined,
      job_params: collectJobParams(data),
    };
  }

  // Shell task
  return {
    ...base,
    prompt: '',
    command: data.get('command') as string,
    args: data.get('args') as string,
    model: undefined,
    skip_permissions: undefined,
    session_key: undefined,
  };
}

function skipPermissionsFormValue(task: Task): string {
  if (task.skip_permissions === 1) return '1';
  if (task.skip_permissions === 0) return '0';
  return '';
}

function resolveTaskType(task: Task): TaskType {
  if (task.task_type === 'shell') return 'shell';
  if (task.task_type === 'run_job') return 'run_job';
  return 'claude';
}

function parseJobParameters(job: Job | undefined): Record<string, string> | null {
  if (!job?.parameters) return null;
  if (typeof job.parameters === 'string') {
    try {
      return JSON.parse(job.parameters as string) as Record<string, string>;
    } catch {
      return null;
    }
  }
  return job.parameters;
}

function isDirty(form: HTMLFormElement, task: Task, taskType: TaskType): boolean {
  const params = getFormParams(form, taskType);
  const data = new FormData(form);
  const base =
    params.name !== task.name ||
    (taskType !== 'run_job' && params.machine_id !== task.machine_id) ||
    (taskType !== 'run_job' && params.working_dir !== task.working_dir) ||
    (Number(data.get('delay_seconds')) || 0) !== (task.delay_seconds ?? 0) ||
    taskType !== resolveTaskType(task) ||
    (data.get('run_if') as string || '') !== (task.run_if ?? '') ||
    (Number(data.get('max_retries')) || 0) !== (task.max_retries ?? 0) ||
    (Number(data.get('retry_delay_seconds')) || 0) !== (task.retry_delay_seconds ?? 0) ||
    (data.get('on_failure') as string || '') !== (task.on_failure ?? '');

  if (base) return true;

  if (taskType === 'claude') {
    return (
      params.prompt !== task.prompt ||
      params.command !== (task.command || 'claude') ||
      params.args !== (task.args ?? '') ||
      (data.get('model') as string) !== (task.model ?? '') ||
      (data.get('skip_permissions') as string) !== skipPermissionsFormValue(task) ||
      (data.get('session_key') as string || '') !== (task.session_key ?? '')
    );
  }

  if (taskType === 'run_job') {
    return (
      (data.get('target_job_id') as string || '') !== (task.target_job_id ?? '') ||
      collectJobParams(data) !== (task.job_params || undefined)
    );
  }

  // Shell
  return (
    (data.get('command') as string) !== (task.command || '') ||
    (data.get('args') as string) !== (task.args ?? '')
  );
}

function TemplatePreview({ template }: { template: SessionTemplate }) {
  return (
    <div className="border-t border-border-primary bg-bg-secondary p-3 text-xs space-y-1.5">
      {template.description && (
        <p className="text-text-secondary">{template.description}</p>
      )}
      <div className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
        {template.command && (
          <>
            <span className="text-text-secondary/70">Command</span>
            <span className="font-mono text-text-primary truncate">{template.command}</span>
          </>
        )}
        {template.args && template.args.length > 0 && (
          <>
            <span className="text-text-secondary/70">Args</span>
            <span className="font-mono text-text-primary truncate">{template.args.join(' ')}</span>
          </>
        )}
        {template.working_dir && (
          <>
            <span className="text-text-secondary/70">Work Dir</span>
            <span className="font-mono text-text-primary truncate">{template.working_dir}</span>
          </>
        )}
        {template.initial_prompt && (
          <>
            <span className="text-text-secondary/70">Prompt</span>
            <span className="text-text-primary line-clamp-2">{template.initial_prompt}</span>
          </>
        )}
        {template.env_vars && Object.keys(template.env_vars).length > 0 && (
          <>
            <span className="text-text-secondary/70">Env Vars</span>
            <span className="text-text-primary">{Object.keys(template.env_vars).length} defined</span>
          </>
        )}
        {template.timeout_seconds > 0 && (
          <>
            <span className="text-text-secondary/70">Timeout</span>
            <span className="text-text-primary">{template.timeout_seconds}s</span>
          </>
        )}
      </div>
      {template.tags && template.tags.length > 0 && (
        <div className="flex gap-1 flex-wrap pt-1">
          {template.tags.map((tag) => (
            <span key={tag} className="bg-bg-tertiary text-text-secondary rounded-full px-1.5 py-0.5 text-[10px]">
              {tag}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

interface TemplateSelectorProps {
  templates: SessionTemplate[];
  selectedId: string;
  taskId: string;
  onSelect: (template: SessionTemplate) => void;
}

function TemplateSelector({ templates, selectedId, taskId, onSelect }: TemplateSelectorProps) {
  const [open, setOpen] = useState(false);
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const selectedTemplate = selectedId
    ? templates.find((t) => t.template_id === selectedId) ?? null
    : null;

  const hoveredTemplate = hoveredId
    ? templates.find((t) => t.template_id === hoveredId) ?? null
    : null;

  // Close on outside click
  useEffect(() => {
    if (!open) return;
    function handleClick(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
        setHoveredId(null);
      }
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [open]);

  // Reset when task changes
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- resetting dropdown state when task changes
    setOpen(false);
    setHoveredId(null);
  }, [taskId]);

  return (
    <div ref={containerRef} className="relative">
      <label className="block text-xs text-text-secondary mb-1">Use Template</label>
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        className="w-full flex items-center justify-between px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
      >
        <span className={selectedTemplate ? 'text-text-primary' : 'text-text-secondary'}>
          {selectedTemplate ? selectedTemplate.name : 'Apply template...'}
        </span>
        <ChevronDown size={14} className={`text-text-secondary transition-transform ${open ? 'rotate-180' : ''}`} />
      </button>

      {open && (
        <div className="absolute left-0 right-0 top-full mt-1 z-40 bg-bg-primary border border-border-primary rounded-lg shadow-lg overflow-hidden">
          <div className="max-h-48 overflow-y-auto">
            <div
              className="px-3 py-1.5 text-sm text-text-secondary hover:bg-bg-tertiary cursor-pointer"
              onMouseDown={() => {
                onSelect({ template_id: '' } as SessionTemplate);
                setOpen(false);
                setHoveredId(null);
              }}
              onMouseEnter={() => setHoveredId(null)}
            >
              Apply template...
            </div>
            {templates.map((t) => (
              <div
                key={t.template_id}
                className={`px-3 py-1.5 text-sm cursor-pointer transition-colors ${
                  t.template_id === selectedId
                    ? 'bg-accent-primary/10 text-accent-primary'
                    : 'text-text-primary hover:bg-bg-tertiary'
                }`}
                onMouseDown={() => {
                  onSelect(t);
                  setOpen(false);
                  setHoveredId(null);
                }}
                onMouseEnter={() => setHoveredId(t.template_id)}
                onMouseLeave={() => setHoveredId(null)}
              >
                <div className="font-medium truncate">{t.name}</div>
                {t.command && (
                  <div className="text-xs text-text-secondary/70 font-mono truncate">{t.command}{t.args?.length ? ' ' + t.args.join(' ') : ''}</div>
                )}
              </div>
            ))}
          </div>
          {hoveredTemplate && <TemplatePreview template={hoveredTemplate} />}
        </div>
      )}
    </div>
  );
}

const TASK_TYPE_LABELS: Record<TaskType, string> = {
  claude: 'Claude Session',
  shell: 'Shell',
  run_job: 'Run Job',
};

export function TaskEditor({ task, machines, onSave, onDelete, onDirtyChange }: TaskEditorProps) {
  const formRef = useRef<HTMLFormElement>(null);
  const lastDirty = useRef(false);
  const { data: templates } = useTemplates();
  const { data: jobs } = useJobs();
  const [selectedTemplateId, setSelectedTemplateId] = useState('');
  const [taskType, setTaskType] = useState<TaskType>(() => task ? resolveTaskType(task) : 'claude');
  const [maxRetriesState, setMaxRetriesState] = useState(task?.max_retries ?? 0);
  const [targetJobId, setTargetJobId] = useState(task?.target_job_id ?? '');

  const currentJobId = task?.job_id;

  // Filter out the current job from the available targets to prevent self-referencing.
  const availableJobs = useMemo(
    () => jobs?.filter((j) => j.job_id !== currentJobId) ?? [],
    [jobs, currentJobId],
  );

  const selectedTargetJob = useMemo(
    () => availableJobs.find((j) => j.job_id === targetJobId),
    [availableJobs, targetJobId],
  );

  const targetJobParams = useMemo(
    () => parseJobParameters(selectedTargetJob),
    [selectedTargetJob],
  );

  // Existing job_params values from the task (for default values in param inputs).
  const existingJobParams = useMemo(() => {
    if (!task?.job_params) return {};
    try {
      return JSON.parse(task.job_params) as Record<string, string>;
    } catch {
      return {};
    }
  }, [task]);

  // Sync task type, max retries, and target job when selected task changes.
  useEffect(() => {
    if (task) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing server data to local form state on task selection change
      setTaskType(resolveTaskType(task));
      setMaxRetriesState(task.max_retries ?? 0);
      setTargetJobId(task.target_job_id ?? '');
    }
  }, [task]); // full task object -- re-syncs on any server update

  const checkDirty = useCallback(() => {
    if (!formRef.current || !task || !onDirtyChange) return;
    const dirty = isDirty(formRef.current, task, taskType);
    if (dirty !== lastDirty.current) {
      lastDirty.current = dirty;
      onDirtyChange(dirty);
    }
  }, [task, onDirtyChange, taskType]);

  // Reset dirty state and template selection when task changes.
  useEffect(() => {
    lastDirty.current = false;
    onDirtyChange?.(false);
    // eslint-disable-next-line react-hooks/set-state-in-effect -- resetting local form state when task changes
    setSelectedTemplateId('');
  }, [task?.step_id, onDirtyChange]);

  const applyTemplate = useCallback((template: SessionTemplate) => {
    const form = formRef.current;
    if (!form) return;

    const setField = (name: string, value: string) => {
      const el = form.elements.namedItem(name) as HTMLInputElement | HTMLTextAreaElement | null;
      if (el) {
        const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
          Object.getPrototypeOf(el), 'value',
        )?.set;
        nativeInputValueSetter?.call(el, value);
        el.dispatchEvent(new Event('input', { bubbles: true }));
      }
    };

    if (template.command) setField('command', template.command);
    if (template.args?.length) setField('args', template.args.join('\n'));
    if (template.working_dir) setField('working_dir', template.working_dir);
    if (template.initial_prompt) setField('prompt', template.initial_prompt);

    lastDirty.current = true;
    onDirtyChange?.(true);
  }, [onDirtyChange]);

  if (!task) {
    return (
      <div className="flex items-center justify-center h-full text-text-secondary text-sm">
        Select a task to edit its configuration
      </div>
    );
  }

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!task) return;
    const params = getFormParams(e.currentTarget, taskType);
    onSave(task.step_id, params);
    lastDirty.current = false;
    onDirtyChange?.(false);
  }

  function handleTemplateSelect(template: SessionTemplate) {
    if (!template.template_id) {
      setSelectedTemplateId('');
      return;
    }
    setSelectedTemplateId(template.template_id);
    applyTemplate(template);
  }

  function handleTaskTypeChange(type: TaskType) {
    setTaskType(type);
    lastDirty.current = true;
    onDirtyChange?.(true);
  }

  function handleTargetJobChange(e: React.ChangeEvent<HTMLSelectElement>) {
    setTargetJobId(e.target.value);
    lastDirty.current = true;
    onDirtyChange?.(true);
  }

  return (
    <form
      ref={formRef}
      onSubmit={handleSubmit}
      onChange={checkDirty}
      className="p-4 space-y-3 overflow-y-auto h-full"
    >
      <h3 className="text-sm font-medium text-text-primary">Task Configuration</h3>

      {/* Task Type Toggle */}
      <div>
        <label className="block text-xs text-text-secondary mb-1">Task Type</label>
        <div className="flex rounded-md overflow-hidden border border-border-primary">
          {(['claude', 'shell', 'run_job'] as const).map((type) => (
            <button
              key={type}
              type="button"
              onClick={() => handleTaskTypeChange(type)}
              className={`flex-1 py-1.5 text-xs font-medium transition-colors ${
                taskType === type
                  ? 'bg-accent-primary text-white'
                  : 'bg-bg-tertiary text-text-secondary hover:text-text-primary'
              }`}
            >
              {TASK_TYPE_LABELS[type]}
            </button>
          ))}
        </div>
      </div>

      {/* Template selector -- only for Claude sessions */}
      {taskType === 'claude' && templates && templates.length > 0 && (
        <TemplateSelector
          templates={templates}
          selectedId={selectedTemplateId}
          taskId={task.step_id}
          onSelect={handleTemplateSelect}
        />
      )}

      <div>
        <label htmlFor="task-name" className="block text-xs text-text-secondary mb-1">Name</label>
        <input
          id="task-name"
          name="name"
          type="text"
          defaultValue={task.name}
          key={task.step_id + '-name'}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
        />
      </div>

      {/* Prompt -- Claude only */}
      {taskType === 'claude' && (
        <div>
          <label htmlFor="task-prompt" className="block text-xs text-text-secondary mb-1">Prompt</label>
          <textarea
            id="task-prompt"
            name="prompt"
            rows={4}
            defaultValue={task.prompt}
            key={task.step_id + '-prompt'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary resize-none font-mono"
            placeholder="Enter the prompt for Claude..."
          />
        </div>
      )}

      {/* Target Job -- run_job only */}
      {taskType === 'run_job' && (
        <>
          <div>
            <label htmlFor="task-target-job" className="block text-xs text-text-secondary mb-1">
              Target Job <span className="text-red-400">*</span>
            </label>
            <select
              id="task-target-job"
              name="target_job_id"
              value={targetJobId}
              onChange={handleTargetJobChange}
              required
              className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
            >
              <option value="">Select a job...</option>
              {availableJobs.map((j) => (
                <option key={j.job_id} value={j.job_id}>{j.name}</option>
              ))}
            </select>
          </div>

          {/* Dynamic parameter fields based on selected job's parameters */}
          {targetJobParams && Object.entries(targetJobParams).map(([key, defaultVal]) => (
            <div key={key}>
              <label htmlFor={`task-job-param-${key}`} className="block text-xs text-text-secondary mb-1">
                {key}
              </label>
              <input
                id={`task-job-param-${key}`}
                name={`job_param_${key}`}
                type="text"
                defaultValue={existingJobParams[key] ?? ''}
                key={task.step_id + '-job-param-' + key + '-' + targetJobId}
                placeholder={defaultVal || `Value for ${key}`}
                className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary font-mono"
              />
            </div>
          ))}
        </>
      )}

      {/* Machine -- claude and shell only */}
      {taskType !== 'run_job' && (
        <div>
          <label htmlFor="task-machine" className="block text-xs text-text-secondary mb-1">Machine</label>
          <select
            id="task-machine"
            name="machine_id"
            defaultValue={task.machine_id}
            key={task.step_id + '-machine'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
          >
            <option value="">Select machine...</option>
            {machines.map((m) => (
              <option key={m.machine_id} value={m.machine_id}>
                {m.display_name || m.machine_id.slice(0, 8)}
              </option>
            ))}
          </select>
        </div>
      )}

      {/* Model -- Claude only */}
      {taskType === 'claude' && (
        <div>
          <label htmlFor="task-model" className="block text-xs text-text-secondary mb-1">Model</label>
          <select
            id="task-model"
            name="model"
            defaultValue={task.model ?? ''}
            key={task.step_id + '-model'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
          >
            <option value="">Default</option>
            <option value="opus">Opus</option>
            <option value="sonnet">Sonnet</option>
            <option value="haiku">Haiku</option>
          </select>
        </div>
      )}

      {/* Skip Permissions -- Claude only */}
      {taskType === 'claude' && (
        <div>
          <label htmlFor="task-skip-permissions" className="block text-xs text-text-secondary mb-1">Skip Permissions</label>
          <select
            id="task-skip-permissions"
            name="skip_permissions"
            defaultValue={skipPermissionsFormValue(task)}
            key={task.step_id + '-skip-permissions'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
          >
            <option value="">Default (from settings)</option>
            <option value="1">On</option>
            <option value="0">Off</option>
          </select>
        </div>
      )}

      {/* Session Key -- Claude only */}
      {taskType === 'claude' && (
        <div>
          <label htmlFor="task-session-key" className="block text-xs text-text-secondary mb-1">Session Key</label>
          <input
            id="task-session-key"
            name="session_key"
            type="text"
            defaultValue={task.session_key ?? ''}
            key={task.step_id + '-session-key'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary font-mono"
            placeholder="Optional -- tasks sharing a key reuse the same session"
          />
        </div>
      )}

      <div>
        <label htmlFor="task-delay" className="block text-xs text-text-secondary mb-1">Delay (seconds)</label>
        <input
          id="task-delay"
          name="delay_seconds"
          type="number"
          min={0}
          max={86400}
          defaultValue={task.delay_seconds ?? 0}
          key={task.step_id + '-delay'}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
        />
        <p className="text-[10px] text-text-secondary/70 mt-0.5">Wait before starting this task (0-86400)</p>
      </div>

      {/* Working Directory -- claude and shell only */}
      {taskType !== 'run_job' && (
        <div>
          <label htmlFor="task-workdir" className="block text-xs text-text-secondary mb-1">Working Directory</label>
          <input
            id="task-workdir"
            name="working_dir"
            type="text"
            defaultValue={task.working_dir}
            key={task.step_id + '-workdir'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary font-mono"
            placeholder="/home/user/project"
          />
        </div>
      )}

      {/* Command -- claude and shell only */}
      {taskType !== 'run_job' && (
        <div>
          <label htmlFor="task-command" className="block text-xs text-text-secondary mb-1">
            Command{taskType === 'shell' ? ' (required)' : ''}
          </label>
          <input
            id="task-command"
            name="command"
            type="text"
            defaultValue={taskType === 'claude' ? (task.command || 'claude') : (task.command || '')}
            key={task.step_id + '-command-' + taskType}
            required={taskType === 'shell'}
            placeholder={taskType === 'shell' ? 'e.g., ./deploy.sh, python script.py' : undefined}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary font-mono"
          />
          {taskType === 'claude' && (
            <p className="text-[10px] text-text-secondary/70 mt-0.5">(defaults to claude)</p>
          )}
        </div>
      )}

      {/* Args -- claude and shell only */}
      {taskType !== 'run_job' && (
        <div>
          <label htmlFor="task-args" className="block text-xs text-text-secondary mb-1">Args (one per line)</label>
          <textarea
            id="task-args"
            name="args"
            rows={2}
            defaultValue={task.args ?? ''}
            key={task.step_id + '-args'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary resize-none font-mono"
          />
        </div>
      )}

      {/* Run If */}
      <div>
        <label htmlFor="task-run-if" className="block text-xs text-text-secondary mb-1">Run If</label>
        <select
          id="task-run-if"
          name="run_if"
          defaultValue={task.run_if ?? ''}
          key={task.step_id + '-run-if'}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
        >
          <option value="">Default (all success)</option>
          <option value="all_success">All dependencies succeeded</option>
          <option value="all_done">All dependencies completed (any status)</option>
        </select>
      </div>

      {/* Max Retries */}
      <div>
        <label htmlFor="task-max-retries" className="block text-xs text-text-secondary mb-1">Max Retries</label>
        <input
          id="task-max-retries"
          name="max_retries"
          type="number"
          min={0}
          max={5}
          defaultValue={task.max_retries ?? 0}
          key={task.step_id + '-max-retries'}
          onChange={(e) => {
            setMaxRetriesState(Number(e.target.value) || 0);
            checkDirty();
          }}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
        />
      </div>

      {/* Retry Delay -- shown if max_retries > 0 */}
      {maxRetriesState > 0 && (
        <div>
          <label htmlFor="task-retry-delay" className="block text-xs text-text-secondary mb-1">Retry Delay (seconds)</label>
          <input
            id="task-retry-delay"
            name="retry_delay_seconds"
            type="number"
            min={0}
            max={3600}
            defaultValue={task.retry_delay_seconds ?? 0}
            key={task.step_id + '-retry-delay'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
          />
        </div>
      )}

      {/* On Failure */}
      <div>
        <label htmlFor="task-on-failure" className="block text-xs text-text-secondary mb-1">On Failure</label>
        <select
          id="task-on-failure"
          name="on_failure"
          defaultValue={task.on_failure ?? ''}
          key={task.step_id + '-on-failure'}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
        >
          <option value="">Fail Run (default)</option>
          <option value="fail_run">Fail Run</option>
          <option value="continue">Continue</option>
        </select>
      </div>

      <div className="flex gap-2 pt-2">
        <button
          type="submit"
          className="flex-1 px-3 py-1.5 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors"
        >
          Save Task
        </button>
        <button
          type="button"
          onClick={() => onDelete(task.step_id)}
          className="px-3 py-1.5 text-sm rounded-md bg-red-600/20 text-red-400 hover:bg-red-600/30 transition-colors"
        >
          Delete
        </button>
      </div>
    </form>
  );
}
