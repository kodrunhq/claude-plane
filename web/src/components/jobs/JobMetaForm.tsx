interface JobMetaFormProps {
  name: string;
  description: string;
  onChange: (field: 'name' | 'description', value: string) => void;
}

export function JobMetaForm({ name, description, onChange }: JobMetaFormProps) {
  return (
    <div className="space-y-3">
      <div>
        <label htmlFor="job-name" className="block text-xs font-medium text-text-secondary mb-1">
          Job Name
        </label>
        <input
          id="job-name"
          type="text"
          value={name}
          onChange={(e) => onChange('name', e.target.value)}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
          placeholder="My Job"
        />
      </div>
      <div>
        <label htmlFor="job-desc" className="block text-xs font-medium text-text-secondary mb-1">
          Description
        </label>
        <textarea
          id="job-desc"
          value={description}
          onChange={(e) => onChange('description', e.target.value)}
          rows={2}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary resize-none"
          placeholder="Optional description..."
        />
      </div>
    </div>
  );
}
