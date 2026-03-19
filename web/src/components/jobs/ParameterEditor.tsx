import { useState, useCallback, useEffect, useMemo, useRef } from 'react';
import { Plus, Trash2 } from 'lucide-react';
import { generateUUID } from '../../lib/uuid';

const PARAM_KEY_REGEX = /^[a-zA-Z_][a-zA-Z0-9_]*$/;

interface ParameterEntry {
  id: string;
  key: string;
  value: string;
}

interface ParameterEditorProps {
  parameters: Record<string, string>;
  onChange: (parameters: Record<string, string>) => void;
}

function generateId(): string {
  return generateUUID();
}

function toEntries(params: Record<string, string>): ParameterEntry[] {
  return Object.entries(params).map(([key, value]) => ({
    id: generateId(),
    key,
    value,
  }));
}

function toRecord(entries: ParameterEntry[]): Record<string, string> | null {
  const seen = new Set<string>();
  for (const entry of entries) {
    if (entry.key && seen.has(entry.key)) {
      return null; // duplicate keys — don't emit
    }
    if (entry.key) {
      seen.add(entry.key);
    }
  }
  const result: Record<string, string> = {};
  for (const entry of entries) {
    if (entry.key) {
      result[entry.key] = entry.value;
    }
  }
  return result;
}

function validateKey(key: string): string | null {
  if (!key.trim()) return 'Key is required';
  if (!PARAM_KEY_REGEX.test(key)) return 'Key must start with a letter or underscore and contain only alphanumeric characters and underscores';
  return null;
}

export function ParameterEditor({ parameters, onChange }: ParameterEditorProps) {
  const [entries, setEntries] = useState<ParameterEntry[]>(() => toEntries(parameters));

  // Track the last value we emitted via onChange so we can distinguish
  // parent updates caused by our own edits (skip sync) from external
  // changes like a server load (need sync).
  const serializedParams = useMemo(() => JSON.stringify(parameters), [parameters]);
  const lastEmittedRef = useRef(serializedParams);

  useEffect(() => {
    // Skip sync when the incoming value matches what we last emitted —
    // this prevents regenerating entry IDs (and losing input focus) on
    // every keystroke.
    if (lastEmittedRef.current === serializedParams) return;
    lastEmittedRef.current = serializedParams;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- syncing server data to local entries on external parameter change
    setEntries(toEntries(parameters));
  }, [serializedParams]); // eslint-disable-line react-hooks/exhaustive-deps -- parameters is captured via serializedParams

  const duplicateKeys = useMemo(() => {
    const seen = new Map<string, number>();
    for (const entry of entries) {
      if (entry.key) {
        seen.set(entry.key, (seen.get(entry.key) ?? 0) + 1);
      }
    }
    const dupes = new Set<string>();
    for (const [key, count] of seen) {
      if (count > 1) dupes.add(key);
    }
    return dupes;
  }, [entries]);

  const emitChange = useCallback(
    (updated: ParameterEntry[]) => {
      setEntries(updated);
      const hasEmptyKey = updated.some((e) => !e.key);
      if (hasEmptyKey) return;
      const record = toRecord(updated);
      if (record !== null) {
        lastEmittedRef.current = JSON.stringify(record);
        onChange(record);
      }
    },
    [onChange],
  );

  const handleAdd = useCallback(() => {
    let i = 1;
    const existingKeys = new Set(entries.map((e) => e.key));
    while (existingKeys.has(`PARAM_${i}`)) i++;
    const newEntry: ParameterEntry = { id: generateId(), key: `PARAM_${i}`, value: '' };
    emitChange([...entries, newEntry]);
  }, [entries, emitChange]);

  const handleRemove = useCallback(
    (id: string) => {
      emitChange(entries.filter((e) => e.id !== id));
    },
    [entries, emitChange],
  );

  const handleKeyChange = useCallback(
    (id: string, newKey: string) => {
      emitChange(entries.map((e) => (e.id === id ? { ...e, key: newKey } : e)));
    },
    [entries, emitChange],
  );

  const handleValueChange = useCallback(
    (id: string, value: string) => {
      emitChange(entries.map((e) => (e.id === id ? { ...e, value } : e)));
    },
    [entries, emitChange],
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

      {entries.map((entry) => {
        const keyError = entry.key ? validateKey(entry.key) : 'Key required';
        const duplicateError = duplicateKeys.has(entry.key) ? 'Duplicate key' : null;
        const error = keyError ?? duplicateError;
        return (
          <ParameterRow
            key={entry.id}
            paramKey={entry.key}
            paramValue={entry.value}
            error={error}
            onKeyChange={(newKey) => handleKeyChange(entry.id, newKey)}
            onValueChange={(newValue) => handleValueChange(entry.id, newValue)}
            onRemove={() => handleRemove(entry.id)}
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
