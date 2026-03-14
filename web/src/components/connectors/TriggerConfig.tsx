import { useState } from 'react';
import { ChevronDown, ChevronRight, X } from 'lucide-react';

export interface TriggerFilters {
  branches?: string[];
  labels?: string[];
  check_names?: string[];
  conclusions?: string[];
  paths?: string[];
  authors_ignore?: string[];
}

export interface TriggerConfigProps {
  type: 'pull_request_opened' | 'check_run_completed' | 'issue_labeled';
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
  filters: TriggerFilters;
  onFiltersChange: (filters: TriggerFilters) => void;
}

const TRIGGER_LABELS: Record<TriggerConfigProps['type'], string> = {
  pull_request_opened: 'Pull Request Opened',
  check_run_completed: 'Check Run Completed',
  issue_labeled: 'Issue Labeled',
};

const CONCLUSION_OPTIONS = [
  'success',
  'failure',
  'timed_out',
  'cancelled',
  'neutral',
  'skipped',
] as const;

const tagInputClass =
  'flex-1 min-w-[120px] bg-transparent text-text-primary text-xs py-1 px-1 focus:outline-none placeholder:text-text-secondary/30';

const pillClass =
  'inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs bg-accent-primary/15 text-accent-primary border border-accent-primary/25';

function TagInput({
  values,
  onChange,
  placeholder,
}: {
  values: string[];
  onChange: (values: string[]) => void;
  placeholder: string;
}) {
  const [input, setInput] = useState('');

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter' && input.trim()) {
      e.preventDefault();
      const trimmed = input.trim();
      if (!values.includes(trimmed)) {
        onChange([...values, trimmed]);
      }
      setInput('');
    } else if (e.key === 'Backspace' && input === '' && values.length > 0) {
      onChange(values.slice(0, -1));
    }
  }

  function remove(val: string) {
    onChange(values.filter((v) => v !== val));
  }

  return (
    <div className="flex flex-wrap gap-1.5 items-center min-h-[36px] w-full rounded-md bg-bg-tertiary border border-gray-600 px-2 py-1.5 focus-within:ring-1 focus-within:ring-accent-primary">
      {values.map((v) => (
        <span key={v} className={pillClass}>
          {v}
          <button
            type="button"
            onClick={() => remove(v)}
            className="hover:text-status-error transition-colors"
            aria-label={`Remove ${v}`}
          >
            <X size={10} />
          </button>
        </span>
      ))}
      <input
        type="text"
        value={input}
        onChange={(e) => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={values.length === 0 ? placeholder : ''}
        className={tagInputClass}
      />
    </div>
  );
}

function FilterField({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="block text-xs text-text-secondary mb-1">{label}</label>
      {children}
    </div>
  );
}

export function TriggerConfig({
  type,
  enabled,
  onToggle,
  filters,
  onFiltersChange,
}: TriggerConfigProps) {
  const [expanded, setExpanded] = useState(false);
  const label = TRIGGER_LABELS[type];

  function updateFilter<K extends keyof TriggerFilters>(
    key: K,
    value: TriggerFilters[K],
  ) {
    onFiltersChange({ ...filters, [key]: value });
  }

  function toggleConclusion(conclusion: string) {
    const current = filters.conclusions ?? [];
    const next = current.includes(conclusion)
      ? current.filter((c) => c !== conclusion)
      : [...current, conclusion];
    updateFilter('conclusions', next);
  }

  return (
    <div className="rounded-md border border-border-primary bg-bg-primary">
      {/* Header row */}
      <div className="flex items-center gap-3 px-3 py-2.5">
        {/* Toggle */}
        <button
          type="button"
          role="switch"
          aria-checked={enabled}
          onClick={() => onToggle(!enabled)}
          className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus:outline-none ${
            enabled ? 'bg-accent-primary' : 'bg-gray-600'
          }`}
        >
          <span
            className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
              enabled ? 'translate-x-4' : 'translate-x-0'
            }`}
          />
        </button>

        <span className="flex-1 text-sm text-text-primary font-medium">{label}</span>

        {/* Expand/collapse (only when enabled) */}
        {enabled && (
          <button
            type="button"
            onClick={() => setExpanded((prev) => !prev)}
            className="text-text-secondary hover:text-text-primary transition-colors"
            aria-label={expanded ? 'Collapse filters' : 'Expand filters'}
          >
            {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </button>
        )}
      </div>

      {/* Filter fields */}
      {enabled && expanded && (
        <div className="border-t border-border-primary px-3 py-3 flex flex-col gap-3">
          {type === 'pull_request_opened' && (
            <>
              <FilterField label="Branches (press Enter to add)">
                <TagInput
                  values={filters.branches ?? []}
                  onChange={(v) => updateFilter('branches', v)}
                  placeholder="e.g. main, release/*"
                />
              </FilterField>
              <FilterField label="Labels (press Enter to add)">
                <TagInput
                  values={filters.labels ?? []}
                  onChange={(v) => updateFilter('labels', v)}
                  placeholder="e.g. bug, enhancement"
                />
              </FilterField>
              <FilterField label="Paths (press Enter to add)">
                <TagInput
                  values={filters.paths ?? []}
                  onChange={(v) => updateFilter('paths', v)}
                  placeholder="e.g. src/**, *.go"
                />
              </FilterField>
              <FilterField label="Ignore authors (press Enter to add)">
                <TagInput
                  values={filters.authors_ignore ?? []}
                  onChange={(v) => updateFilter('authors_ignore', v)}
                  placeholder="e.g. dependabot[bot]"
                />
              </FilterField>
            </>
          )}

          {type === 'check_run_completed' && (
            <>
              <FilterField label="Check names (press Enter to add)">
                <TagInput
                  values={filters.check_names ?? []}
                  onChange={(v) => updateFilter('check_names', v)}
                  placeholder="e.g. CI, Build"
                />
              </FilterField>
              <FilterField label="Conclusions">
                <div className="flex flex-wrap gap-2">
                  {CONCLUSION_OPTIONS.map((c) => {
                    const checked = (filters.conclusions ?? []).includes(c);
                    return (
                      <label
                        key={c}
                        className="flex items-center gap-1.5 cursor-pointer select-none"
                      >
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={() => toggleConclusion(c)}
                          className="accent-accent-primary"
                        />
                        <span className="text-xs text-text-primary">{c}</span>
                      </label>
                    );
                  })}
                </div>
              </FilterField>
            </>
          )}

          {type === 'issue_labeled' && (
            <FilterField label="Labels (press Enter to add)">
              <TagInput
                values={filters.labels ?? []}
                onChange={(v) => updateFilter('labels', v)}
                placeholder="e.g. bug, help wanted"
              />
            </FilterField>
          )}
        </div>
      )}
    </div>
  );
}
