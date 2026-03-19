import { screen } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { RunsTable } from '../../../components/runs/RunsTable.tsx';
import { renderWithProviders } from '../../../test/render.tsx';
import { buildRun } from '../../../test/factories.ts';
import type { Run } from '../../../types/job.ts';

function renderTable(
  runs: Run[],
  overrides?: { showJobName?: boolean; compact?: boolean; onRowClick?: (id: string) => void },
) {
  const onRowClick = overrides?.onRowClick ?? vi.fn();
  return {
    onRowClick,
    ...renderWithProviders(
      <RunsTable
        runs={runs}
        showJobName={overrides?.showJobName}
        compact={overrides?.compact}
        onRowClick={onRowClick}
      />,
    ),
  };
}

describe('RunsTable', () => {
  describe('empty state', () => {
    it('renders empty state when no runs', () => {
      renderTable([]);
      expect(screen.getByText('No runs found')).toBeInTheDocument();
      expect(screen.getByText('No runs match the current filters.')).toBeInTheDocument();
    });

    it('does not render a table when empty', () => {
      renderTable([]);
      expect(screen.queryByRole('table')).not.toBeInTheDocument();
    });
  });

  describe('table rendering', () => {
    it('renders a table when runs are present', () => {
      renderTable([buildRun()]);
      expect(screen.getByRole('table')).toBeInTheDocument();
    });

    it('renders a row for each run', () => {
      const runs = [buildRun(), buildRun(), buildRun()];
      renderTable(runs);
      const rows = screen.getAllByRole('row');
      // +1 for header row
      expect(rows).toHaveLength(runs.length + 1);
    });
  });

  describe('row data', () => {
    it('renders the status badge in each row', () => {
      const run = buildRun({ status: 'running' });
      renderTable([run]);
      expect(screen.getByText('running')).toBeInTheDocument();
    });

    it('renders the trigger type', () => {
      const run = buildRun({ trigger_type: 'cron' });
      renderTable([run]);
      expect(screen.getByText('cron')).toBeInTheDocument();
    });

    it('defaults trigger type to "manual"', () => {
      const run = buildRun({ trigger_type: undefined });
      renderTable([run]);
      expect(screen.getByText('manual')).toBeInTheDocument();
    });

    it('displays full machine names (not truncated)', () => {
      const run = buildRun({ machine_ids: 'production-worker-very-long-name' });
      renderTable([run]);
      expect(screen.getByText('production-worker-very-long-name')).toBeInTheDocument();
    });

    it('formats multiple machine IDs as first + count', () => {
      const run = buildRun({ machine_ids: 'worker-1,worker-2,worker-3' });
      renderTable([run]);
      expect(screen.getByText('worker-1 +2')).toBeInTheDocument();
    });

    it('shows dash when machine_ids is undefined', () => {
      const run = buildRun({ machine_ids: undefined });
      renderTable([run]);
      // The dash character
      const cells = screen.getAllByRole('cell');
      const machineCell = cells.find((c) => c.textContent === '\u2014');
      expect(machineCell).toBeTruthy();
    });
  });

  describe('row click navigation', () => {
    it('calls onRowClick with run_id when row is clicked', async () => {
      const run = buildRun({ run_id: 'run-abc-123' });
      const { onRowClick, user } = renderTable([run]);

      const row = screen.getByRole('row', { name: /open run/i });
      await user.click(row);
      expect(onRowClick).toHaveBeenCalledWith('run-abc-123');
    });

    it('calls onRowClick on Enter keypress', async () => {
      const run = buildRun({ run_id: 'run-key-enter' });
      const { onRowClick, user } = renderTable([run]);

      const row = screen.getByRole('row', { name: /open run/i });
      row.focus();
      await user.keyboard('{Enter}');
      expect(onRowClick).toHaveBeenCalledWith('run-key-enter');
    });

    it('calls onRowClick on Space keypress', async () => {
      const run = buildRun({ run_id: 'run-key-space' });
      const { onRowClick, user } = renderTable([run]);

      const row = screen.getByRole('row', { name: /open run/i });
      row.focus();
      await user.keyboard(' ');
      expect(onRowClick).toHaveBeenCalledWith('run-key-space');
    });

    it('rows are focusable (tabIndex=0)', () => {
      const run = buildRun();
      renderTable([run]);
      const row = screen.getByRole('row', { name: /open run/i });
      expect(row).toHaveAttribute('tabindex', '0');
    });
  });

  describe('showJobName option', () => {
    it('shows Job Name column header when showJobName is true', () => {
      renderTable([buildRun({ job_name: 'Deploy Job' })], { showJobName: true });
      expect(screen.getByText('Job Name')).toBeInTheDocument();
      expect(screen.getByText('Deploy Job')).toBeInTheDocument();
    });

    it('does not show Job Name column header when showJobName is false', () => {
      renderTable([buildRun({ job_name: 'Deploy Job' })], { showJobName: false });
      expect(screen.queryByText('Job Name')).not.toBeInTheDocument();
    });
  });

  describe('compact mode', () => {
    it('hides Run ID column in compact mode', () => {
      renderTable([buildRun()], { compact: true });
      expect(screen.queryByText('Run ID')).not.toBeInTheDocument();
    });

    it('hides Machine column in compact mode', () => {
      renderTable([buildRun()], { compact: true });
      expect(screen.queryByText('Machine')).not.toBeInTheDocument();
    });

    it('still shows Status, Trigger, Started, Duration in compact mode', () => {
      renderTable([buildRun()], { compact: true });
      expect(screen.getByText('Status')).toBeInTheDocument();
      expect(screen.getByText('Trigger')).toBeInTheDocument();
      expect(screen.getByText('Started')).toBeInTheDocument();
      expect(screen.getByText('Duration')).toBeInTheDocument();
    });
  });
});
