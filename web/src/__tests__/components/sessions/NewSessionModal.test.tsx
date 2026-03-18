import { describe, it, expect } from 'vitest';
import { renderWithProviders, screen, waitFor } from '../../../test/render.tsx';
import { NewSessionModal } from '../../../components/sessions/NewSessionModal.tsx';
import { mockMachines } from '../../../test/handlers.ts';

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
});
