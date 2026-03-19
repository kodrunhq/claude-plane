import { describe, it, expect, vi } from 'vitest';
import { screen, within } from '@testing-library/react';
import { TaskEditor } from '../../../components/jobs/TaskEditor';
import { renderWithProviders } from '../../../test/render';
import { buildMachine } from '../../../test/factories';
import type { Task } from '../../../types/job';

function buildTask(overrides?: Partial<Task>): Task {
  return {
    step_id: 'step-1',
    job_id: 'job-1',
    name: 'Test Task',
    prompt: 'Do something',
    machine_id: 'machine-1',
    working_dir: '/home/user',
    command: 'claude',
    args: '',
    task_type: 'claude_session',
    ...overrides,
  };
}

const machines = [
  buildMachine({ machine_id: 'machine-1', display_name: 'Ubuntu NUC Worker' }),
  buildMachine({ machine_id: 'machine-2', display_name: 'ARM Builder' }),
];

function renderEditor(task: Task, props?: Record<string, unknown>) {
  return renderWithProviders(
    <TaskEditor
      task={task}
      machines={machines}
      onSave={vi.fn()}
      onDelete={vi.fn()}
      {...props}
    />,
  );
}

describe('TaskEditor', () => {
  // Shell tasks require a Command field (the binary to execute).
  // The backend validates that shell steps have a non-empty Command.
  it('shows Command field when task type is shell', async () => {
    const task = buildTask({ task_type: 'shell', command: '/usr/bin/make' });
    const { user } = renderEditor(task);

    // Switch to Shell tab to make sure we're in shell mode
    const shellButton = screen.getByRole('button', { name: 'Shell' });
    await user.click(shellButton);

    // Command input should exist for shell tasks
    const commandInput = document.getElementById('task-command') as HTMLInputElement;
    expect(commandInput).not.toBeNull();
    expect(commandInput.value).toBe('/usr/bin/make');
  });

  it('shows Command field when task type is claude', () => {
    const task = buildTask({ task_type: 'claude_session' });
    renderEditor(task);

    expect(document.getElementById('task-command')).not.toBeNull();
  });

  // ── Run Job task type shows Target Job, not Command ───────────────────────
  it('shows Target Job field for run_job task type', async () => {
    const task = buildTask({ task_type: 'run_job', command: '' });
    const { user } = renderEditor(task);

    const runJobButton = screen.getByRole('button', { name: 'Run Job' });
    await user.click(runJobButton);

    expect(screen.getByLabelText(/Target Job/)).toBeDefined();
    expect(document.getElementById('task-command')).toBeNull();
  });

  // ── Working directory browse button ───────────────────────────────────────
  it('shows a browse button (FolderOpen icon) next to working directory for claude type', () => {
    const task = buildTask({ task_type: 'claude_session' });
    renderEditor(task);

    const browseButton = screen.getByTitle('Browse directories');
    expect(browseButton).toBeDefined();
    // The button should contain an SVG (FolderOpen icon)
    expect(browseButton.querySelector('svg')).not.toBeNull();
  });

  it('shows a browse button next to working directory for shell type', async () => {
    const task = buildTask({ task_type: 'shell' });
    const { user } = renderEditor(task);

    const shellButton = screen.getByRole('button', { name: 'Shell' });
    await user.click(shellButton);

    const browseButton = screen.getByTitle('Browse directories');
    expect(browseButton).toBeDefined();
  });

  // ── Machine dropdown shows full machine names (not truncated) ─────────────
  it('shows full machine names in dropdown options', () => {
    const task = buildTask();
    renderEditor(task);

    const select = screen.getByLabelText('Machine') as HTMLSelectElement;
    const options = within(select).getAllByRole('option');

    // Should contain both full display names (plus the placeholder)
    const optionTexts = options.map((o) => o.textContent);
    expect(optionTexts).toContain('Ubuntu NUC Worker');
    expect(optionTexts).toContain('ARM Builder');
  });

  // ── Save button submits form ──────────────────────────────────────────────
  it('calls onSave with task step_id when Save button is clicked', async () => {
    const onSave = vi.fn();
    const task = buildTask({ step_id: 'step-42' });
    const { user } = renderWithProviders(
      <TaskEditor
        task={task}
        machines={machines}
        onSave={onSave}
        onDelete={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Save Task' }));
    expect(onSave).toHaveBeenCalledOnce();
    expect(onSave.mock.calls[0][0]).toBe('step-42');
  });

  // ── Delete button calls onDelete ──────────────────────────────────────────
  it('calls onDelete with task step_id when Delete button is clicked', async () => {
    const onDelete = vi.fn();
    const task = buildTask({ step_id: 'step-99' });
    const { user } = renderWithProviders(
      <TaskEditor
        task={task}
        machines={machines}
        onSave={vi.fn()}
        onDelete={onDelete}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Delete' }));
    expect(onDelete).toHaveBeenCalledWith('step-99');
  });
});
