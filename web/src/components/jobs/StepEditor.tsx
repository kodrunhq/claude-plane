import { useRef, useCallback, useEffect } from 'react';
import type { Step, UpdateStepParams } from '../../types/job.ts';
import type { Machine } from '../../lib/types.ts';

interface StepEditorProps {
  step: Step | null;
  machines: Machine[];
  onSave: (stepId: string, params: UpdateStepParams) => void;
  onDelete: (stepId: string) => void;
  onDirtyChange?: (dirty: boolean) => void;
}

function getFormParams(form: HTMLFormElement): UpdateStepParams {
  const data = new FormData(form);
  return {
    name: data.get('name') as string,
    prompt: data.get('prompt') as string,
    machine_id: data.get('machine_id') as string,
    working_dir: data.get('working_dir') as string,
    command: (data.get('command') as string) || 'claude',
    args: data.get('args') as string,
  };
}

function isDirty(form: HTMLFormElement, step: Step): boolean {
  const params = getFormParams(form);
  return (
    params.name !== step.name ||
    params.prompt !== step.prompt ||
    params.machine_id !== step.machine_id ||
    params.working_dir !== step.working_dir ||
    params.command !== (step.command || 'claude') ||
    params.args !== (step.args ?? '')
  );
}

export function StepEditor({ step, machines, onSave, onDelete, onDirtyChange }: StepEditorProps) {
  const formRef = useRef<HTMLFormElement>(null);
  const lastDirty = useRef(false);

  const checkDirty = useCallback(() => {
    if (!formRef.current || !step || !onDirtyChange) return;
    const dirty = isDirty(formRef.current, step);
    if (dirty !== lastDirty.current) {
      lastDirty.current = dirty;
      onDirtyChange(dirty);
    }
  }, [step, onDirtyChange]);

  // Reset dirty state when step changes
  useEffect(() => {
    lastDirty.current = false;
    onDirtyChange?.(false);
  }, [step?.step_id, onDirtyChange]);

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
    const params = getFormParams(e.currentTarget);
    onSave(step.step_id, params);
    lastDirty.current = false;
    onDirtyChange?.(false);
  }

  return (
    <form
      ref={formRef}
      onSubmit={handleSubmit}
      onChange={checkDirty}
      className="p-4 space-y-3 overflow-y-auto h-full"
    >
      <h3 className="text-sm font-medium text-text-primary">Step Configuration</h3>

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
        <label htmlFor="step-command" className="block text-xs text-text-secondary mb-1">Command</label>
        <input
          id="step-command"
          name="command"
          type="text"
          defaultValue={step.command || 'claude'}
          key={step.step_id + '-command'}
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
