import { useState, useCallback } from 'react';
import { X, Plus } from 'lucide-react';
import type { SessionTemplate, CreateTemplateParams } from '../../types/template.ts';

interface TemplateFormProps {
  initialValues?: SessionTemplate;
  onSubmit: (params: CreateTemplateParams) => Promise<void>;
  onCancel: () => void;
  isLoading?: boolean;
}

export function TemplateForm({ initialValues, onSubmit, onCancel, isLoading }: TemplateFormProps) {
  const [name, setName] = useState(initialValues?.name ?? '');
  const [description, setDescription] = useState(initialValues?.description ?? '');
  const [command, setCommand] = useState(initialValues?.command ?? '');
  const [args, setArgs] = useState<string[]>(initialValues?.args ?? []);
  const [argInput, setArgInput] = useState('');
  const [workingDir, setWorkingDir] = useState(initialValues?.working_dir ?? '');
  const [envVars, setEnvVars] = useState<Array<{ key: string; value: string }>>(
    Object.entries(initialValues?.env_vars ?? {}).map(([key, value]) => ({ key, value })),
  );
  const [envKeyInput, setEnvKeyInput] = useState('');
  const [envValueInput, setEnvValueInput] = useState('');
  const [initialPrompt, setInitialPrompt] = useState(initialValues?.initial_prompt ?? '');
  const [timeoutSeconds, setTimeoutSeconds] = useState(initialValues?.timeout_seconds ?? 0);

  const addArg = useCallback(() => {
    const trimmed = argInput.trim();
    if (!trimmed) return;
    setArgs((prev) => [...prev, trimmed]);
    setArgInput('');
  }, [argInput]);

  const removeArg = useCallback((index: number) => {
    setArgs((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const addEnvVar = useCallback(() => {
    const key = envKeyInput.trim();
    const value = envValueInput.trim();
    if (!key) return;
    setEnvVars((prev) => [...prev, { key, value }]);
    setEnvKeyInput('');
    setEnvValueInput('');
  }, [envKeyInput, envValueInput]);

  const removeEnvVar = useCallback((index: number) => {
    setEnvVars((prev) => prev.filter((_, i) => i !== index));
  }, []);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();

    const envVarsObj: Record<string, string> = {};
    for (const { key, value } of envVars) {
      if (key) envVarsObj[key] = value;
    }

    const params: CreateTemplateParams = {
      name,
      ...(description ? { description } : {}),
      ...(command ? { command } : {}),
      ...(args.length > 0 ? { args } : {}),
      ...(workingDir ? { working_dir: workingDir } : {}),
      ...(Object.keys(envVarsObj).length > 0 ? { env_vars: envVarsObj } : {}),
      ...(initialPrompt ? { initial_prompt: initialPrompt } : {}),
      terminal_rows: initialValues?.terminal_rows ?? 24,
      terminal_cols: initialValues?.terminal_cols ?? 80,
      ...(initialValues?.tags && initialValues.tags.length > 0 ? { tags: initialValues.tags } : {}),
      ...(timeoutSeconds > 0 ? { timeout_seconds: timeoutSeconds } : {}),
    };

    await onSubmit(params);
  }

  const inputClass =
    'w-full rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-2 focus:outline-none focus:ring-1 focus:ring-accent-primary placeholder:text-text-secondary/30';

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-5 max-w-2xl">
      {/* Name */}
      <div>
        <label className="block text-sm text-text-secondary mb-1">
          Name <span className="text-status-error">*</span>
        </label>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="My Template"
          className={inputClass}
          required
        />
      </div>

      {/* Description */}
      <div>
        <label className="block text-sm text-text-secondary mb-1">Description</label>
        <input
          type="text"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="What this template is for..."
          className={inputClass}
        />
      </div>

      {/* Command */}
      <div>
        <label className="block text-sm text-text-secondary mb-1">Command</label>
        <input
          type="text"
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          placeholder="claude"
          className={inputClass}
        />
      </div>

      {/* Args */}
      <div>
        <label className="block text-sm text-text-secondary mb-1">Arguments</label>
        <div className="flex gap-2">
          <input
            type="text"
            value={argInput}
            onChange={(e) => setArgInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                addArg();
              }
            }}
            placeholder="Add argument..."
            className={inputClass}
          />
          <button
            type="button"
            onClick={addArg}
            className="shrink-0 px-3 py-2 text-sm rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary border border-gray-600 transition-colors"
          >
            <Plus size={14} />
          </button>
        </div>
        {args.length > 0 && (
          <div className="flex flex-wrap gap-1.5 mt-2">
            {args.map((arg, i) => (
              <span
                key={i}
                className="inline-flex items-center gap-1 bg-bg-tertiary text-text-secondary rounded-full px-2.5 py-1 text-xs"
              >
                {arg}
                <button
                  type="button"
                  onClick={() => removeArg(i)}
                  className="text-text-secondary/60 hover:text-text-primary"
                >
                  <X size={12} />
                </button>
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Working Directory */}
      <div>
        <label className="block text-sm text-text-secondary mb-1">Working Directory</label>
        <input
          type="text"
          value={workingDir}
          onChange={(e) => setWorkingDir(e.target.value)}
          placeholder="/home/user/project"
          className={inputClass}
        />
      </div>

      {/* Env Vars */}
      <div>
        <label className="block text-sm text-text-secondary mb-1">Environment Variables</label>
        <div className="flex gap-2">
          <input
            type="text"
            value={envKeyInput}
            onChange={(e) => setEnvKeyInput(e.target.value)}
            placeholder="KEY"
            className={inputClass}
          />
          <input
            type="text"
            value={envValueInput}
            onChange={(e) => setEnvValueInput(e.target.value)}
            placeholder="value"
            className={inputClass}
          />
          <button
            type="button"
            onClick={addEnvVar}
            className="shrink-0 px-3 py-2 text-sm rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary border border-gray-600 transition-colors"
          >
            <Plus size={14} />
          </button>
        </div>
        {envVars.length > 0 && (
          <div className="mt-2 space-y-1">
            {envVars.map((ev, i) => (
              <div
                key={i}
                className="flex items-center gap-2 bg-bg-tertiary rounded-md px-3 py-1.5 text-xs"
              >
                <span className="text-text-primary font-mono">{ev.key}</span>
                <span className="text-text-secondary">=</span>
                <span className="text-text-secondary font-mono flex-1 truncate">{ev.value}</span>
                <button
                  type="button"
                  onClick={() => removeEnvVar(i)}
                  className="text-text-secondary/60 hover:text-text-primary shrink-0"
                >
                  <X size={12} />
                </button>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Initial Prompt */}
      <div>
        <label className="block text-sm text-text-secondary mb-1">Initial Prompt</label>
        <textarea
          value={initialPrompt}
          onChange={(e) => setInitialPrompt(e.target.value)}
          placeholder="Use ${VAR_NAME} for template variables..."
          rows={4}
          className={`${inputClass} resize-y`}
        />
      </div>

      {/* Timeout */}
      <div>
        <label className="block text-sm text-text-secondary mb-1">
          Timeout (seconds) <span className="text-text-secondary/50">(0 = no timeout)</span>
        </label>
        <input
          type="number"
          value={timeoutSeconds}
          onChange={(e) => setTimeoutSeconds(Number(e.target.value))}
          min={0}
          className={inputClass}
        />
      </div>

      {/* Actions */}
      <div className="flex justify-end gap-3 pt-2">
        <button
          type="button"
          onClick={onCancel}
          className="px-4 py-2 text-sm rounded-md text-text-secondary hover:text-text-primary bg-bg-tertiary hover:bg-bg-tertiary/80 transition-colors"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={!name.trim() || isLoading}
          className="px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {isLoading ? 'Saving...' : initialValues ? 'Update Template' : 'Create Template'}
        </button>
      </div>
    </form>
  );
}
