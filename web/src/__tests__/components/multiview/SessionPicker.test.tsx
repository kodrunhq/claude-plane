import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../../test/render.tsx';
import { SessionPicker } from '../../../components/multiview/SessionPicker.tsx';
import { buildSession, buildMachine } from '../../../test/factories.ts';

const runningSessions = [
  buildSession({ session_id: 'sess-run-1', status: 'running', machine_id: 'machine-a', command: 'claude', working_dir: '/home/user/project' }),
  buildSession({ session_id: 'sess-run-2', status: 'running', machine_id: 'machine-b', command: 'bash', working_dir: '/tmp' }),
];

const machines = [
  buildMachine({ machine_id: 'machine-a', display_name: 'Alpha', status: 'connected' }),
  buildMachine({ machine_id: 'machine-b', display_name: 'Beta', status: 'connected' }),
];

function setupHandlers() {
  server.use(
    http.get('/api/v1/sessions', () => HttpResponse.json(runningSessions)),
    http.get('/api/v1/machines', () => HttpResponse.json(machines)),
  );
}

describe('SessionPicker', () => {
  it('renders running sessions and they are clickable', async () => {
    setupHandlers();
    const onSelect = vi.fn();
    const onClose = vi.fn();

    const { user } = renderWithProviders(
      <SessionPicker onSelect={onSelect} onClose={onClose} />,
    );

    // Wait for sessions to load — session_id.slice(0,8) is shown
    await waitFor(() => {
      expect(screen.getAllByText('sess-run').length).toBeGreaterThanOrEqual(2);
    });

    // Both sessions should be visible
    const sessionLabels = screen.getAllByText('sess-run');
    expect(sessionLabels).toHaveLength(2);

    // Find the first session button and click it
    const firstSessionButton = sessionLabels[0].closest('button');
    expect(firstSessionButton).toBeTruthy();

    await user.click(firstSessionButton!);
    expect(onSelect).toHaveBeenCalledWith('sess-run-1');
  });

  it('shows "No running sessions found" when no sessions available', async () => {
    server.use(
      http.get('/api/v1/sessions', () => HttpResponse.json([])),
      http.get('/api/v1/machines', () => HttpResponse.json([])),
    );

    const onSelect = vi.fn();
    const onClose = vi.fn();

    renderWithProviders(
      <SessionPicker onSelect={onSelect} onClose={onClose} />,
    );

    await waitFor(() => {
      expect(screen.getByText('No running sessions found')).toBeInTheDocument();
    });
  });

  it('populates the machine filter dropdown', async () => {
    setupHandlers();
    const onSelect = vi.fn();
    const onClose = vi.fn();

    renderWithProviders(
      <SessionPicker onSelect={onSelect} onClose={onClose} />,
    );

    await waitFor(() => {
      expect(screen.getByText('Alpha')).toBeInTheDocument();
    });

    expect(screen.getByText('Beta')).toBeInTheDocument();
    expect(screen.getByDisplayValue('All machines')).toBeInTheDocument();
  });

  it('excludes sessions listed in excludeSessionIds', async () => {
    setupHandlers();
    const onSelect = vi.fn();
    const onClose = vi.fn();

    const { user } = renderWithProviders(
      <SessionPicker
        onSelect={onSelect}
        onClose={onClose}
        excludeSessionIds={['sess-run-1']}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText('Already in view')).toBeInTheDocument();
    });

    // The excluded session's button should be disabled
    const excludedButton = screen.getByText('Already in view').closest('button');
    expect(excludedButton).toBeDisabled();

    // Clicking the excluded session should not trigger onSelect
    await user.click(excludedButton!);
    expect(onSelect).not.toHaveBeenCalled();
  });

  it('calls onClose when backdrop is clicked', async () => {
    setupHandlers();
    const onSelect = vi.fn();
    const onClose = vi.fn();

    const { user } = renderWithProviders(
      <SessionPicker onSelect={onSelect} onClose={onClose} />,
    );

    // Click the backdrop (outermost div)
    const backdrop = screen.getByRole('dialog').parentElement!;
    await user.click(backdrop);
    expect(onClose).toHaveBeenCalled();
  });
});
