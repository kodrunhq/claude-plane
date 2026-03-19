import { describe, it, expect, vi } from 'vitest';
import { renderWithProviders, screen } from '../../../test/render.tsx';
import { RunNowModal } from '../../../components/jobs/RunNowModal.tsx';
import type { Task } from '../../../types/job.ts';

describe('RunNowModal', () => {
  const defaultProps = {
    defaultParameters: {},
    steps: [{ step_id: 's1', job_id: 'j1', name: 'Task 1', prompt: 'do stuff', machine_id: 'm1', working_dir: '/', command: 'claude', args: '', task_type: 'claude' }] as Task[],
    onRun: vi.fn(),
    onClose: vi.fn(),
  };

  function renderModal(overrides?: Partial<typeof defaultProps>) {
    const props = { ...defaultProps, ...overrides };
    // Reset mocks between renders within the same test if needed
    return renderWithProviders(<RunNowModal {...props} />);
  }

  it('renders modal with Run Job title', () => {
    renderModal();
    expect(screen.getByText('Run Job')).toBeInTheDocument();
  });

  it('shows "Run this job now?" when no parameters', () => {
    renderModal({ defaultParameters: {} });
    expect(screen.getByText('Run this job now?')).toBeInTheDocument();
  });

  it('renders parameter inputs when defaultParameters are provided', () => {
    renderModal({ defaultParameters: { BRANCH: 'main', ENV: 'staging' } });
    expect(screen.getByText('BRANCH')).toBeInTheDocument();
    expect(screen.getByText('ENV')).toBeInTheDocument();
    expect(screen.getByDisplayValue('main')).toBeInTheDocument();
    expect(screen.getByDisplayValue('staging')).toBeInTheDocument();
  });

  it('shows override instruction text when parameters exist', () => {
    renderModal({ defaultParameters: { KEY: 'value' } });
    expect(screen.getByText(/Override parameter values for this run/)).toBeInTheDocument();
  });

  it('does not show override text when no parameters', () => {
    renderModal({ defaultParameters: {} });
    expect(screen.queryByText(/Override parameter values for this run/)).not.toBeInTheDocument();
  });

  it('parameter input value changes on user input', async () => {
    const { user } = renderModal({ defaultParameters: { BRANCH: 'main' } });
    const input = screen.getByDisplayValue('main');
    await user.clear(input);
    await user.type(input, 'develop');
    expect(input).toHaveValue('develop');
  });

  it('Run button calls onRun with overrides on submit', async () => {
    const onRun = vi.fn();
    const { user } = renderModal({ defaultParameters: { BRANCH: 'main' }, onRun });

    await user.click(screen.getByText('Run'));
    expect(onRun).toHaveBeenCalledWith({ BRANCH: 'main' });
  });

  it('Run button calls onRun with updated override values', async () => {
    const onRun = vi.fn();
    const { user } = renderModal({ defaultParameters: { BRANCH: 'main' }, onRun });

    const input = screen.getByDisplayValue('main');
    await user.clear(input);
    await user.type(input, 'feature-x');
    await user.click(screen.getByText('Run'));

    expect(onRun).toHaveBeenCalledWith({ BRANCH: 'feature-x' });
  });

  it('Cancel button calls onClose', async () => {
    const onClose = vi.fn();
    const { user } = renderModal({ onClose });

    await user.click(screen.getByText('Cancel'));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('X button calls onClose', async () => {
    const onClose = vi.fn();
    const { user } = renderModal({ onClose });

    // The X button is the first button with type="button" in the header
    const buttons = screen.getAllByRole('button');
    // X button is the first one (in the header)
    const xButton = buttons.find((b) => b.getAttribute('type') === 'button' && b.closest('.border-b'));
    expect(xButton).toBeTruthy();
    await user.click(xButton!);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('clicking backdrop overlay calls onClose', async () => {
    const onClose = vi.fn();
    const { user } = renderModal({ onClose });

    // The overlay is the outer fixed div
    const overlay = screen.getByText('Run Job').closest('.fixed');
    expect(overlay).toBeTruthy();
    await user.click(overlay!);
    expect(onClose).toHaveBeenCalled();
  });

  it('shows empty prompt warning when a claude step has no prompt', () => {
    const steps: Task[] = [
      { step_id: 's1', job_id: 'j1', name: 'Task 1', prompt: '', machine_id: 'm1', working_dir: '/', command: 'claude', args: '', task_type: 'claude' },
    ];
    renderModal({ steps });
    expect(screen.getByText(/One or more Claude tasks have an empty prompt/)).toBeInTheDocument();
  });

  it('does not show empty prompt warning when all prompts are filled', () => {
    const steps: Task[] = [
      { step_id: 's1', job_id: 'j1', name: 'Task 1', prompt: 'analyze', machine_id: 'm1', working_dir: '/', command: 'claude', args: '', task_type: 'claude' },
    ];
    renderModal({ steps });
    expect(screen.queryByText(/One or more Claude tasks have an empty prompt/)).not.toBeInTheDocument();
  });

  it('does not show empty prompt warning for shell tasks with empty prompt', () => {
    const steps: Task[] = [
      { step_id: 's1', job_id: 'j1', name: 'Lint', prompt: '', machine_id: 'm1', working_dir: '/', command: 'npm', args: 'run lint', task_type: 'shell' },
    ];
    renderModal({ steps });
    expect(screen.queryByText(/One or more Claude tasks have an empty prompt/)).not.toBeInTheDocument();
  });

  it('does not call onRun when steps array is empty (shows toast)', async () => {
    const onRun = vi.fn();
    const { user } = renderModal({ steps: [], onRun });

    await user.click(screen.getByText('Run'));
    expect(onRun).not.toHaveBeenCalled();
  });

  it('calls onRun with empty overrides when no parameters and steps exist', async () => {
    const onRun = vi.fn();
    const { user } = renderModal({ defaultParameters: {}, onRun });

    await user.click(screen.getByText('Run'));
    expect(onRun).toHaveBeenCalledWith({});
  });
});
