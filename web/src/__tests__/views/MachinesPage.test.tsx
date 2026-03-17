import { describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { MachinesPage } from '../../views/MachinesPage.tsx';
import { mockMachines } from '../../test/handlers.ts';

describe('MachinesPage', () => {
  it('renders page heading', () => {
    renderWithProviders(<MachinesPage />);

    expect(screen.getByText('Machines')).toBeInTheDocument();
  });

  it('renders machine cards with display names from API', async () => {
    renderWithProviders(<MachinesPage />);

    await waitFor(() => {
      expect(screen.getByText(mockMachines[0].display_name!)).toBeInTheDocument();
      expect(screen.getByText(mockMachines[1].display_name!)).toBeInTheDocument();
    });
  });

  it('shows machine_id prefix on each card', async () => {
    renderWithProviders(<MachinesPage />);

    await waitFor(() => {
      // MachineCard renders machine_id.slice(0, 12)
      expect(screen.getByText(mockMachines[0].machine_id.slice(0, 12))).toBeInTheDocument();
      expect(screen.getByText(mockMachines[1].machine_id.slice(0, 12))).toBeInTheDocument();
    });
  });

  it('shows online/offline counts', async () => {
    renderWithProviders(<MachinesPage />);

    await waitFor(() => {
      // mockMachines: 1 connected, 1 disconnected
      expect(screen.getByText('1 online')).toBeInTheDocument();
      expect(screen.getByText('1 offline')).toBeInTheDocument();
    });
  });

  it('shows empty state when no machines', async () => {
    server.use(
      http.get('/api/v1/machines', () => HttpResponse.json([])),
    );

    renderWithProviders(<MachinesPage />);

    await waitFor(() => {
      expect(screen.getByText('No machines registered')).toBeInTheDocument();
    });
  });
});
