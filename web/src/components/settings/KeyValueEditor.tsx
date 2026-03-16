import { Plus, Trash2 } from 'lucide-react';

interface KeyValueEditorProps {
  entries: ReadonlyArray<readonly [string, string]>;
  onChange: (entries: ReadonlyArray<readonly [string, string]>) => void;
  keyPlaceholder?: string;
  valuePlaceholder?: string;
}

export function KeyValueEditor({
  entries,
  onChange,
  keyPlaceholder = 'Key',
  valuePlaceholder = 'Value',
}: KeyValueEditorProps) {
  function handleAdd() {
    onChange([...entries, ['', ''] as const]);
  }

  function handleRemove(index: number) {
    onChange(entries.filter((_, i) => i !== index));
  }

  function handleKeyChange(index: number, key: string) {
    onChange(entries.map((entry, i) => (i === index ? [key, entry[1]] as const : entry)));
  }

  function handleValueChange(index: number, value: string) {
    onChange(entries.map((entry, i) => (i === index ? [entry[0], value] as const : entry)));
  }

  return (
    <div className="space-y-2">
      {entries.map(([key, value], index) => (
        <div key={index} className="flex flex-col sm:flex-row sm:items-center gap-2">
          <input
            type="text"
            value={key}
            onChange={(e) => handleKeyChange(index, e.target.value)}
            placeholder={keyPlaceholder}
            className="flex-1 px-3 py-2 text-sm rounded-lg bg-bg-tertiary border border-border-primary text-text-primary placeholder:text-text-secondary/50 focus:outline-none focus:ring-1 focus:ring-accent-primary"
          />
          <input
            type="text"
            value={value}
            onChange={(e) => handleValueChange(index, e.target.value)}
            placeholder={valuePlaceholder}
            className="flex-1 px-3 py-2 text-sm rounded-lg bg-bg-tertiary border border-border-primary text-text-primary placeholder:text-text-secondary/50 focus:outline-none focus:ring-1 focus:ring-accent-primary"
          />
          <button
            type="button"
            onClick={() => handleRemove(index)}
            className="p-2 rounded-lg text-text-secondary hover:text-status-error hover:bg-status-error/10 transition-colors"
          >
            <Trash2 size={16} />
          </button>
        </div>
      ))}
      <button
        type="button"
        onClick={handleAdd}
        className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md text-text-secondary hover:text-text-primary hover:bg-bg-tertiary transition-colors"
      >
        <Plus size={14} />
        Add Variable
      </button>
    </div>
  );
}
