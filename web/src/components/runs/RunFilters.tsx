import type { Job } from '../../types/job.ts';

interface RunFiltersProps {
  jobs: Job[];
  selectedJobId: string;
  selectedStatus: string;
  selectedTriggerType: string;
  onJobChange: (jobId: string) => void;
  onStatusChange: (status: string) => void;
  onTriggerTypeChange: (type: string) => void;
}

const STATUS_OPTIONS = [
  { value: 'all', label: 'All Statuses' },
  { value: 'pending', label: 'Pending' },
  { value: 'running', label: 'Running' },
  { value: 'completed', label: 'Completed' },
  { value: 'failed', label: 'Failed' },
  { value: 'cancelled', label: 'Cancelled' },
];

const TRIGGER_OPTIONS = [
  { value: 'all', label: 'All Triggers' },
  { value: 'manual', label: 'Manual' },
  { value: 'scheduled', label: 'Scheduled' },
];

const SELECT_CLASS =
  'rounded-md bg-bg-tertiary border border-gray-600 text-text-primary text-sm px-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-primary';

export function RunFilters({
  jobs,
  selectedJobId,
  selectedStatus,
  selectedTriggerType,
  onJobChange,
  onStatusChange,
  onTriggerTypeChange,
}: RunFiltersProps) {
  return (
    <div className="flex items-center gap-4">
      <div>
        <label className="block text-xs text-text-secondary mb-1">Job</label>
        <select value={selectedJobId} onChange={(e) => onJobChange(e.target.value)} className={SELECT_CLASS}>
          <option value="all">All Jobs</option>
          {jobs.map((job) => (
            <option key={job.job_id} value={job.job_id}>
              {job.name}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label className="block text-xs text-text-secondary mb-1">Status</label>
        <select value={selectedStatus} onChange={(e) => onStatusChange(e.target.value)} className={SELECT_CLASS}>
          {STATUS_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label className="block text-xs text-text-secondary mb-1">Trigger</label>
        <select value={selectedTriggerType} onChange={(e) => onTriggerTypeChange(e.target.value)} className={SELECT_CLASS}>
          {TRIGGER_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>
    </div>
  );
}
