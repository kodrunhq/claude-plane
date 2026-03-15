import { useState } from 'react';
import { X, Play } from 'lucide-react';

interface RunNowModalProps {
  defaultParameters: Record<string, string>;
  onRun: (parameters: Record<string, string>) => void;
  onClose: () => void;
}

export function RunNowModal({ defaultParameters, onRun, onClose }: RunNowModalProps) {
  const [overrides, setOverrides] = useState<Record<string, string>>({ ...defaultParameters });

  const entries = Object.entries(overrides);
  const hasParams = entries.length > 0;

  function handleValueChange(key: string, value: string) {
    setOverrides({ ...overrides, [key]: value });
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    onRun(overrides);
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
      onClick={onClose}
    >
      <div
        className="bg-bg-primary border border-border-primary rounded-lg shadow-xl w-full max-w-md mx-4"
        onClick={(e) => e.stopPropagation()}
      >
        <form onSubmit={handleSubmit}>
          {/* Header */}
          <div className="flex items-center justify-between px-4 py-3 border-b border-border-primary">
            <h3 className="text-sm font-medium text-text-primary">Run Job</h3>
            <button
              type="button"
              onClick={onClose}
              className="text-text-secondary hover:text-text-primary transition-colors"
            >
              <X size={16} />
            </button>
          </div>

          {/* Body */}
          <div className="px-4 py-4 space-y-3">
            {hasParams ? (
              <>
                <p className="text-xs text-text-secondary">
                  Override parameter values for this run, or keep the defaults.
                </p>
                {entries.map(([key, value]) => (
                  <div key={key}>
                    <label className="block text-xs text-text-secondary mb-1 font-mono">
                      {key}
                    </label>
                    <input
                      type="text"
                      value={value}
                      onChange={(e) => handleValueChange(key, e.target.value)}
                      className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
                    />
                  </div>
                ))}
              </>
            ) : (
              <p className="text-sm text-text-secondary">
                Run this job now?
              </p>
            )}
          </div>

          {/* Footer */}
          <div className="flex items-center justify-end gap-2 px-4 py-3 border-t border-border-primary">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 text-xs rounded-md text-text-secondary hover:text-text-primary transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-green-600 hover:bg-green-600/80 text-white transition-colors"
            >
              <Play size={14} />
              Run
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
