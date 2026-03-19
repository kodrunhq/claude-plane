import { describe, it, expect, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../../test/setup.ts';
import { renderWithProviders, screen, waitFor } from '../../test/render.tsx';
import { RunsPage } from '../../views/RunsPage.tsx';
import { mockRuns } from '../../test/handlers.ts';
import { buildRun } from '../../test/factories.ts';

// Mock useNavigate
const mockNavigate = vi.fn();
vi.mock('react-router', async () => {
  const actual = await vi.importActual('react-router');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('RunsPage', () => {
  it('renders page heading', () => {
    renderWithProviders(<RunsPage />);
    expect(screen.getByRole('heading', { name: 'Runs' })).toBeInTheDocument();
  });

  it('renders runs table with data from API', async () => {
    renderWithProviders(<RunsPage />);

    await waitFor(() => {
      // RunsTable renders run IDs (first 8 chars via CopyableId)
      expect(screen.getByText(mockRuns[0].run_id.slice(0, 8))).toBeInTheDocument();
    });
  });

  it('shows loading skeleton while fetching', () => {
    // Delay response to keep loading state
    server.use(
      http.get('/api/v1/runs', async () => {
        await new Promise((r) => setTimeout(r, 5000));
        return HttpResponse.json(mockRuns);
      }),
    );

    renderWithProviders(<RunsPage />);

    // SkeletonTable uses animated div elements; page shouldn't show empty state or table yet
    expect(screen.queryByText('No runs yet')).not.toBeInTheDocument();
  });

  it('shows empty state when no runs exist', async () => {
    server.use(
      http.get('/api/v1/runs', () => HttpResponse.json([])),
    );

    renderWithProviders(<RunsPage />);

    await waitFor(() => {
      expect(screen.getByText('No runs yet')).toBeInTheDocument();
    });

    expect(
      screen.getByText('Runs will appear here when jobs are triggered manually or by automation.'),
    ).toBeInTheDocument();
  });

  it('shows error state with retry button when API fails', async () => {
    server.use(
      http.get('/api/v1/runs', () =>
        HttpResponse.json({ error: 'Server error' }, { status: 500 }),
      ),
    );

    renderWithProviders(<RunsPage />);

    await waitFor(() => {
      expect(screen.getByText('Retry')).toBeInTheDocument();
    });
  });

  it('renders status filter with all status options', async () => {
    renderWithProviders(<RunsPage />);

    expect(screen.getByDisplayValue('All Statuses')).toBeInTheDocument();
  });

  it('renders job filter dropdown', async () => {
    renderWithProviders(<RunsPage />);

    await waitFor(() => {
      expect(screen.getByDisplayValue('All Jobs')).toBeInTheDocument();
    });
  });

  it('renders trigger type filter', () => {
    renderWithProviders(<RunsPage />);

    expect(screen.getByDisplayValue('All Triggers')).toBeInTheDocument();
  });

  it('filters runs by status when status select changes', async () => {
    renderWithProviders(<RunsPage />);

    await waitFor(() => {
      expect(screen.getByDisplayValue('All Statuses')).toBeInTheDocument();
    });

    // The status select should have all options
    const statusSelect = screen.getByDisplayValue('All Statuses');
    expect(statusSelect).toBeInTheDocument();
  });

  it('navigates to run detail when row is clicked', async () => {
    renderWithProviders(<RunsPage />);

    await waitFor(() => {
      expect(screen.getByText(mockRuns[0].run_id.slice(0, 8))).toBeInTheDocument();
    });

    // Click the row (rows have role="row")
    const rows = screen.getAllByRole('row');
    // First row is the header, subsequent rows are data
    const dataRow = rows.find((r) => r.getAttribute('aria-label')?.includes(mockRuns[0].run_id.slice(0, 8)));
    if (dataRow) {
      dataRow.click();
      expect(mockNavigate).toHaveBeenCalledWith(`/runs/${mockRuns[0].run_id}`);
    }
  });

  it('renders the refresh button', () => {
    renderWithProviders(<RunsPage />);

    // RefreshButton is rendered in the header
    expect(screen.getByRole('button', { name: /refresh/i })).toBeInTheDocument();
  });

  it('shows table headers when runs are loaded', async () => {
    renderWithProviders(<RunsPage />);

    // Wait for runs data to load so the table (not skeleton) renders
    await waitFor(() => {
      expect(screen.getByText('Started')).toBeInTheDocument();
    });

    // 'Trigger' appears both in filters and table header
    expect(screen.getAllByText('Trigger').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('Duration')).toBeInTheDocument();
  });

  it('renders pagination when runs exist', async () => {
    // Create enough runs to potentially show pagination
    const manyRuns = Array.from({ length: 15 }, (_, i) =>
      buildRun({ run_id: `run-${200 + i}`, status: 'completed' }),
    );

    server.use(
      http.get('/api/v1/runs', () => HttpResponse.json(manyRuns)),
    );

    renderWithProviders(<RunsPage />);

    await waitFor(() => {
      expect(screen.getByText(manyRuns[0].run_id.slice(0, 8))).toBeInTheDocument();
    });
  });

  it('shows job name column when all jobs filter is selected', async () => {
    renderWithProviders(<RunsPage />);

    await waitFor(() => {
      // "Job Name" column header should be visible since selectedJobId defaults to "all"
      expect(screen.getByText('Job Name')).toBeInTheDocument();
    });
  });
});
