interface JobSettingsPanelProps {
  timeoutSeconds: number;
  maxConcurrentRuns: number;
  onTimeoutChange: (value: number) => void;
  onMaxConcurrentChange: (value: number) => void;
}

export function JobSettingsPanel({
  timeoutSeconds,
  maxConcurrentRuns,
  onTimeoutChange,
  onMaxConcurrentChange,
}: JobSettingsPanelProps) {
  return (
    <div className="p-6 max-w-lg space-y-6">
      <h3 className="text-sm font-medium text-text-primary">Job Settings</h3>

      <div>
        <label htmlFor="job-timeout" className="block text-xs font-medium text-text-secondary mb-1">
          Job Timeout (seconds)
        </label>
        <input
          id="job-timeout"
          type="number"
          min={0}
          max={86400}
          value={timeoutSeconds}
          onChange={(e) => onTimeoutChange(clampInt(e.target.value, 0, 86400))}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
        />
        <p className="text-[10px] text-text-secondary/70 mt-0.5">
          Maximum time for the entire job run. 0 means no timeout.
        </p>
      </div>

      <div>
        <label htmlFor="job-max-concurrent" className="block text-xs font-medium text-text-secondary mb-1">
          Max Concurrent Runs
        </label>
        <input
          id="job-max-concurrent"
          type="number"
          min={0}
          max={100}
          value={maxConcurrentRuns}
          onChange={(e) => onMaxConcurrentChange(clampInt(e.target.value, 0, 100))}
          className="w-full px-3 py-1.5 text-sm rounded-md bg-bg-tertiary border border-border-primary text-text-primary focus:outline-none focus:border-accent-primary"
        />
        <p className="text-[10px] text-text-secondary/70 mt-0.5">
          Maximum number of concurrent runs for this job. 0 means unlimited.
        </p>
      </div>
    </div>
  );
}

function clampInt(raw: string, min: number, max: number): number {
  const n = parseInt(raw, 10);
  if (isNaN(n)) return min;
  return Math.max(min, Math.min(max, n));
}
