import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen } from '@testing-library/react';
import { RunFilters } from '../../../components/runs/RunFilters';
import { renderWithProviders } from '../../../test/render';
import { buildJob } from '../../../test/factories';
import type { Job } from '../../../types/job';

/** Find a <select> by looking for a <label> with the given text and grabbing the sibling select. */
function getSelectByLabel(labelText: string): HTMLSelectElement {
  const label = screen.getByText(labelText, { selector: 'label' });
  const select = label.parentElement?.querySelector('select');
  if (!select) throw new Error(`No select found for label "${labelText}"`);
  return select as HTMLSelectElement;
}

const defaultProps = {
  jobs: [] as Job[],
  selectedJobId: 'all',
  selectedStatus: 'all',
  selectedTriggerType: 'all',
  onJobChange: vi.fn(),
  onStatusChange: vi.fn(),
  onTriggerTypeChange: vi.fn(),
};

describe('RunFilters', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders Job, Status, and Trigger filter labels', () => {
    renderWithProviders(<RunFilters {...defaultProps} />);
    expect(screen.getByText('Job')).toBeInTheDocument();
    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByText('Trigger')).toBeInTheDocument();
  });

  it('renders job select with All Jobs option by default', () => {
    renderWithProviders(<RunFilters {...defaultProps} />);
    expect(screen.getByText('All Jobs')).toBeInTheDocument();
  });

  it('renders job options from the jobs prop', () => {
    const jobs = [
      buildJob({ job_id: 'j1', name: 'Deploy Prod' }),
      buildJob({ job_id: 'j2', name: 'Run Tests' }),
    ];
    renderWithProviders(<RunFilters {...defaultProps} jobs={jobs} />);
    expect(screen.getByText('Deploy Prod')).toBeInTheDocument();
    expect(screen.getByText('Run Tests')).toBeInTheDocument();
  });

  it('renders all status options', () => {
    renderWithProviders(<RunFilters {...defaultProps} />);
    const statusSelect = getSelectByLabel('Status');
    const options = Array.from(statusSelect.options).map((o) => o.text);
    expect(options).toEqual([
      'All Statuses', 'Pending', 'Running', 'Completed', 'Failed', 'Cancelled',
    ]);
  });

  it('renders all trigger options', () => {
    renderWithProviders(<RunFilters {...defaultProps} />);
    const triggerSelect = getSelectByLabel('Trigger');
    const options = Array.from(triggerSelect.options).map((o) => o.text);
    expect(options).toEqual(['All Triggers', 'Manual', 'Scheduled']);
  });

  it('reflects selected job value', () => {
    const jobs = [buildJob({ job_id: 'j1', name: 'My Job' })];
    renderWithProviders(
      <RunFilters {...defaultProps} jobs={jobs} selectedJobId="j1" />,
    );
    const jobSelect = getSelectByLabel('Job');
    expect(jobSelect.value).toBe('j1');
  });

  it('reflects selected status value', () => {
    renderWithProviders(
      <RunFilters {...defaultProps} selectedStatus="failed" />,
    );
    const statusSelect = getSelectByLabel('Status');
    expect(statusSelect.value).toBe('failed');
  });

  it('reflects selected trigger type value', () => {
    renderWithProviders(
      <RunFilters {...defaultProps} selectedTriggerType="manual" />,
    );
    const triggerSelect = getSelectByLabel('Trigger');
    expect(triggerSelect.value).toBe('manual');
  });

  it('calls onJobChange when job select changes', async () => {
    const onJobChange = vi.fn();
    const jobs = [buildJob({ job_id: 'j1', name: 'Deploy' })];
    const { user } = renderWithProviders(
      <RunFilters {...defaultProps} jobs={jobs} onJobChange={onJobChange} />,
    );
    await user.selectOptions(getSelectByLabel('Job'), 'j1');
    expect(onJobChange).toHaveBeenCalledWith('j1');
  });

  it('calls onStatusChange when status select changes', async () => {
    const onStatusChange = vi.fn();
    const { user } = renderWithProviders(
      <RunFilters {...defaultProps} onStatusChange={onStatusChange} />,
    );
    await user.selectOptions(getSelectByLabel('Status'), 'running');
    expect(onStatusChange).toHaveBeenCalledWith('running');
  });

  it('calls onTriggerTypeChange when trigger select changes', async () => {
    const onTriggerTypeChange = vi.fn();
    const { user } = renderWithProviders(
      <RunFilters {...defaultProps} onTriggerTypeChange={onTriggerTypeChange} />,
    );
    await user.selectOptions(getSelectByLabel('Trigger'), 'scheduled');
    expect(onTriggerTypeChange).toHaveBeenCalledWith('scheduled');
  });

  it('calls onJobChange with "all" when All Jobs is selected', async () => {
    const onJobChange = vi.fn();
    const jobs = [buildJob({ job_id: 'j1', name: 'Deploy' })];
    const { user } = renderWithProviders(
      <RunFilters
        {...defaultProps}
        jobs={jobs}
        selectedJobId="j1"
        onJobChange={onJobChange}
      />,
    );
    await user.selectOptions(getSelectByLabel('Job'), 'all');
    expect(onJobChange).toHaveBeenCalledWith('all');
  });

  it('calls onStatusChange with "all" when All Statuses is selected', async () => {
    const onStatusChange = vi.fn();
    const { user } = renderWithProviders(
      <RunFilters
        {...defaultProps}
        selectedStatus="running"
        onStatusChange={onStatusChange}
      />,
    );
    await user.selectOptions(getSelectByLabel('Status'), 'all');
    expect(onStatusChange).toHaveBeenCalledWith('all');
  });

  it('renders empty job list with only All Jobs option', () => {
    renderWithProviders(<RunFilters {...defaultProps} jobs={[]} />);
    const jobSelect = getSelectByLabel('Job');
    expect(jobSelect.options).toHaveLength(1);
    expect(jobSelect.options[0].text).toBe('All Jobs');
  });
});
