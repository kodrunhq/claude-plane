import { useCallback } from 'react';
import { Plus, Trash2 } from 'lucide-react';

const PARAM_KEY_REGEX = /^[a-zA-Z_][a-zA-Z0-9_]*$/;

interface ParameterEditorProps {
  parameters: Record<string, string>;
  onChange: (parameters: Record<string, string>) => void;
}

function validateKey(key: string): string | null {
  if (!key.trim()) return 'Key is required';
  if (!PARAM_KEY_REGEX.test(key)) return 'Key must start with a letter or underscore and contain only alphanumeric characters and underscores';
  return null;
}

export function ParameterEditor({ parameters, onChange }: ParameterEditorProps) {
  const entries = Object.entries(parameters);

  const handleAdd = useCallback(() => {
    const newKey = generateUniqueKey(parameters);
    onChange({ ...parameters, [newKey]: '' });
  }, [parameters, onChange]);

  const handleRemove = useCallback(
    (key: string) => {
      const next = { ...parameters };
      delete next[key];
      onChange(next);
    },
    [parameters, onChange],
  );

  const handleKeyChange = useCallback(
    (oldKey: string, newKey: string) => {
      if (oldKey === newKey) return;
      // Build new object preserving order, replacing the old key
      const next: Record<string, string> = {};
      for (const [k, v] of Object.entries(parameters)) {
        if (k === oldKey) {
          next[newKey] = v;
        } else {
          next[k] = v;
        }
      }
      onChange(next);
    },
    [parameters, onChange],
  );

  const handleValueChange = useCallback(
    (key: string, value: string) => {
      onChange({ ...parameters, [key]: value });
    },
    [parameters, onChange],
  );

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <label className="block text-xs font-medium text-text-secondary">Parameters</label>
        <button
          type="button"
          onClick={handleAdd}
          className="flex items-center gap-1 text-xs text-accent-primary hover:text-accent-primary/80 transition-colors"
        >
          <Plus size={12} />
          Add
        </button>
      </div>

      {entries.length === 0 && (
        <p className="text-xs text-text-secondary/70">
          No parameters defined. Parameters let you pass values when triggering a run.
        </p>
      )}

      {entries.map(([key, value]) => {
        const error = key ? validateKey(key) : null;
        return (
          <ParameterRow
            key={key}
            paramKey={key}
            paramValue={value}
            error={error}
            onKeyChange={(newKey) => handleKeyChange(key, newKey)}
            onValueChange={(newValue) => handleValueChange(key, newValue)}
            onRemove={() => handleRemove(key)}
          />
        );
      })}
    </div>
  );
}

interface ParameterRowProps {
  paramKey: string;
  paramValue: string;
  error: string | null;
  onKeyChange: (newKey: string) => void;
  onValueChange: (newValue: string) => void;
  onRemove: () => void;
}

function ParameterRow({ paramKey, paramValue, error, onKeyChange, onValueChange, onRemove }: ParameterRowProps) {
  return (
    <div className="space-y-0.5">
      <div className="flex items-center gap-2">
        <input
          type="text"
          value={paramKey}
          onChange={(e) => onKeyChange(e.target.value)}
          placeholder="KEY"
          className={`flex-1 px-2 py-1 text-xs rounded-md bg-bg-tertiary border text-text-primary font-mono focus:outline-none focus:border-accent-primary ${
            error ? 'border-red-500' : 'border-border-primary'
          }`}
        />
        <input
          type="text"
          value={paramValue}
          onChange={(e) => onValueChange(e.target.value)}
          placeholder="default value"
          className="flex-1 px-2 py-1 text-xs rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
        />
        <button
          type="button"
          onClick={onRemove}
          className="text-text-secondary hover:text-red-400 transition-colors shrink-0"
        >
          <Trash2 size={14} />
        </button>
      </div>
      {error && <p className="text-[10px] text-red-400 pl-1">{error}</p>}
    </div>
  );
}

function generateUniqueKey(existing: Record<string, string>): string {
  let i = 1;
  while (`PARAM_${i}` in existing) i++;
  return `PARAM_${i}`;
}
