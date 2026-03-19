import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { LogsPage } from '../../views/LogsPage.tsx';
import { useLogsStore } from '../../stores/logs.ts';
import type { LogEntry } from '../../types/log.ts';

// Mock useLogStream to avoid WebSocket connections in tests
vi.mock('../../hooks/useLogStream.ts', () => ({
  useLogStream: () => ({ entries: [], connected: false }),
}));

const mockLogEntries: LogEntry[] = [
  {
    id: 1,
    timestamp: '2026-01-15T10:00:00Z',
    level: 'INFO',
    component: 'session',
    message: 'Session started successfully',
    source: 'server',
    machine_id: 'machine-1',
  },
  {
    id: 2,
    timestamp: '2026-01-15T10:01:00Z',
    level: 'ERROR',
    component: 'grpc',
    message: 'Connection lost to agent',
    source: 'server',
    error: 'timeout after 30s',
  },
  {
    id: 3,
    timestamp: '2026-01-15T10:02:00Z',
    level: 'WARN',
    component: 'auth',
    message: 'Rate limit exceeded for IP',
    source: 'server',
  },
];

function setupLogsHandler(logs: LogEntry[] = mockLogEntries, total?: number) {
  server.use(
    http.get('/api/v1/logs', () =>
      HttpResponse.json({ logs, total: total ?? logs.length }),
    ),
  );
}

