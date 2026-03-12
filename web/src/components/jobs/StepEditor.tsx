import type { Step, UpdateStepParams } from '../../types/job.ts';
import type { Machine } from '../../lib/types.ts';

interface StepEditorProps {
  step: Step | null;
  machines: Machine[];
  onSave: (stepId: string, params: UpdateStepParams) => void;
  onDelete: (stepId: string) => void;
}

export function StepEditor({ step, machines, onSave, onDelete }: StepEditorProps) {
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
    const form = new FormData(e.currentTarget);
    const params: UpdateStepParams = {
      name: form.get('name') as string,
      prompt: form.get('prompt') as string,
      machine_id: form.get('machine_id') as string,
      working_dir: form.get('working_dir') as string,
      command: form.get('command') as string || 'claude',
      args: (form.get('args') as string),
    };
    onSave(step.step_id, params);
  }

  return (
    <form onSubmit={handleSubmit} className="p-4 space-y-3 overflow-y-auto h-full">
      <h3 className="text-sm font-medium text-text-primary">Step Configuration</h3>

      <div>
        <label htmlFor="step-name" className="block text-xs text-text-secondary mb-1">Name</label>
        <input
          id="step-name"
          name="name"
          type="text"
          defaultValue={step.name}
          key={step.step_id + '-name'}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-gray-700 text-text-primary focus:outline-none focus:border-accent-primary"
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
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-gray-700 text-text-primary focus:outline-none focus:border-accent-primary resize-none font-mono"
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
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-gray-700 text-text-primary focus:outline-none focus:border-accent-primary"
        >
          <option value="">Select machine...</option>
          {machines.map((m) => (
            <option key={m.machine_id} value={m.machine_id}>
              {m.hostname} ({m.machine_id.slice(0, 8)})
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
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-gray-700 text-text-primary focus:outline-none focus:border-accent-primary font-mono"
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
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-gray-700 text-text-primary focus:outline-none focus:border-accent-primary font-mono"
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
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-gray-700 text-text-primary focus:outline-none focus:border-accent-primary resize-none font-mono"
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
