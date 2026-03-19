import { describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { SessionsPage } from '../../views/SessionsPage.tsx';
import { buildSession, buildMachine } from '../../test/factories.ts';
import { mockMachines } from '../../test/handlers.ts';

const runningSessions = [
  buildSession({ session_id: 'aaaaaaaa-0001-0001-0001-000000000001', status: 'running', machine_id: 'machine-100', command: 'claude', working_dir: '/home/user/project-alpha' }),
  buildSession({ session_id: 'bbbbbbbb-0002-0002-0002-000000000002', status: 'running', machine_id: 'machine-100', command: 'claude', working_dir: '/home/user/project-beta' }),
  buildSession({ session_id: 'cccccccc-0003-0003-0003-000000000003', status: 'running', machine_id: 'machine-101', command: 'bash', working_dir: '/tmp' }),
];

describe('SessionsPage interactions', () => {
  it('Multi-View toggle button enables selection mode with checkboxes', async () => {
    server.use(
      http.get('/api/v1/sessions', () => HttpResponse.json(runningSessions)),
    );

    const { user } = renderWithProviders(<SessionsPage />);

    // Wait for sessions to render
    await waitFor(() => {
      expect(screen.getByText(runningSessions[0].session_id.slice(0, 8))).toBeInTheDocument();
    });

    // No checkboxes initially
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0);

    // Click Multi-View button
    await user.click(screen.getByText('Multi-View'));

    // Checkboxes should now appear on each session card
    const checkboxes = screen.getAllByRole('checkbox');
    expect(checkboxes.length).toBe(runningSessions.length);
  });

  it('selecting 2+ sessions shows "Open in Multi-View" floating bar', async () => {
    server.use(
      http.get('/api/v1/sessions', () => HttpResponse.json(runningSessions)),
    );

    const { user } = renderWithProviders(<SessionsPage />);

    await waitFor(() => {
      expect(screen.getByText(runningSessions[0].session_id.slice(0, 8))).toBeInTheDocument();
    });

    // Enter multi-select mode
    await user.click(screen.getByText('Multi-View'));

    // Select first session — floating bar should NOT appear yet (need 2+)
    const firstCheckbox = screen.getByLabelText(`Select session ${runningSessions[0].session_id.slice(0, 8)}`);
    await user.click(firstCheckbox);
    expect(screen.queryByText('Open in Multi-View')).not.toBeInTheDocument();

    // Select second session — floating bar SHOULD appear
    const secondCheckbox = screen.getByLabelText(`Select session ${runningSessions[1].session_id.slice(0, 8)}`);
    await user.click(secondCheckbox);

    expect(screen.getByText('Open in Multi-View')).toBeInTheDocument();
    expect(screen.getByText('2 sessions selected')).toBeInTheDocument();
  });

  it('status filter dropdown filters sessions', async () => {
    // Return different sessions for different status queries
    server.use(
      http.get('/api/v1/sessions', ({ request }) => {
        const url = new URL(request.url);
        const status = url.searchParams.get('status');
        if (status === 'completed') {
          return HttpResponse.json([
            buildSession({ session_id: 'dddddddd-done-done-done-000000000004', status: 'completed', command: 'claude' }),
          ]);
        }
        return HttpResponse.json(runningSessions);
      }),
    );

    const { user } = renderWithProviders(<SessionsPage />);

    // Wait for initial running sessions
    await waitFor(() => {
      expect(screen.getByText(runningSessions[0].session_id.slice(0, 8))).toBeInTheDocument();
    });

    // Change status filter to "All Statuses"
    const statusLabel = screen.getByText('Status');
    const statusSelect = statusLabel.parentElement!.querySelector('select')!;
    await user.selectOptions(statusSelect, 'completed');

    // The completed session should appear after the filter change triggers a new fetch
    await waitFor(() => {
      expect(screen.getByText('dddddddd')).toBeInTheDocument();
    });
  });

  it('machine filter dropdown shows machine names', async () => {
    server.use(
      http.get('/api/v1/sessions', () => HttpResponse.json(runningSessions)),
    );

    renderWithProviders(<SessionsPage />);

    // Wait for machines to load in the filter dropdown
    await waitFor(() => {
      const machineSelect = screen.getByDisplayValue('All Machines');
      const options = machineSelect.querySelectorAll('option');
      const optionTexts = Array.from(options).map((opt) => opt.textContent);

      // Should show "All Machines" as first option
      expect(optionTexts).toContain('All Machines');

      // Should show machine display names from the mock machines
      const connectedMachine = mockMachines.find((m) => m.status === 'connected')!;
      expect(optionTexts).toContain(connectedMachine.display_name);
    });
  });

  it('New Session button opens modal', async () => {
    server.use(
      http.get('/api/v1/sessions', () => HttpResponse.json(runningSessions)),
    );

    const { user } = renderWithProviders(<SessionsPage />);

    // Modal should not be visible initially
    expect(screen.queryByText('Session Type')).not.toBeInTheDocument();

    // Click New Session button
    await user.click(screen.getByText('New Session'));

    // Modal should now be visible with its form fields
    expect(screen.getByText('Session Type')).toBeInTheDocument();
    expect(screen.getByText('Create Session')).toBeInTheDocument();
  });

  it('search input filters sessions by text', async () => {
    server.use(
      http.get('/api/v1/sessions', () => HttpResponse.json(runningSessions)),
    );

    const { user } = renderWithProviders(<SessionsPage />);

    // Wait for sessions to load
    await waitFor(() => {
      expect(screen.getByText(runningSessions[0].session_id.slice(0, 8))).toBeInTheDocument();
    });

    // All 3 sessions should be visible
    for (const session of runningSessions) {
      expect(screen.getByText(session.session_id.slice(0, 8))).toBeInTheDocument();
    }

    // Type in the search box to filter by working directory
    const searchInput = screen.getByPlaceholderText('Search sessions...');
    await user.type(searchInput, 'project-alpha');

    // Only the first session (with working_dir containing "project-alpha") should remain
    await waitFor(() => {
      expect(screen.getByText(runningSessions[0].session_id.slice(0, 8))).toBeInTheDocument();
      expect(screen.queryByText(runningSessions[1].session_id.slice(0, 8))).not.toBeInTheDocument();
      expect(screen.queryByText(runningSessions[2].session_id.slice(0, 8))).not.toBeInTheDocument();
    });
  });

  it('search input filters sessions by command', async () => {
    server.use(
      http.get('/api/v1/sessions', () => HttpResponse.json(runningSessions)),
    );

    const { user } = renderWithProviders(<SessionsPage />);

    await waitFor(() => {
      expect(screen.getByText(runningSessions[0].session_id.slice(0, 8))).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText('Search sessions...');
    await user.type(searchInput, 'bash');

    // Only the terminal session (command: 'bash') should remain
    await waitFor(() => {
      expect(screen.getByText(runningSessions[2].session_id.slice(0, 8))).toBeInTheDocument();
      expect(screen.queryByText(runningSessions[0].session_id.slice(0, 8))).not.toBeInTheDocument();
    });
  });

  it('toggling Multi-View off clears selected sessions', async () => {
    server.use(
      http.get('/api/v1/sessions', () => HttpResponse.json(runningSessions)),
    );

    const { user } = renderWithProviders(<SessionsPage />);

    await waitFor(() => {
      expect(screen.getByText(runningSessions[0].session_id.slice(0, 8))).toBeInTheDocument();
    });

    // Enter multi-select mode and select a session
    await user.click(screen.getByText('Multi-View'));
    const checkbox = screen.getByLabelText(`Select session ${runningSessions[0].session_id.slice(0, 8)}`);
    await user.click(checkbox);
    expect(checkbox).toBeChecked();

    // Toggle multi-select mode off
    await user.click(screen.getByText('Multi-View'));

    // Checkboxes should disappear (selection cleared)
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0);

    // Re-enter multi-select mode — previous selection should be cleared
    await user.click(screen.getByText('Multi-View'));
    const checkboxes = screen.getAllByRole('checkbox');
    for (const cb of checkboxes) {
      expect(cb).not.toBeChecked();
    }
  });
});