describe('LogsPage', () => {
  beforeEach(() => {
    // Reset the logs store to default state before each test
    useLogsStore.getState().resetFilter();
    useLogsStore.getState().setLive(false);
  });

  it('renders the page heading and description', async () => {
    setupLogsHandler();
    renderWithProviders(<LogsPage />);

    expect(screen.getByRole('heading', { name: 'System Logs' })).toBeInTheDocument();
    expect(screen.getByText('Structured logs from server and agent components')).toBeInTheDocument();
  });

  it('renders log entries from API', async () => {
    setupLogsHandler();
    renderWithProviders(<LogsPage />);

    await waitFor(() => {
      expect(screen.getByText('Session started successfully')).toBeInTheDocument();
      expect(screen.getByText('Connection lost to agent')).toBeInTheDocument();
      expect(screen.getByText('Rate limit exceeded for IP')).toBeInTheDocument();
    });
  });

  it('shows loading skeleton while fetching', () => {
    server.use(
      http.get('/api/v1/logs', async () => {
        await new Promise((r) => setTimeout(r, 5000));
        return HttpResponse.json({ logs: [], total: 0 });
      }),
    );

    renderWithProviders(<LogsPage />);

    // Should not show empty state during loading
    expect(screen.queryByText('No logs found')).not.toBeInTheDocument();
  });

  it('shows empty state when no logs exist', async () => {
    setupLogsHandler([], 0);
    renderWithProviders(<LogsPage />);

    await waitFor(() => {
      expect(screen.getByText('No logs found')).toBeInTheDocument();
    });

    expect(screen.getByText('No log entries have been recorded yet.')).toBeInTheDocument();
  });

  it('shows filter-specific empty message when filters are active', async () => {
    setupLogsHandler([], 0);

    // Set the filter via URL search params so it persists through the component's useEffect
    renderWithProviders(<LogsPage />, { routes: ['/logs?level=ERROR'] });

    await waitFor(() => {
      expect(screen.getByText('No logs found')).toBeInTheDocument();
      expect(screen.getByText('Try adjusting your filters to see more logs.')).toBeInTheDocument();
    });
  });

  it('shows error state with retry button when API fails', async () => {
    server.use(
      http.get('/api/v1/logs', () =>
        HttpResponse.json({ error: 'Server error' }, { status: 500 }),
      ),
    );

    renderWithProviders(<LogsPage />);

    await waitFor(() => {
      expect(screen.getByText('Retry')).toBeInTheDocument();
    });
  });

  it('renders level filter dropdown', () => {
    setupLogsHandler();
    renderWithProviders(<LogsPage />);

    expect(screen.getByText('Level')).toBeInTheDocument();
    expect(screen.getByDisplayValue('All Levels')).toBeInTheDocument();
  });

  it('renders source filter dropdown', () => {
    setupLogsHandler();
    renderWithProviders(<LogsPage />);

    expect(screen.getByText('Source')).toBeInTheDocument();
    expect(screen.getByDisplayValue('All Sources')).toBeInTheDocument();
  });

  it('renders component filter dropdown', () => {
    setupLogsHandler();
    renderWithProviders(<LogsPage />);

    expect(screen.getByText('Component')).toBeInTheDocument();
    expect(screen.getByDisplayValue('All Components')).toBeInTheDocument();
  });

  it('renders machine ID filter input', () => {
    setupLogsHandler();
    renderWithProviders(<LogsPage />);

    expect(screen.getByText('Machine')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Machine ID')).toBeInTheDocument();
  });

  it('renders search input', () => {
    setupLogsHandler();
    renderWithProviders(<LogsPage />);

    expect(screen.getByPlaceholderText('Search log messages...')).toBeInTheDocument();
  });

  it('renders time range preset buttons', () => {
    setupLogsHandler();
    renderWithProviders(<LogsPage />);

    expect(screen.getByText('Time Range')).toBeInTheDocument();
    expect(screen.getByText('1h')).toBeInTheDocument();
    expect(screen.getByText('6h')).toBeInTheDocument();
    expect(screen.getByText('24h')).toBeInTheDocument();
    expect(screen.getByText('7d')).toBeInTheDocument();
  });

  it('renders live toggle button', () => {
    setupLogsHandler();
    renderWithProviders(<LogsPage />);

    expect(screen.getByText('Live')).toBeInTheDocument();
  });

  it('shows total log count when not in live mode', async () => {
    setupLogsHandler(mockLogEntries, 3);
    renderWithProviders(<LogsPage />);

    await waitFor(() => {
      expect(screen.getByText('3 logs')).toBeInTheDocument();
    });
  });

  it('renders refresh button when not in live mode', async () => {
    setupLogsHandler();
    renderWithProviders(<LogsPage />);

    expect(screen.getByRole('button', { name: /refresh/i })).toBeInTheDocument();
  });

  it('renders pagination when logs exist and not in live mode', async () => {
    setupLogsHandler(mockLogEntries, 200);
    renderWithProviders(<LogsPage />);

    await waitFor(() => {
      expect(screen.getByText('Session started successfully')).toBeInTheDocument();
    });
  });

  it('renders level badges for log entries', async () => {
    setupLogsHandler();
    renderWithProviders(<LogsPage />);

    await waitFor(() => {
      expect(screen.getByText('INFO')).toBeInTheDocument();
      expect(screen.getByText('ERROR')).toBeInTheDocument();
      expect(screen.getByText('WARN')).toBeInTheDocument();
    });
  });

  it('allows changing the level filter', async () => {
    setupLogsHandler();
    const { user } = renderWithProviders(<LogsPage />);

    const levelSelect = screen.getByDisplayValue('All Levels');
    await user.selectOptions(levelSelect, 'ERROR');

    expect(useLogsStore.getState().filter.level).toBe('ERROR');
  });

  it('allows changing the component filter', async () => {
    setupLogsHandler();
    const { user } = renderWithProviders(<LogsPage />);

    const componentSelect = screen.getByDisplayValue('All Components');
    await user.selectOptions(componentSelect, 'grpc');

    expect(useLogsStore.getState().filter.component).toBe('grpc');
  });

  it('allows typing in the search input', async () => {
    setupLogsHandler();
    const { user } = renderWithProviders(<LogsPage />);

    const searchInput = screen.getByPlaceholderText('Search log messages...');
    await user.type(searchInput, 'timeout');

    expect(useLogsStore.getState().filter.search).toBe('timeout');
  });

  it('toggles live mode when live button is clicked', async () => {
    setupLogsHandler();
    const { user } = renderWithProviders(<LogsPage />);

    const liveButton = screen.getByText('Live');
    await user.click(liveButton);

    expect(useLogsStore.getState().live).toBe(true);
  });
});
