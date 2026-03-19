import { describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../../test/render.tsx';
import { ConnectionHealth } from '../../../components/dashboard/ConnectionHealth.tsx';
import type { Machine, MachineHealth } from '../../../lib/types.ts';

const healthData: MachineHealth = {
  cpu_cores: 4,
  memory_total_mb: 16384,
  memory_used_mb: 8192,
  active_sessions: 2,
  max_sessions: 5,
};

const connectedMachineWithHealth: Machine = {
  machine_id: 'machine-h1',
  display_name: 'Production Worker Alpha',
  status: 'connected',
  max_sessions: 5,
  home_dir: '/home/worker',
  last_health: '2026-01-15T10:00:00Z',
  last_seen_at: '2026-01-15T10:00:00Z',
  cert_expires: '2027-01-15T10:00:00Z',
  created_at: '2026-01-15T10:00:00Z',
  health: healthData,
};

const connectedMachineNoHealth: Machine = {
  machine_id: 'machine-h2',
  display_name: 'Staging Worker Beta',
  status: 'connected',
  max_sessions: 3,
  home_dir: '/home/worker',
  last_health: '2026-01-15T10:00:00Z',
  last_seen_at: '2026-01-15T10:00:00Z',
  cert_expires: '2027-01-15T10:00:00Z',
  created_at: '2026-01-15T10:00:00Z',
};

const disconnectedMachine: Machine = {
  machine_id: 'machine-h3',
  display_name: 'Offline Worker Gamma',
  status: 'disconnected',
  max_sessions: 5,
  home_dir: '/home/worker',
  last_health: '2026-01-14T10:00:00Z',
  last_seen_at: '2026-01-14T10:00:00Z',
  cert_expires: '2027-01-15T10:00:00Z',
  created_at: '2026-01-14T10:00:00Z',
};

describe('ConnectionHealth', () => {
  it('renders Connection Health heading', async () => {
    renderWithProviders(<ConnectionHealth />);
    expect(screen.getByText('Connection Health')).toBeInTheDocument();
  });

  it('shows loading skeleton initially', () => {
    // Use a handler that never resolves to see loading state
    server.use(
      http.get('/api/v1/machines', () => new Promise(() => {})),
    );

    renderWithProviders(<ConnectionHealth />);
    expect(screen.getByText('Connection Health')).toBeInTheDocument();
  });

  it('displays machine names in full (not truncated)', async () => {
    server.use(
      http.get('/api/v1/machines', () =>
        HttpResponse.json([connectedMachineWithHealth, connectedMachineNoHealth]),
      ),
    );

    renderWithProviders(<ConnectionHealth />);

    await waitFor(() => {
      expect(screen.getByText('Production Worker Alpha')).toBeInTheDocument();
      expect(screen.getByText('Staging Worker Beta')).toBeInTheDocument();
    });
  });

  it('displays green dot for healthy connected machines', async () => {
    server.use(
      http.get('/api/v1/machines', () =>
        HttpResponse.json([connectedMachineWithHealth]),
      ),
    );

    renderWithProviders(<ConnectionHealth />);

    await waitFor(() => {
      expect(screen.getByText('Production Worker Alpha')).toBeInTheDocument();
    });

    const dot = screen.getByTitle('Healthy');
    expect(dot).toBeInTheDocument();
    expect(dot.className).toContain('bg-green-500');
  });

  it('displays yellow dot for connected machine without health data', async () => {
    server.use(
      http.get('/api/v1/machines', () =>
        HttpResponse.json([connectedMachineNoHealth]),
      ),
    );

    renderWithProviders(<ConnectionHealth />);

    await waitFor(() => {
      expect(screen.getByText('Staging Worker Beta')).toBeInTheDocument();
    });

    const dot = screen.getByTitle('No health data');
    expect(dot).toBeInTheDocument();
    expect(dot.className).toContain('bg-yellow-500');
  });

  it('displays red dot for disconnected machines', async () => {
    server.use(
      http.get('/api/v1/machines', () =>
        HttpResponse.json([disconnectedMachine]),
      ),
    );

    renderWithProviders(<ConnectionHealth />);

    await waitFor(() => {
      expect(screen.getByText('Offline Worker Gamma')).toBeInTheDocument();
    });

    const dot = screen.getByTitle('Disconnected');
    expect(dot).toBeInTheDocument();
    expect(dot.className).toContain('bg-red-500');
  });

  it('displays session count for machines with health data', async () => {
    server.use(
      http.get('/api/v1/machines', () =>
        HttpResponse.json([connectedMachineWithHealth]),
      ),
    );

    renderWithProviders(<ConnectionHealth />);

    await waitFor(() => {
      expect(screen.getByText('Sessions: 2 / 5')).toBeInTheDocument();
    });
  });

  it('displays "unknown" sessions for connected machines without health', async () => {
    server.use(
      http.get('/api/v1/machines', () =>
        HttpResponse.json([connectedMachineNoHealth]),
      ),
    );

    renderWithProviders(<ConnectionHealth />);

    await waitFor(() => {
      expect(screen.getByText('Sessions: unknown')).toBeInTheDocument();
    });
  });

  it('displays "--" sessions for disconnected machines without health', async () => {
    server.use(
      http.get('/api/v1/machines', () =>
        HttpResponse.json([disconnectedMachine]),
      ),
    );

    renderWithProviders(<ConnectionHealth />);

    await waitFor(() => {
      expect(screen.getByText('Sessions: --')).toBeInTheDocument();
    });
  });

  it('shows "No machines registered" when list is empty', async () => {
    server.use(
      http.get('/api/v1/machines', () => HttpResponse.json([])),
    );

    renderWithProviders(<ConnectionHealth />);

    await waitFor(() => {
      expect(screen.getByText('No machines registered.')).toBeInTheDocument();
    });
  });

  it('falls back to machine_id when display_name is empty', async () => {
    const machineNoName: Machine = {
      ...connectedMachineWithHealth,
      machine_id: 'worker-fallback-id',
      display_name: '',
    };

    server.use(
      http.get('/api/v1/machines', () => HttpResponse.json([machineNoName])),
    );

    renderWithProviders(<ConnectionHealth />);

    await waitFor(() => {
      expect(screen.getByText('worker-fallback-id')).toBeInTheDocument();
    });
  });

  it('renders multiple machines in a grid', async () => {
    server.use(
      http.get('/api/v1/machines', () =>
        HttpResponse.json([connectedMachineWithHealth, connectedMachineNoHealth, disconnectedMachine]),
      ),
    );

    renderWithProviders(<ConnectionHealth />);

    await waitFor(() => {
      expect(screen.getByText('Production Worker Alpha')).toBeInTheDocument();
      expect(screen.getByText('Staging Worker Beta')).toBeInTheDocument();
      expect(screen.getByText('Offline Worker Gamma')).toBeInTheDocument();
    });
  });

  it('displays last seen time for machines', async () => {
    server.use(
      http.get('/api/v1/machines', () =>
        HttpResponse.json([connectedMachineWithHealth]),
      ),
    );

    renderWithProviders(<ConnectionHealth />);

    await waitFor(() => {
      expect(screen.getByText(/Last seen:/)).toBeInTheDocument();
    });
  });
});
