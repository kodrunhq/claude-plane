import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../../test/render.tsx';
import { NewSessionModal } from '../../../components/sessions/NewSessionModal.tsx';
import { mockMachines } from '../../../test/handlers.ts';
import { buildSession } from '../../../test/factories.ts';

describe('NewSessionModal', () => {
  it('renders nothing when closed', () => {
    renderWithProviders(
      <NewSessionModal open={false} onClose={() => {}} />,
    );

    // Modal uses createPortal and returns null when not open
    expect(screen.queryByText('New Session')).not.toBeInTheDocument();
  });

  it('renders modal with form fields when open', () => {
    renderWithProviders(
      <NewSessionModal open={true} onClose={() => {}} />,
    );

    expect(screen.getByText('New Session')).toBeInTheDocument();
    expect(screen.getByText('Session Type')).toBeInTheDocument();
    expect(screen.getByText('Machine')).toBeInTheDocument();
    expect(screen.getByText('Create Session')).toBeInTheDocument();
    expect(screen.getByText('Cancel')).toBeInTheDocument();
  });

  it('populates machine dropdown from API with online machines', async () => {
    renderWithProviders(
      <NewSessionModal open={true} onClose={() => {}} />,
    );

    const connectedMachine = mockMachines.find((m) => m.status === 'connected')!;

    await waitFor(() => {
      // The machine select has "Select a machine..." as default
      const machineSelect = screen.getByDisplayValue('Select a machine...');
      expect(machineSelect).toBeInTheDocument();

      // The connected machine should appear as an option
      const options = machineSelect.querySelectorAll('option');
      const machineOption = Array.from(options).find(
        (opt) => opt.textContent?.includes(connectedMachine.display_name!),
      );
      expect(machineOption).toBeTruthy();
    });
  });

  it('shows session type toggle with Claude and Terminal options', () => {
    renderWithProviders(
      <NewSessionModal open={true} onClose={() => {}} />,
    );

    expect(screen.getByText('Claude')).toBeInTheDocument();
    expect(screen.getByText('Terminal')).toBeInTheDocument();
  });

  it('shows Claude-specific fields by default', () => {
    renderWithProviders(
      <NewSessionModal open={true} onClose={() => {}} />,
    );

    // Claude mode shows Additional Arguments, Model, Skip Permissions fields
    expect(screen.getByText(/Additional Arguments/)).toBeInTheDocument();
    expect(screen.getByText(/Model/)).toBeInTheDocument();
    expect(screen.getByText(/Skip Permissions/)).toBeInTheDocument();
  });

  it('selecting Terminal type hides Claude-specific fields', async () => {
    const { user } = renderWithProviders(
      <NewSessionModal open={true} onClose={() => {}} />,
    );

    // Claude-specific fields are visible by default
    expect(screen.getByText(/Additional Arguments/)).toBeInTheDocument();
    expect(screen.getByText(/Model/)).toBeInTheDocument();
    expect(screen.getByText(/Skip Permissions/)).toBeInTheDocument();

    // Click the Terminal toggle button
    await user.click(screen.getByRole('button', { name: /Terminal/i }));

    // Claude-specific fields should now be hidden
    expect(screen.queryByText(/Additional Arguments/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Model/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Skip Permissions/)).not.toBeInTheDocument();
    // Template field should also be hidden
    expect(screen.queryByText('Template')).not.toBeInTheDocument();
  });

  it('machine dropdown shows full machine names (not truncated)', async () => {
    renderWithProviders(
      <NewSessionModal open={true} onClose={() => {}} />,
    );

    const connectedMachine = mockMachines.find((m) => m.status === 'connected')!;

    await waitFor(() => {
      const machineSelect = screen.getByDisplayValue('Select a machine...');
      const options = machineSelect.querySelectorAll('option');
      const machineOption = Array.from(options).find(
        (opt) => opt.textContent === connectedMachine.display_name,
      );
      // Verify the full name is displayed, not truncated
      expect(machineOption).toBeTruthy();
      expect(machineOption!.textContent).toBe(connectedMachine.display_name);
    });
  });

  it('working directory browse button is enabled when machine is selected', async () => {
    const { user } = renderWithProviders(
      <NewSessionModal open={true} onClose={() => {}} />,
    );

    await waitFor(() => {
      expect(screen.getByDisplayValue('Select a machine...')).toBeInTheDocument();
    });

    // Initially browse button should be disabled (no machine selected)
    const browseButton = screen.getByTitle('Browse directories');
    expect(browseButton).toBeDisabled();

    // Select a machine
    const connectedMachine = mockMachines.find((m) => m.status === 'connected')!;
    await waitFor(async () => {
      const machineSelect = screen.getByDisplayValue('Select a machine...');
      await user.selectOptions(machineSelect, connectedMachine.machine_id);
    });

    // Browse button should now be enabled
    expect(browseButton).toBeEnabled();
  });

  it('working directory browse button is disabled when no machine selected', () => {
    renderWithProviders(
      <NewSessionModal open={true} onClose={() => {}} />,
    );

    const browseButton = screen.getByTitle('Browse directories');
    expect(browseButton).toBeDisabled();
  });

  it('working directory browse button works for both claude and terminal types', async () => {
    const { user } = renderWithProviders(
      <NewSessionModal open={true} onClose={() => {}} />,
    );

    // Wait for machines to load, then select one
    const connectedMachine = mockMachines.find((m) => m.status === 'connected')!;
    await waitFor(async () => {
      const machineSelect = screen.getByDisplayValue('Select a machine...');
      await user.selectOptions(machineSelect, connectedMachine.machine_id);
    });

    // Browse button is present and enabled in Claude mode
    const browseButton = screen.getByTitle('Browse directories');
    expect(browseButton).toBeEnabled();

    // Switch to Terminal mode
    await user.click(screen.getByRole('button', { name: /Terminal/i }));

    // Browse button should still be present and enabled in Terminal mode
    const browseButtonAfter = screen.getByTitle('Browse directories');
    expect(browseButtonAfter).toBeEnabled();
  });

  it('form submission calls createSession.mutateAsync with correct params for claude type', async () => {
    const createdSession = buildSession({ session_id: 'new-sess-1', machine_id: 'machine-100' });
    let capturedBody: Record<string, unknown> | null = null;

    server.use(
      http.post('/api/v1/sessions', async ({ request }) => {
        capturedBody = await request.json() as Record<string, unknown>;
        return HttpResponse.json(createdSession);
      }),
    );

    const onClose = vi.fn();
    const { user } = renderWithProviders(
      <NewSessionModal open={true} onClose={onClose} />,
    );

    // Wait for machines to load
    const connectedMachine = mockMachines.find((m) => m.status === 'connected')!;
    await waitFor(async () => {
      const machineSelect = screen.getByDisplayValue('Select a machine...');
      await user.selectOptions(machineSelect, connectedMachine.machine_id);
    });

    // Fill in optional claude fields
    const modelSelect = screen.getByDisplayValue('Default');
    await user.selectOptions(modelSelect, 'sonnet');

    // Submit the form
    await user.click(screen.getByText('Create Session'));

    await waitFor(() => {
      expect(capturedBody).not.toBeNull();
    });

    expect(capturedBody!.machine_id).toBe(connectedMachine.machine_id);
    expect(capturedBody!.model).toBe('sonnet');
    // Should NOT have command: 'bash' for claude type
    expect(capturedBody!.command).toBeUndefined();
  });

  it('form submission calls createSession.mutateAsync with command bash for terminal type', async () => {
    const createdSession = buildSession({ session_id: 'new-sess-2', machine_id: 'machine-100', command: 'bash' });
    let capturedBody: Record<string, unknown> | null = null;

    server.use(
      http.post('/api/v1/sessions', async ({ request }) => {
        capturedBody = await request.json() as Record<string, unknown>;
        return HttpResponse.json(createdSession);
      }),
    );

    const onClose = vi.fn();
    const { user } = renderWithProviders(
      <NewSessionModal open={true} onClose={onClose} />,
    );

    // Switch to terminal type
    await user.click(screen.getByRole('button', { name: /Terminal/i }));

    // Wait for machines to load and select one
    const connectedMachine = mockMachines.find((m) => m.status === 'connected')!;
    await waitFor(async () => {
      const machineSelect = screen.getByDisplayValue('Select a machine...');
      await user.selectOptions(machineSelect, connectedMachine.machine_id);
    });

    // Submit the form
    await user.click(screen.getByText('Create Session'));

    await waitFor(() => {
      expect(capturedBody).not.toBeNull();
    });

    expect(capturedBody!.machine_id).toBe(connectedMachine.machine_id);
    expect(capturedBody!.command).toBe('bash');
    // Should NOT have claude-specific fields
    expect(capturedBody!.model).toBeUndefined();
    expect(capturedBody!.args).toBeUndefined();
    expect(capturedBody!.skip_permissions).toBeUndefined();
    expect(capturedBody!.template_id).toBeUndefined();
  });

  it('cancel button closes modal', async () => {
    const onClose = vi.fn();
    const { user } = renderWithProviders(
      <NewSessionModal open={true} onClose={onClose} />,
    );

    await user.click(screen.getByText('Cancel'));

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('closing modal resets form state', async () => {
    const onClose = vi.fn();
    const { user, rerender } = renderWithProviders(
      <NewSessionModal open={true} onClose={onClose} />,
    );

    // Switch to Terminal mode to change form state
    await user.click(screen.getByRole('button', { name: /Terminal/i }));

    // Verify Terminal mode is active (Claude-specific fields hidden)
    expect(screen.queryByText(/Additional Arguments/)).not.toBeInTheDocument();

    // Simulate closing and reopening by rerendering with open=false then open=true
    rerender(<NewSessionModal open={false} onClose={onClose} />);
    rerender(<NewSessionModal open={true} onClose={onClose} />);

    // Form should be reset to default Claude mode with Claude-specific fields visible
    expect(screen.getByText(/Additional Arguments/)).toBeInTheDocument();
    expect(screen.getByText(/Model/)).toBeInTheDocument();
    expect(screen.getByText(/Skip Permissions/)).toBeInTheDocument();
  });
});
