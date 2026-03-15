import { useRef, useCallback, useEffect, useState } from 'react';
import { ChevronDown } from 'lucide-react';
import type { Step, UpdateStepParams } from '../../types/job.ts';
import type { Machine } from '../../lib/types.ts';
import type { SessionTemplate } from '../../types/template.ts';
import { useTemplates } from '../../hooks/useTemplates.ts';

interface StepEditorProps {
  step: Step | null;
  machines: Machine[];
  onSave: (stepId: string, params: UpdateStepParams) => void;
  onDelete: (stepId: string) => void;
  onDirtyChange?: (dirty: boolean) => void;
}

type TaskType = 'claude' | 'shell';

function parseSkipPermissions(value: string): number | null | undefined {
  if (value === '1') return 1;
  if (value === '0') return 0;
  return undefined;
}

function getFormParams(form: HTMLFormElement, taskType: TaskType): UpdateStepParams {
  const data = new FormData(form);
  const base: UpdateStepParams = {
    name: data.get('name') as string,
    machine_id: data.get('machine_id') as string,
    working_dir: data.get('working_dir') as string,
    task_type: taskType,
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

function skipPermissionsFormValue(step: Step): string {
  if (step.skip_permissions === 1) return '1';
  if (step.skip_permissions === 0) return '0';
  return '';
}

function resolveTaskType(step: Step): TaskType {
  if (step.task_type === 'shell') return 'shell';
  return 'claude';
}

function isDirty(form: HTMLFormElement, step: Step, taskType: TaskType): boolean {
  const params = getFormParams(form, taskType);
  const data = new FormData(form);
  const base =
    params.name !== step.name ||
    params.machine_id !== step.machine_id ||
    params.working_dir !== step.working_dir ||
    (Number(data.get('delay_seconds')) || 0) !== (step.delay_seconds ?? 0) ||
    taskType !== resolveTaskType(step) ||
    (data.get('run_if') as string || '') !== (step.run_if ?? '') ||
    (Number(data.get('max_retries')) || 0) !== (step.max_retries ?? 0) ||
    (Number(data.get('retry_delay_seconds')) || 0) !== (step.retry_delay_seconds ?? 0) ||
    (data.get('on_failure') as string || '') !== (step.on_failure ?? '');

  if (base) return true;

  if (taskType === 'claude') {
    return (
      params.prompt !== step.prompt ||
      params.command !== (step.command || 'claude') ||
      params.args !== (step.args ?? '') ||
      (data.get('model') as string) !== (step.model ?? '') ||
      (data.get('skip_permissions') as string) !== skipPermissionsFormValue(step) ||
      (data.get('session_key') as string || '') !== (step.session_key ?? '')
    );
  }

  // Shell
  return (
    (data.get('command') as string) !== (step.command || '') ||
    (data.get('args') as string) !== (step.args ?? '')
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
  stepId: string;
  onSelect: (template: SessionTemplate) => void;
}

function TemplateSelector({ templates, selectedId, stepId, onSelect }: TemplateSelectorProps) {
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

  // Reset when step changes
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- resetting dropdown state when step changes
    setOpen(false);
    setHoveredId(null);
  }, [stepId]);

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

export function StepEditor({ step, machines, onSave, onDelete, onDirtyChange }: StepEditorProps) {
  const formRef = useRef<HTMLFormElement>(null);
  const lastDirty = useRef(false);
  const { data: templates } = useTemplates();
  const [selectedTemplateId, setSelectedTemplateId] = useState('');
  const [taskType, setTaskType] = useState<TaskType>('claude');

  // Sync task type from step
  useEffect(() => {
    if (step) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing task type from step data
      setTaskType(resolveTaskType(step));
    }
  }, [step?.step_id, step?.task_type]);

  const checkDirty = useCallback(() => {
    if (!formRef.current || !step || !onDirtyChange) return;
    const dirty = isDirty(formRef.current, step, taskType);
    if (dirty !== lastDirty.current) {
      lastDirty.current = dirty;
      onDirtyChange(dirty);
    }
  }, [step, onDirtyChange, taskType]);

  // Reset state when step changes
  useEffect(() => {
    lastDirty.current = false;
    onDirtyChange?.(false);
    // eslint-disable-next-line react-hooks/set-state-in-effect -- resetting local form state when step changes
    setSelectedTemplateId('');
  }, [step?.step_id, onDirtyChange]);

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

  if (!step) {
    return (
      <div className="flex items-center justify-center h-full text-text-secondary text-sm">
        Select a step to edit its configuration
      </div>
    );
  }

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!step) return;
    const params = getFormParams(e.currentTarget, taskType);
    onSave(step.step_id, params);
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

  const maxRetries = step.max_retries ?? 0;

  return (
    <form
      ref={formRef}
      onSubmit={handleSubmit}
      onChange={checkDirty}
      className="p-4 space-y-3 overflow-y-auto h-full"
    >
      <h3 className="text-sm font-medium text-text-primary">Step Configuration</h3>

      {/* Task Type Toggle */}
      <div>
        <label className="block text-xs text-text-secondary mb-1">Task Type</label>
        <div className="flex rounded-md overflow-hidden border border-border-primary">
          <button
            type="button"
            onClick={() => handleTaskTypeChange('claude')}
            className={`flex-1 py-1.5 text-xs font-medium transition-colors ${
              taskType === 'claude'
                ? 'bg-accent-primary text-white'
                : 'bg-bg-tertiary text-text-secondary hover:text-text-primary'
            }`}
          >
            Claude Session
          </button>
          <button
            type="button"
            onClick={() => handleTaskTypeChange('shell')}
            className={`flex-1 py-1.5 text-xs font-medium transition-colors ${
              taskType === 'shell'
                ? 'bg-accent-primary text-white'
                : 'bg-bg-tertiary text-text-secondary hover:text-text-primary'
            }`}
          >
            Shell
          </button>
        </div>
      </div>

      {/* Template selector — only for Claude sessions */}
      {taskType === 'claude' && templates && templates.length > 0 && (
        <TemplateSelector
          templates={templates}
          selectedId={selectedTemplateId}
          stepId={step.step_id}
          onSelect={handleTemplateSelect}
        />
      )}

      <div>
        <label htmlFor="step-name" className="block text-xs text-text-secondary mb-1">Name</label>
        <input
          id="step-name"
          name="name"
          type="text"
          defaultValue={step.name}
          key={step.step_id + '-name'}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
        />
      </div>

      {/* Prompt — Claude only */}
      {taskType === 'claude' && (
        <div>
          <label htmlFor="step-prompt" className="block text-xs text-text-secondary mb-1">Prompt</label>
          <textarea
            id="step-prompt"
            name="prompt"
            rows={4}
            defaultValue={step.prompt}
            key={step.step_id + '-prompt'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary resize-none font-mono"
            placeholder="Enter the prompt for Claude..."
          />
        </div>
      )}

      <div>
        <label htmlFor="step-machine" className="block text-xs text-text-secondary mb-1">Machine</label>
        <select
          id="step-machine"
          name="machine_id"
          defaultValue={step.machine_id}
          key={step.step_id + '-machine'}
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

      {/* Model — Claude only */}
      {taskType === 'claude' && (
        <div>
          <label htmlFor="step-model" className="block text-xs text-text-secondary mb-1">Model</label>
          <select
            id="step-model"
            name="model"
            defaultValue={step.model ?? ''}
            key={step.step_id + '-model'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
          >
            <option value="">Default</option>
            <option value="opus">Opus</option>
            <option value="sonnet">Sonnet</option>
            <option value="haiku">Haiku</option>
          </select>
        </div>
      )}

      {/* Skip Permissions — Claude only */}
      {taskType === 'claude' && (
        <div>
          <label htmlFor="step-skip-permissions" className="block text-xs text-text-secondary mb-1">Skip Permissions</label>
          <select
            id="step-skip-permissions"
            name="skip_permissions"
            defaultValue={skipPermissionsFormValue(step)}
            key={step.step_id + '-skip-permissions'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
          >
            <option value="">Default (from settings)</option>
            <option value="1">On</option>
            <option value="0">Off</option>
          </select>
        </div>
      )}

      {/* Session Key — Claude only */}
      {taskType === 'claude' && (
        <div>
          <label htmlFor="step-session-key" className="block text-xs text-text-secondary mb-1">Session Key</label>
          <input
            id="step-session-key"
            name="session_key"
            type="text"
            defaultValue={step.session_key ?? ''}
            key={step.step_id + '-session-key'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary font-mono"
            placeholder="Optional — steps sharing a key reuse the same session"
          />
        </div>
      )}

      <div>
        <label htmlFor="step-delay" className="block text-xs text-text-secondary mb-1">Delay (seconds)</label>
        <input
          id="step-delay"
          name="delay_seconds"
          type="number"
          min={0}
          max={86400}
          defaultValue={step.delay_seconds ?? 0}
          key={step.step_id + '-delay'}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
        />
        <p className="text-[10px] text-text-secondary/70 mt-0.5">Wait before starting this step (0-86400)</p>
      </div>

      <div>
        <label htmlFor="step-workdir" className="block text-xs text-text-secondary mb-1">Working Directory</label>
        <input
          id="step-workdir"
          name="working_dir"
          type="text"
          defaultValue={step.working_dir}
          key={step.step_id + '-workdir'}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary font-mono"
          placeholder="/home/user/project"
        />
      </div>

      <div>
        <label htmlFor="step-command" className="block text-xs text-text-secondary mb-1">
          Command{taskType === 'shell' ? ' (required)' : ''}
        </label>
        <input
          id="step-command"
          name="command"
          type="text"
          defaultValue={taskType === 'claude' ? (step.command || 'claude') : step.command}
          key={step.step_id + '-command-' + taskType}
          required={taskType === 'shell'}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary font-mono"
        />
      </div>

      <div>
        <label htmlFor="step-args" className="block text-xs text-text-secondary mb-1">Args (one per line)</label>
        <textarea
          id="step-args"
          name="args"
          rows={2}
          defaultValue={step.args ?? ''}
          key={step.step_id + '-args'}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary resize-none font-mono"
        />
      </div>

      {/* Run If */}
      <div>
        <label htmlFor="step-run-if" className="block text-xs text-text-secondary mb-1">Run If</label>
        <select
          id="step-run-if"
          name="run_if"
          defaultValue={step.run_if ?? ''}
          key={step.step_id + '-run-if'}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
        >
          <option value="">Always (default)</option>
          <option value="all_success">All dependencies succeeded</option>
          <option value="at_least_one_success">At least one dependency succeeded</option>
          <option value="all_done">All dependencies completed (any status)</option>
          <option value="all_failed">All dependencies failed</option>
        </select>
      </div>

      {/* Max Retries */}
      <div>
        <label htmlFor="step-max-retries" className="block text-xs text-text-secondary mb-1">Max Retries</label>
        <input
          id="step-max-retries"
          name="max_retries"
          type="number"
          min={0}
          max={5}
          defaultValue={maxRetries}
          key={step.step_id + '-max-retries'}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
        />
      </div>

      {/* Retry Delay — shown if max_retries > 0 */}
      {maxRetries > 0 && (
        <div>
          <label htmlFor="step-retry-delay" className="block text-xs text-text-secondary mb-1">Retry Delay (seconds)</label>
          <input
            id="step-retry-delay"
            name="retry_delay_seconds"
            type="number"
            min={0}
            max={3600}
            defaultValue={step.retry_delay_seconds ?? 0}
            key={step.step_id + '-retry-delay'}
            className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
          />
        </div>
      )}

      {/* On Failure */}
      <div>
        <label htmlFor="step-on-failure" className="block text-xs text-text-secondary mb-1">On Failure</label>
        <select
          id="step-on-failure"
          name="on_failure"
          defaultValue={step.on_failure ?? ''}
          key={step.step_id + '-on-failure'}
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
          Save Step
        </button>
        <button
          type="button"
          onClick={() => onDelete(step.step_id)}
          className="px-3 py-1.5 text-sm rounded-md bg-red-600/20 text-red-400 hover:bg-red-600/30 transition-colors"
        >
          Delete
        </button>
      </div>
    </form>
  );
}
